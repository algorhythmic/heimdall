package continuity

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"heimdall/internal/authz"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"sort"
	"strings"
	"time"
)

type SummaryOptions struct {
	Target  string
	Subtree bool
	Sort    string
	Limit   int
	Cursor  string
}
type SummaryCheckpoint struct {
	ID      string    `json:"id"`
	At      time.Time `json:"at"`
	Summary string    `json:"summary"`
}
type SummaryDecision struct {
	ID     string `json:"id"`
	Target string `json:"target"`
	Text   string `json:"text"`
	Status string `json:"status"`
}
type SummaryStep struct {
	Target    string `json:"target"`
	DependsOn string `json:"depends_on"`
	Status    string `json:"status"`
}
type ProgressOverviewItem struct {
	Target               string             `json:"target"`
	Title                string             `json:"title"`
	TaskRevision         int64              `json:"task_revision"`
	Status               string             `json:"status"`
	ResumeBy             string             `json:"resume_by,omitempty"`
	Checkpoint           *SummaryCheckpoint `json:"checkpoint,omitempty"`
	NextAction           string             `json:"next_action"`
	DirectionStatus      string             `json:"direction_status"`
	Blockers             []string           `json:"blockers"`
	Dependencies         []DependencyView   `json:"dependencies"`
	StepPrerequisites    []SummaryStep      `json:"step_prerequisites"`
	PendingDecisions     []SummaryDecision  `json:"pending_decisions"`
	PendingDecisionCount int                `json:"pending_decision_count"`
}
type ProgressOverview struct {
	Version      int                    `json:"version"`
	Target       string                 `json:"target"`
	Subtree      bool                   `json:"subtree"`
	Sort         string                 `json:"sort"`
	Observations string                 `json:"observations"`
	Items        []ProgressOverviewItem `json:"items"`
	NextCursor   string                 `json:"next_cursor,omitempty"`
}
type summaryCursor struct {
	Version   int    `json:"version"`
	Authority string `json:"authority"`
	Target    string `json:"target"`
	Subtree   bool   `json:"subtree"`
	Sort      string `json:"sort"`
	After     string `json:"after"`
	Event     int64  `json:"event"`
}

func (r SummaryOptions) Validate() error {
	if (r.Target != "*" && !model.ValidID(r.Target)) || !model.Contains([]string{"id", "checkpoint", "due"}, r.Sort) || r.Limit < 1 || r.Limit > 50 || len(r.Cursor) > 4096 || (r.Target == "*" && r.Subtree) {
		return fmt.Errorf("summary requires task or explicit all-task scope, sort id/checkpoint/due, limit 1..50 and optional subtree/cursor")
	}
	return nil
}
func summaryItem(st model.State, target string, visible func(string) bool) ProgressOverviewItem {
	t := st.Tasks[target]
	i := ProgressOverviewItem{Target: target, Title: t.Task.Title, TaskRevision: t.Revision, Status: t.Task.Status, ResumeBy: t.Task.ResumeBy, Blockers: []string{}, Dependencies: DependencyViews(st, target, visible), StepPrerequisites: []SummaryStep{}, PendingDecisions: []SummaryDecision{}}
	i.NextAction, i.DirectionStatus = RecordedNextAction(st, target)
	lineage, _ := model.TargetLineage(st, target)
	for _, parent := range lineage {
		if !visible(parent) {
			i.NextAction = t.Task.NextAction
			i.DirectionStatus = "scope_limited"
			break
		}
	}
	if cp, ok := st.Checkpoints[st.CheckpointHeads[target]]; ok {
		i.Checkpoint = &SummaryCheckpoint{ID: cp.ID, At: cp.At, Summary: cp.Summary}
		i.Blockers = append(i.Blockers, cp.Blockers...)
	}
	byStep := map[string]model.Step{}
	for _, s := range t.Task.Subtasks {
		byStep[s.ID] = s
	}
	for _, s := range t.Task.Subtasks {
		for _, after := range s.After {
			p := byStep[after]
			status := "pending"
			if p.Status == "done" {
				status = "satisfied"
			} else if p.Status == "dropped" {
				status = "dropped"
			}
			i.StepPrerequisites = append(i.StepPrerequisites, SummaryStep{Target: target + "#" + s.ID, DependsOn: target + "#" + after, Status: status})
		}
	}
	sort.Slice(i.StepPrerequisites, func(a, b int) bool {
		x, y := i.StepPrerequisites[a], i.StepPrerequisites[b]
		if x.Target != y.Target {
			return x.Target < y.Target
		}
		return x.DependsOn < y.DependsOn
	})
	ps := []model.ProgressProposal{}
	for _, p := range st.ProgressProposals {
		if p.Kind == "decision" && (p.Target == target || strings.HasPrefix(p.Target, target+"#")) {
			status := model.ProgressStatus(st, p)
			if status == "draft" || status == "reviewed" {
				ps = append(ps, p)
			}
		}
	}
	sort.Slice(ps, func(a, b int) bool {
		if !ps[a].At.Equal(ps[b].At) {
			return ps[a].At.Before(ps[b].At)
		}
		return ps[a].ID < ps[b].ID
	})
	i.PendingDecisionCount = len(ps)
	if len(ps) > 8 {
		ps = ps[:8]
	}
	for _, p := range ps {
		i.PendingDecisions = append(i.PendingDecisions, SummaryDecision{ID: p.ID, Target: p.Target, Text: p.Text, Status: model.ProgressStatus(st, p)})
	}
	return i
}

// BuildProgressOverview uses recorded state only. Scope is checked before rows,
// ordering and cursor positions are constructed; inaccessible edges are opaque.
func BuildProgressOverview(st model.State, r SummaryOptions, authority string, visible func(string) bool) (ProgressOverview, error) {
	out := ProgressOverview{Version: 1, Target: r.Target, Subtree: r.Subtree, Sort: r.Sort, Observations: "recorded_only", Items: []ProgressOverviewItem{}}
	if err := r.Validate(); err != nil {
		return out, err
	}
	if r.Target == "*" {
		if authority != "cli" {
			return out, authz.ErrDenied
		}
	} else if !visible(r.Target) {
		return out, authz.ErrDenied
	}
	if r.Target != "*" {
		if _, ok := st.Tasks[r.Target]; !ok {
			return out, fmt.Errorf("summary task not found")
		}
	}
	ids := []string{}
	selection := model.Grant{Target: r.Target, Subtree: r.Subtree}
	for id := range st.Tasks {
		if visible(id) && (r.Target == "*" || selection.Contains(st, id)) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(a, b int) bool {
		x, y := ids[a], ids[b]
		switch r.Sort {
		case "checkpoint":
			cx, cy := st.Checkpoints[st.CheckpointHeads[x]], st.Checkpoints[st.CheckpointHeads[y]]
			if !cx.At.Equal(cy.At) {
				return cx.At.After(cy.At)
			}
		case "due":
			dx, dy := st.Tasks[x].Task.ResumeBy, st.Tasks[y].Task.ResumeBy
			if dx != dy {
				if dx == "" {
					return false
				}
				if dy == "" {
					return true
				}
				return dx < dy
			}
		}
		return x < y
	})
	start := 0
	if r.Cursor != "" {
		var c summaryCursor
		raw, err := base64.RawURLEncoding.DecodeString(r.Cursor)
		if err != nil || model.StrictJSON(raw, &c) != nil || c.Version != 1 || c.Authority != authority || c.Target != r.Target || c.Subtree != r.Subtree || c.Sort != r.Sort {
			return out, fmt.Errorf("summary cursor scope mismatch")
		}
		if c.Event != st.LastEventID {
			return out, fmt.Errorf("summary changed; restart pagination: %w", store.ErrConflict)
		}
		found := false
		for n, id := range ids {
			if id == c.After {
				start = n + 1
				found = true
				break
			}
		}
		if !found {
			return out, fmt.Errorf("invalid summary cursor position")
		}
	}
	bytes := 8192
	last := ""
	for _, id := range ids[start:] {
		i := summaryItem(st, id, visible)
		raw, _ := json.Marshal(i)
		if bytes+len(raw) > MaxReadBytes || len(out.Items) >= r.Limit {
			if len(out.Items) == 0 {
				return out, fmt.Errorf("summary item exceeds 512 KiB")
			}
			b, _ := json.Marshal(summaryCursor{Version: 1, Authority: authority, Target: r.Target, Subtree: r.Subtree, Sort: r.Sort, After: last, Event: st.LastEventID})
			out.NextCursor = base64.RawURLEncoding.EncodeToString(b)
			break
		}
		out.Items = append(out.Items, i)
		last = id
		bytes += len(raw) + 1
	}
	return out, nil
}
func (s Service) ProgressOverview(ctx context.Context, r SummaryOptions) (ProgressOverview, error) {
	st, err := s.Store.State(ctx)
	if err != nil {
		return ProgressOverview{}, err
	}
	return BuildProgressOverview(st, r, "cli", func(string) bool { return true })
}
func ScopedProgressOverview(st model.State, g model.Grant, r SummaryOptions) (ProgressOverview, error) {
	return BuildProgressOverview(st, r, g.ID, func(target string) bool { return g.Contains(st, target) })
}
