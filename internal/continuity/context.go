package continuity

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"reflect"
	"slices"
	"sort"
	"time"
)

type Issue struct {
	Code   string `json:"code"`
	Target string `json:"target"`
	Detail string `json:"detail"`
}
type ResourceCheck struct {
	Resource model.Resource  `json:"resource"`
	Expected *model.Snapshot `json:"expected,omitempty"`
	Observed *model.Snapshot `json:"observed,omitempty"`
	Status   string          `json:"status"`
	Detail   string          `json:"detail,omitempty"`
}
type Bundle struct {
	Version         int                `json:"version"`
	SourceEvent     int64              `json:"source_event"`
	Target          string             `json:"target"`
	Task            model.TaskRecord   `json:"task"`
	Ancestors       []model.TaskRecord `json:"ancestors"`
	Contracts       []model.Contract   `json:"contracts"`
	Decisions       []model.Decision   `json:"decisions"`
	Checkpoint      *model.Checkpoint  `json:"checkpoint,omitempty"`
	Resources       []ResourceCheck    `json:"resources"`
	Issues          []Issue            `json:"issues"`
	ResumeStatus    string             `json:"resume_status"`
	Coverage        map[string]string  `json:"coverage"`
	EstimatedTokens int                `json:"estimated_tokens"`
}
type BudgetError struct {
	Required int `json:"required_estimate"`
	Budget   int `json:"budget"`
}

func (e *BudgetError) Error() string {
	return fmt.Sprintf("budget_too_small: required estimate %d, budget %d; mandatory context was not truncated", e.Required, e.Budget)
}
func (s Service) Context(ctx context.Context, target string, budget int) (Bundle, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	st, err := s.Store.State(ctx)
	if err != nil {
		return Bundle{}, err
	}
	return buildContext(ctx, st, target, budget)
}
func buildContext(ctx context.Context, st model.State, target string, budget int) (Bundle, error) {
	out := Bundle{Version: 1, SourceEvent: st.LastEventID, Target: target, Ancestors: []model.TaskRecord{}, Contracts: []model.Contract{}, Decisions: []model.Decision{}, Resources: []ResourceCheck{}, Issues: []Issue{}, ResumeStatus: "ready", Coverage: map[string]string{"resources": "two-pass file/tree observation; exclusions apply; no filesystem lock", "git": "commit/ref identity not captured", "browser_actions": "not task-bound; outcomes not evaluated", "retrieval": "not integrated", "execution": "no runner or execution authorization implied", "budget": "ceil(serialized UTF-8 bytes/4), an estimate, not a tokenizer"}}
	if budget < 0 {
		return out, fmt.Errorf("budget must be nonnegative")
	}
	task, step, err := model.ResolveTarget(st, target)
	if err != nil {
		return out, err
	}
	out.Task = task
	targets, err := lineage(st, target)
	if err != nil {
		return out, err
	}
	issue := func(code, target, detail string) {
		out.Issues = append(out.Issues, Issue{code, target, detail})
		out.ResumeStatus = "needs_review"
	}
	for _, t := range targets {
		r, _, _ := model.ResolveTarget(st, t)
		if t != target && t != task.Task.ID {
			out.Ancestors = append(out.Ancestors, r)
		}
		if id := st.ContractHeads[t]; id != "" {
			c := st.Contracts[id]
			out.Contracts = append(out.Contracts, c)
			ownLineage, err := lineage(st, t)
			if err != nil {
				return out, err
			}
			if c.Version != 2 || !slices.Equal(c.ResourceIDs, resourceIDs(st, ownLineage)) {
				issue("contract_scope_changed", t, "Resource scope is changed or unreviewed; explicitly reaccept the contract")
			}
			if c.TaskRevision != r.Revision {
				issue("stale_contract", t, "Task revision changed after acceptance; review and explicitly reaccept its contract")
			}
		} else if t == target {
			issue("missing_contract", t, "Accept a task contract before recording a checkpoint")
		}
		if model.Contains(r.Workflow.Success, r.Task.Status) || model.Contains(r.Workflow.Dropped, r.Task.Status) || r.Task.Status == "blocked" {
			issue("inactive_task", t, "Task or ancestor is terminal or blocked")
		}
	}
	if step != nil && (step.Status == "done" || step.Status == "dropped" || step.Status == "blocked") {
		issue("inactive_step", target, "Step is terminal or blocked")
	}
	ids := decisionIDs(st, targets)
	for _, id := range ids {
		out.Decisions = append(out.Decisions, st.Decisions[id])
	}
	expected := map[string]model.Snapshot{}
	if cp, ok := st.Checkpoints[st.CheckpointHeads[target]]; ok {
		out.Checkpoint = &cp
		if !reflect.DeepEqual(cp.Context, versions(st, targets)) {
			issue("context_changed", target, "Task lineage, revision or accepted contract changed since checkpoint")
		}
		if !reflect.DeepEqual(cp.Decisions, ids) {
			issue("decisions_changed", target, "Accepted decisions changed since checkpoint")
		}
		for _, b := range cp.Blockers {
			issue("checkpoint_blocker", target, b)
		}
		for _, r := range cp.Resources {
			expected[r.ID] = r.Snapshot
		}
	} else {
		issue("missing_checkpoint", target, "No saved checkpoint; begin explicitly rather than assuming prior progress")
	}
	active := map[string]bool{}
	for i, r := range resources(st, targets) {
		active[r.ID] = true
		check := ResourceCheck{Resource: r, Status: "unknown"}
		if prior, ok := expected[r.ID]; ok {
			check.Expected = &prior
		} else if out.Checkpoint != nil {
			issue("resource_added", r.ID, "Binding was added after checkpoint")
		}
		if i >= 16 {
			check.Detail = "lineage resource observation limit exceeded"
			issue("resource_unavailable", r.ID, check.Detail)
			out.Resources = append(out.Resources, check)
			continue
		}
		live, err := Observe(ctx, r)
		if err != nil {
			check.Detail = err.Error()
			issue("resource_unavailable", r.ID, err.Error())
		} else {
			check.Observed = &live
			check.Status = "observed"
			if check.Expected != nil {
				if live == *check.Expected {
					check.Status = "matched"
				} else {
					check.Status = "changed"
					issue("resource_changed", r.ID, "Working files differ from the checkpoint")
				}
			}
		}
		out.Resources = append(out.Resources, check)
	}
	removed := []string{}
	for id := range expected {
		if !active[id] {
			removed = append(removed, id)
		}
	}
	sort.Strings(removed)
	for _, id := range removed {
		issue("resource_removed", id, "A checkpoint resource is no longer bound in the current lineage")
	}
	// Fixed-point estimate includes the estimate field itself. Required context is never packed away.
	for i := 0; i < 8; i++ {
		raw, err := json.Marshal(out)
		if err != nil {
			return out, err
		}
		n := (len(raw) + 3) / 4
		if n == out.EstimatedTokens {
			break
		}
		out.EstimatedTokens = n
	}
	if out.EstimatedTokens > budget {
		return Bundle{}, &BudgetError{Required: out.EstimatedTokens, Budget: budget}
	}
	return out, nil
}

type View struct {
	Target         string             `json:"target"`
	TaskRevision   int64              `json:"task_revision"`
	ContractHead   string             `json:"contract_head"`
	CheckpointHead string             `json:"checkpoint_head"`
	Contracts      []model.Contract   `json:"contracts"`
	Decisions      []model.Decision   `json:"decisions"`
	Resources      []model.Resource   `json:"resources"`
	Checkpoints    []model.Checkpoint `json:"checkpoints"`
}

func (s Service) View(ctx context.Context, target string) (View, error) {
	st, err := s.Store.State(ctx)
	if err != nil {
		return View{}, err
	}
	r, _, err := model.ResolveTarget(st, target)
	if err != nil {
		return View{}, err
	}
	v := View{Target: target, TaskRevision: r.Revision, ContractHead: st.ContractHeads[target], CheckpointHead: st.CheckpointHeads[target], Contracts: []model.Contract{}, Decisions: []model.Decision{}, Resources: []model.Resource{}, Checkpoints: []model.Checkpoint{}}
	for _, c := range st.Contracts {
		if c.Target == target {
			v.Contracts = append(v.Contracts, c)
		}
	}
	for _, d := range st.Decisions {
		if d.Target == target {
			v.Decisions = append(v.Decisions, d)
		}
	}
	for _, r := range st.Resources {
		if r.Target == target {
			v.Resources = append(v.Resources, r)
		}
	}
	for _, c := range st.Checkpoints {
		if c.Target == target {
			v.Checkpoints = append(v.Checkpoints, c)
		}
	}
	sort.Slice(v.Contracts, func(i, j int) bool { return v.Contracts[i].ID < v.Contracts[j].ID })
	sort.Slice(v.Decisions, func(i, j int) bool { return v.Decisions[i].ID < v.Decisions[j].ID })
	sort.Slice(v.Resources, func(i, j int) bool { return v.Resources[i].ID < v.Resources[j].ID })
	sort.Slice(v.Checkpoints, func(i, j int) bool {
		if v.Checkpoints[i].SourceEvent != v.Checkpoints[j].SourceEvent {
			return v.Checkpoints[i].SourceEvent < v.Checkpoints[j].SourceEvent
		}
		return v.Checkpoints[i].ID < v.Checkpoints[j].ID
	})
	return v, nil
}
