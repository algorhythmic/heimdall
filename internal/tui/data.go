// Package tui is the local, keyboard-first Heimdall workstream interface.
// It uses the existing CLI daemon boundary and never opens the database.
package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/continuity"
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
}
type row struct {
	Target string
	Depth  int
	Step   bool
}
type need struct {
	ID, Kind, Target, Text string
	At                     time.Time
}

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
func (a *App) needs() []need {
	st := a.data.State
	out := []need{}
	for id, p := range st.Proposals {
		if p.Status == "pending" && a.inScope(p.Target) {
			out = append(out, need{id, "completion", p.Target, "completion proposed", p.CreatedAt})
		}
	}
	for id, p := range st.ProgressProposals {
		s := model.ProgressStatus(st, p)
		if (s == "draft" || s == "reviewed") && a.inScope(p.Target) {
			out = append(out, need{id, p.Kind, p.Target, p.Text, p.At})
		}
	}
	for id, c := range st.Captures {
		if !c.Expired && (len(c.Targets) == 0 || model.Contains(c.Targets, "unassigned")) && a.opts.Target == "" {
			text := c.Title
			if text == "" {
				text = c.Why
				if text == "" {
					text = c.Pointer
				}
			}
			out = append(out, need{id, "link", "", text, c.CreatedAt})
		}
	}
	blocked := map[string][]model.AgentRecord{}
	unbound := map[string][]model.AgentRecord{}
	for _, r := range a.agents() {
		target, bound := a.agentTask(r)
		if r.Status == "blocked" && target != "" {
			blocked[target] = append(blocked[target], r)
		}
		if target != "" && !bound {
			unbound[target] = append(unbound[target], r)
		}
	}
	for target, rs := range blocked {
		oldest := rs[0].At
		kinds := map[string]int{}
		for _, r := range rs {
			kinds[r.Agent]++
			if r.At.Before(oldest) {
				oldest = r.At
			}
		}
		parts := []string{}
		for kind, n := range kinds {
			parts = append(parts, fmt.Sprintf("%d %s", n, kind))
		}
		sort.Strings(parts)
		out = append(out, need{"agent:" + target, "agent", target, strings.Join(parts, " ") + " blocked " + age(oldest, a.now()), oldest})
	}
	for target, rs := range unbound {
		names := []string{}
		oldest := rs[0].At
		for _, r := range rs {
			names = append(names, r.PaneID)
			if r.At.Before(oldest) {
				oldest = r.At
			}
		}
		sort.Strings(names)
		out = append(out, need{"unbound:" + target, "unbound", target, fmt.Sprintf("%d agents · %s", len(rs), strings.Join(names, ", ")), oldest})
	}
	for _, id := range a.taskOrder() {
		t := st.Tasks[id]
		if model.Contains(t.Workflow.Success, t.Task.Status) || model.Contains(t.Workflow.Dropped, t.Task.Status) {
			continue
		}
		cp, ok := st.Checkpoints[st.CheckpointHeads[id]]
		if !ok || a.now().Sub(cp.At) > 5*24*time.Hour {
			text := "no saved progress"
			if ok {
				text = "unsaved " + age(cp.At, a.now())
			}
			out = append(out, need{"unsaved:" + id, "unsaved", id, text, t.CreatedAt})
		}
	}
	rank := func(s string) int {
		switch s {
		case "completion":
			return 0
		case "decision", "artifact":
			return 1
		case "agent":
			return 2
		case "unbound":
			return 3
		case "link":
			return 4
		}
		return 5
	}
	sort.Slice(out, func(i, j int) bool {
		if rank(out[i].Kind) != rank(out[j].Kind) {
			return rank(out[i].Kind) < rank(out[j].Kind)
		}
		if !out[i].At.Equal(out[j].At) {
			return out[i].At.Before(out[j].At)
		}
		return out[i].ID < out[j].ID
	})
	return out
}
func age(at, now time.Time) string {
	if at.IsZero() {
		return "never"
	}
	d := now.Sub(at)
	if d < 0 {
		return "clock?"
	}
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
func rootOf(target string) string { return strings.Split(target, "#")[0] }

// agents returns the live (active) recorded agent observations.
func (a *App) agents() []model.AgentRecord {
	out := []model.AgentRecord{}
	for _, r := range a.data.State.AgentHeads {
		if r.Active {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ContainerKey() < out[j].ContainerKey() })
	return out
}

// agentTask attributes an observed agent to a task: a session-bound pane wins;
// otherwise a cwd inside a bound resource root names the task without binding.
// The bool reports whether the attribution is an explicit surface binding.
func (a *App) agentTask(r model.AgentRecord) (string, bool) {
	st := a.data.State
	for surface, head := range st.SessionHeads {
		b := st.SessionBindings[head]
		if !b.Active || b.Locator == nil || st.SessionHeads[surface] != head || b.SurfaceID != surface {
			continue
		}
		if b.Locator.Host+"|"+b.Locator.SourceEpoch+"|"+b.Locator.SessionID+"|"+b.Locator.PaneID == r.ContainerKey() {
			return b.Target, true
		}
	}
	match := ""
	for _, res := range st.Resources {
		if !res.Active || res.Kind != "tree" || res.Root == "" {
			continue
		}
		if r.Cwd == res.Root || strings.HasPrefix(r.Cwd, res.Root+"/") {
			if match != "" && match != res.Target {
				return match, false
			}
			match = res.Target
		}
	}
	return match, false
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
