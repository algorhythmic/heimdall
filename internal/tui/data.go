// Package tui is the local, keyboard-first Heimdall workstream interface.
// It uses the existing CLI daemon boundary and never opens the database.
package tui

import (
	"context"
	"encoding/json"
	"heimdall/internal/continuity"
	"heimdall/internal/core"
	"heimdall/internal/model"
	"net/url"
	"sort"
	"strings"
	"time"
)

type Call func(context.Context, string, string, any) ([]byte, error)
type Options struct {
	Target, DataDir string
	Compact         bool
	Now             func() time.Time
}
type snapshot struct {
	State       model.State
	At          time.Time
	Target      string
	Resume      *continuity.ResumeView
	DetailError string
	Usage       []usageRecord
}
type row struct {
	Target string
	Depth  int
	Step   bool
}
type need = continuity.Need

func fetch(ctx context.Context, call Call, target string) (snapshot, error) {
	raw, err := call(ctx, "GET", "/state", nil)
	if err != nil {
		return snapshot{}, err
	}
	v := snapshot{State: model.Empty(), At: time.Now(), Target: target}
	if err = json.Unmarshal(raw, &v.State); err != nil {
		return v, err
	}
	if target != "" {
		raw, err = call(ctx, "GET", "/continuity/resume?target="+url.QueryEscape(target)+"&budget=120000", nil)
		if err != nil {
			v.DetailError = err.Error()
		} else {
			var r continuity.ResumeView
			if err = json.Unmarshal(raw, &r); err != nil {
				v.DetailError = err.Error()
			} else if r.Target != target {
				v.DetailError = "Unexpected task in response"
			} else {
				v.Resume = &r
			}
		}
	}
	v.Usage = readAgentUsage()
	return v, nil
}
func (a *App) inScope(target string) bool {
	if a.opts.Target == "" {
		return true
	}
	root := strings.Split(a.opts.Target, "#")[0]
	return (model.Grant{Target: root, Subtree: true}).Contains(a.data.State, target)
}
func (a *App) taskOrder() []string {
	ids := []string{}
	for id := range a.data.State.Tasks {
		if a.inScope(id) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		x, y := a.data.State.Tasks[ids[i]], a.data.State.Tasks[ids[j]]
		if x.Task.ResumeBy != y.Task.ResumeBy {
			if x.Task.ResumeBy == "" {
				return false
			}
			if y.Task.ResumeBy == "" {
				return true
			}
			return x.Task.ResumeBy < y.Task.ResumeBy
		}
		xc, yc := a.data.State.Checkpoints[a.data.State.CheckpointHeads[ids[i]]], a.data.State.Checkpoints[a.data.State.CheckpointHeads[ids[j]]]
		if !xc.At.Equal(yc.At) {
			return xc.At.After(yc.At)
		}
		return ids[i] < ids[j]
	})
	return ids
}
func (a *App) rows() []row {
	ids := a.taskOrder()
	out := []row{}
	seen := map[string]bool{}
	var add func(string, int)
	add = func(id string, depth int) {
		if seen[id] {
			return
		}
		seen[id] = true
		t := a.data.State.Tasks[id].Task
		query := strings.ToLower(a.query)
		matches := strings.Contains(strings.ToLower(id+" "+t.Title+" "+t.NextAction), query)
		if matches {
			out = append(out, row{id, depth, false})
		}
		if a.expanded[id] || query != "" {
			for _, s := range t.Subtasks {
				if matches || strings.Contains(strings.ToLower(s.ID+" "+s.Title), query) {
					out = append(out, row{id + "#" + s.ID, depth + 1, true})
				}
			}
			for _, child := range ids {
				if a.data.State.Tasks[child].Task.Parent == id {
					add(child, depth+1)
				}
			}
		}
	}
	for _, id := range ids {
		p := a.data.State.Tasks[id].Task.Parent
		if p == "" || !a.inScope(p) {
			add(id, 0)
		}
	}
	// A filter also searches collapsed descendants without changing their identity.
	if a.query != "" {
		for _, id := range ids {
			add(id, 0)
		}
	}
	return out
}

// scopeRoot is the task scope for needs and row filtering; empty scopes all.
func (a *App) scopeRoot() string { return rootOf(a.opts.Target) }

// needs returns shared attention items minus view-local snoozes.
func (a *App) needs() []need {
	out := []need{}
	for _, n := range continuity.Needs(a.data.State, a.scopeRoot(), a.now()) {
		if n.Kind == "agent" && a.snoozed[n.ID] != 0 && a.snoozed[n.ID] == a.agentMarker(n.Target) {
			continue
		}
		out = append(out, n)
	}
	return out
}

func age(at, now time.Time) string { return continuity.Age(at, now) }
func rootOf(target string) string  { return strings.Split(target, "#")[0] }

// agents returns the live (active) recorded agent observations.
func (a *App) agents() []model.AgentRecord { return continuity.ActiveAgents(a.data.State) }

// agentTask attributes an observed agent to a task via the shared rules.
func (a *App) agentTask(r model.AgentRecord) (string, bool) {
	return continuity.AgentTask(a.data.State, r)
}

// agentMarker is the snooze fingerprint for a blocked-agent row: the latest
// observed state sequence among that task's blocked agents.
func (a *App) agentMarker(target string) int64 {
	var marker int64
	for _, r := range a.agentsForTask(target) {
		if r.Status == "blocked" && r.StateSeq > marker {
			marker = r.StateSeq
		}
	}
	return marker
}

// agentsForTask returns the live agent records attributed to one task.
func (a *App) agentsForTask(target string) []model.AgentRecord {
	out := []model.AgentRecord{}
	for _, r := range a.agents() {
		if t, _ := a.agentTask(r); t == target {
			out = append(out, r)
		}
	}
	return out
}
func (a *App) choose(target string) {
	if target == a.selected {
		return
	}
	a.selected = target
	a.detailOffset = 0
	a.refresh()
}

// selectedNeed returns the highlighted needs-you entry when the needs panel
// is focused.
func (a *App) selectedNeed() *need {
	if a.panel != 0 {
		return nil
	}
	ns := a.needs()
	if len(ns) == 0 || a.needIndex >= len(ns) {
		return nil
	}
	return &ns[a.needIndex]
}

// jumpNeed focuses the pane of a blocked agent through the journaled Herdr
// jump path. The pane identity comes from the recorded observation.
func (a *App) jumpNeed(n *need) {
	pane := ""
	for _, r := range a.agentsForTask(n.Target) {
		if r.Status == "blocked" {
			pane = r.PaneID
			break
		}
	}
	if pane == "" {
		a.message = "No blocked agent pane recorded for " + n.Target
		return
	}
	a.message = "Jumping to " + pane + "…"
	target := n.Target
	a.async("jump", target, 0, func(ctx context.Context) (any, error) {
		body := map[string]any{"version": 1, "id": model.NewID(), "target": target, "pane_id": pane}
		raw, err := a.call(ctx, "POST", "/workspace/herdr/jump", body)
		if err != nil {
			return nil, err
		}
		var r struct {
			Status string `json:"status"`
			Issue  string `json:"issue"`
		}
		if err := json.Unmarshal(raw, &r); err != nil {
			return nil, err
		}
		return r.Status + " " + r.Issue, nil
	})
}

// waitNeed snoozes a blocked-agent need until any agent state in the row
// changes. The store is untouched; this is view state only.
func (a *App) waitNeed(n *need) {
	a.snoozed[n.ID] = a.agentMarker(n.Target)
	a.message = "Waiting · " + n.Target + " reappears when agent state changes"
}

// parkNeed opens an explicit confirmation that moves an unsaved task out of
// the active workflow. Inspection precedes mutation; the journaled update is
// identical to `heimdall update <task> --status backlog`.
func (a *App) parkNeed(n *need) {
	t, ok := a.data.State.Tasks[n.Target]
	if !ok {
		a.message = "Task no longer recorded"
		return
	}
	task := t.Task
	parked := "backlog"
	if !model.Contains(t.Workflow.Statuses, parked) || t.Task.Status == parked {
		a.message = "Task " + n.Target + " has no backlog state to park into"
		return
	}
	task.Status = parked
	d := a.newDialog("confirm", "park · "+n.Target, n.Target)
	d.lines = []line{
		{{"Move " + n.Target + " to " + parked + "?", fg}},
		plain("Enter confirms this explicit status change. Escape cancels."),
	}
	revision := a.data.State.Revision
	raw, _ := json.Marshal(struct {
		Command core.Command `json:"command"`
	}{core.Command{ID: model.NewID(), Op: "update", ExpectedRevision: &revision, Task: &task}})
	d.pending = &savedRequest{1, n.Target, "/commands", raw}
}

// reobserveNeed asks the daemon to reconcile an uncertain workspace operation.
func (a *App) reobserveNeed(n *need) {
	id := strings.TrimPrefix(n.ID, "uncertain:")
	op, ok := a.data.State.WorkspaceOperations[id]
	if !ok {
		a.message = "Operation no longer recorded"
		return
	}
	a.message = "Re-observing " + op.Intent.Kind + " " + op.Intent.Target + "…"
	a.async("reconcile", op.Intent.Target, 0, func(ctx context.Context) (any, error) {
		_, err := a.call(ctx, "POST", "/workspace/operation/reconcile", map[string]any{
			"version": 1, "id": model.NewID(), "target": op.Intent.Target,
			"operation_id": id, "expected_revision": op.Revision,
			"reason": "re-observe uncertain operation"})
		return nil, err
	})
}
func (a *App) reconcile() {
	rows := a.rows()
	found := false
	for _, r := range rows {
		if r.Target == a.selected {
			found = true
			break
		}
	}
	if !found {
		if len(rows) > 0 {
			a.selected = rows[0].Target
		} else {
			a.selected = ""
		}
	}
	ns := a.needs()
	a.needIndex = min(a.needIndex, max(0, len(ns)-1))
}
