package core

import (
	"fmt"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"sort"
	"strings"
	"time"
)

type CheckResult struct {
	ID       string   `json:"id"`
	Kind     string   `json:"kind"`
	Status   string   `json:"status"`
	Evidence []string `json:"evidence"`
}

func Evaluate(st model.State, id string) []CheckResult {
	r, ok := st.Tasks[id]
	if !ok {
		return nil
	}
	out := []CheckResult{}
	for _, c := range r.Task.Done.Checks {
		cr := CheckResult{ID: c.ID, Kind: c.Kind, Status: "unsupported", Evidence: []string{}}
		switch c.Kind {
		case "manual":
			cr.Status = "not_matched"
		case "silence":
			cr.Status = "unknown" // Timer expiry without mailbox coverage is not fulfillment.
		case "subtasks_done":
			cr.Status = "matched"
			count := 0
			for _, s := range r.Task.Subtasks {
				if s.Status == "dropped" {
					continue
				}
				count++
				if s.Status != "done" {
					cr.Status = "not_matched"
				} else {
					cr.Evidence = append(cr.Evidence, id+"#"+s.ID+":"+r.Completed[s.ID].Token)
				}
			}
			if count == 0 {
				cr.Status = "not_matched"
			}
		case "children_done":
			cr.Status = "matched"
			count := 0
			for _, childID := range sortedKeys(st.Tasks) {
				child := st.Tasks[childID]
				if child.Task.Parent != id || model.Contains(child.Workflow.Dropped, child.Task.Status) {
					continue
				}
				count++
				if !model.Contains(child.Workflow.Success, child.Task.Status) {
					cr.Status = "not_matched"
				} else {
					cr.Evidence = append(cr.Evidence, fmt.Sprintf("%s@%d", childID, child.Revision))
				}
			}
			if count == 0 {
				cr.Status = "not_matched"
			}
		}
		sort.Strings(cr.Evidence)
		out = append(out, cr)
	}
	return out
}
func proposalFor(st model.State, id string, now time.Time) (model.Proposal, bool) {
	r := st.Tasks[id]
	if terminal(r) {
		return model.Proposal{}, false
	}
	results := Evaluate(st, id)
	if len(results) == 0 {
		return model.Proposal{}, false
	}
	matched := 0
	evidence := []string{}
	for _, x := range results {
		if x.Status == "matched" {
			matched++
			evidence = append(evidence, x.ID+":"+strings.Join(x.Evidence, ","))
		}
	}
	if matched == 0 || (r.Task.Done.Mode == "all" && matched != len(results)) {
		return model.Proposal{}, false
	}
	sort.Strings(evidence)
	key := digest([]byte(fmt.Sprintf("fulfill\n%s\n%d\n%s", id, r.Revision, strings.Join(evidence, "\n"))))
	return model.Proposal{ID: key, Target: id, TargetRevision: r.Revision, Kind: "fulfill", Status: "pending", Evidence: evidence, CreatedAt: now}, true
}
func (b *builder) ratify(c Command) error {
	p, ok := b.state.Proposals[c.Target]
	if !ok || p.Status != "pending" {
		return fmt.Errorf("proposal is not pending")
	}
	if c.Action != "accept" && c.Action != "reject" {
		return fmt.Errorf("action must be accept or reject")
	}
	fresh, matched := proposalFor(b.state, p.Target, b.now)
	if !matched || fresh.ID != p.ID {
		return store.ErrConflict
	}
	verb := "rejected"
	p.Status = "rejected"
	if c.Action == "accept" {
		verb = "accepted"
		p.Status = "accepted"
	}
	if err := b.emit("proposal", verb, p.ID, p); err != nil {
		return err
	}
	if c.Action == "accept" {
		return b.transition(p.Target, "complete")
	}
	return nil
}
func (b *builder) reconcile() error {
	candidates := map[string]model.Proposal{}
	for _, id := range sortedKeys(b.state.Tasks) {
		if p, ok := proposalFor(b.state, id, b.now); ok {
			candidates[p.ID] = p
		}
	}
	for _, id := range sortedKeys(b.state.Proposals) {
		p := b.state.Proposals[id]
		if p.Status == "pending" {
			if _, ok := candidates[id]; !ok {
				p.Status = "superseded"
				if err := b.emit("proposal", "superseded", id, p); err != nil {
					return err
				}
			}
		}
	}
	for _, id := range sortedKeys(candidates) {
		if _, exists := b.state.Proposals[id]; !exists {
			if err := b.emit("proposal", "created", id, candidates[id]); err != nil {
				return err
			}
		}
	}
	wanted := map[string]model.Timer{}
	for _, id := range sortedKeys(b.state.Captures) {
		c := b.state.Captures[id]
		if c.ExpiresAt != nil && !c.Expired {
			key := "capture-" + digest([]byte(id+":"+c.ExpiresAt.Format(time.RFC3339Nano)))
			wanted[key] = model.Timer{ID: key, Target: id, Kind: "capture_expiry", DueAt: *c.ExpiresAt, Status: "scheduled"}
		}
	}
	for _, id := range sortedKeys(b.state.Tasks) {
		r := b.state.Tasks[id]
		if terminal(r) {
			continue
		}
		for _, c := range r.Task.Done.Checks {
			if c.Kind != "silence" {
				continue
			}
			parts := strings.Split(c.After, "#")
			if len(parts) != 2 {
				continue
			}
			stamp, ok := r.Completed[parts[1]]
			if !ok {
				continue
			}
			key := "silence-" + digest([]byte(id+"/"+c.ID+"/"+stamp.Token+fmt.Sprint(c.Days)))
			wanted[key] = model.Timer{ID: key, Target: id, Kind: "silence_review", Anchor: stamp.Token, DueAt: stamp.At.AddDate(0, 0, c.Days), Status: "scheduled"}
		}
	}
	for _, id := range sortedKeys(b.state.Timers) {
		t := b.state.Timers[id]
		if t.Status == "scheduled" || (t.Status == "due" && t.Kind == "silence_review") {
			if _, ok := wanted[id]; !ok {
				t.Status = "cancelled"
				if err := b.emit("timer", "cancelled", id, t); err != nil {
					return err
				}
			}
		}
	}
	for _, id := range sortedKeys(wanted) {
		if _, ok := b.state.Timers[id]; !ok {
			if err := b.emit("timer", "scheduled", id, wanted[id]); err != nil {
				return err
			}
		}
	}
	return nil
}
func (b *builder) tick() error {
	for _, id := range sortedKeys(b.state.Timers) {
		t := b.state.Timers[id]
		if t.Status != "scheduled" || t.DueAt.After(b.now) {
			continue
		}
		t.Status = "due"
		if t.Kind == "capture_expiry" {
			c, ok := b.state.Captures[t.Target]
			if !ok || c.Expired {
				continue
			}
			c.Expired = true
			t.Outcome = "expired"
			if err := b.emit("capture", "expired", c.ID, c); err != nil {
				return err
			}
		} else {
			t.Outcome = "review_required_without_mail_coverage"
		}
		if err := b.emit("timer", "due", id, t); err != nil {
			return err
		}
	}
	return nil
}
