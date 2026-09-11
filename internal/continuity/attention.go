package continuity

import (
	"fmt"
	"heimdall/internal/model"
	"sort"
	"strings"
	"time"
)

// Need is one surfaced attention item shared by the TUI needs-you panel and
// lightweight status consumers such as a desktop bar. Text is a short
// human-readable summary; identity and kind drive client actions.
type Need struct {
	ID     string    `json:"id"`
	Kind   string    `json:"kind"`
	Target string    `json:"target"`
	Text   string    `json:"text"`
	At     time.Time `json:"at"`
}

func needScope(st model.State, scopeRoot, target string) bool {
	if scopeRoot == "" {
		return true
	}
	return (model.Grant{Target: scopeRoot, Subtree: true}).Contains(st, target)
}

func needTaskOrder(st model.State, scopeRoot string) []string {
	ids := []string{}
	for id := range st.Tasks {
		if needScope(st, scopeRoot, id) {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		x, y := st.Tasks[ids[i]], st.Tasks[ids[j]]
		if x.Task.ResumeBy != y.Task.ResumeBy {
			if x.Task.ResumeBy == "" {
				return false
			}
			if y.Task.ResumeBy == "" {
				return true
			}
			return x.Task.ResumeBy < y.Task.ResumeBy
		}
		xc, yc := st.Checkpoints[st.CheckpointHeads[ids[i]]], st.Checkpoints[st.CheckpointHeads[ids[j]]]
		if !xc.At.Equal(yc.At) {
			return xc.At.After(yc.At)
		}
		return ids[i] < ids[j]
	})
	return ids
}

// ActiveAgents returns the live recorded agent observations.
func ActiveAgents(st model.State) []model.AgentRecord {
	out := []model.AgentRecord{}
	for _, r := range st.AgentHeads {
		if r.Active {
			out = append(out, r)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ContainerKey() < out[j].ContainerKey() })
	return out
}

// AgentTask attributes an observed agent to a task: a session-bound pane wins;
// otherwise a cwd inside a bound resource root names the task without binding.
// The bool reports whether the attribution is an explicit surface binding.
func AgentTask(st model.State, r model.AgentRecord) (string, bool) {
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

// Needs computes every outstanding attention item for a state. scopeRoot
// limits results to a task subtree; empty scopes everything. Client-local
// suppression (snooze) is not handled here.
func Needs(st model.State, scopeRoot string, now time.Time) []Need {
	out := []Need{}
	for id, p := range st.Proposals {
		if p.Status == "pending" && needScope(st, scopeRoot, p.Target) {
			out = append(out, Need{id, "completion", p.Target, "completion proposed", p.CreatedAt})
		}
	}
	for id, p := range st.ProgressProposals {
		s := model.ProgressStatus(st, p)
		if (s == "draft" || s == "reviewed") && needScope(st, scopeRoot, p.Target) {
			out = append(out, Need{id, p.Kind, p.Target, p.Text, p.At})
		}
	}
	for id, c := range st.Captures {
		if !c.Expired && (len(c.Targets) == 0 || model.Contains(c.Targets, "unassigned")) && scopeRoot == "" {
			text := c.Title
			if text == "" {
				text = c.Why
				if text == "" {
					text = c.Pointer
				}
			}
			out = append(out, Need{id, "link", "", text, c.CreatedAt})
		}
	}
	for id, op := range st.WorkspaceOperations {
		if op.Status != "uncertain" || !needScope(st, scopeRoot, op.Intent.Target) {
			continue
		}
		detail := op.LastReason
		if detail == "" {
			detail = "acknowledged, not observed"
		}
		label := op.Intent.Kind + " " + op.Intent.Target
		if len(op.Intent.SurfaceIDs) > 0 {
			label += fmt.Sprintf(" · %d surfaces", len(op.Intent.SurfaceIDs))
		}
		out = append(out, Need{"uncertain:" + id, "uncertain", op.Intent.Target,
			label + " · " + detail + " " + Age(op.UpdatedAt, now), op.UpdatedAt})
	}
	blocked := map[string][]model.AgentRecord{}
	unbound := map[string][]model.AgentRecord{}
	for _, r := range ActiveAgents(st) {
		target, bound := AgentTask(st, r)
		if !needScope(st, scopeRoot, target) {
			continue
		}
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
		out = append(out, Need{"agent:" + target, "agent", target, strings.Join(parts, " ") + " blocked " + Age(oldest, now), oldest})
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
		out = append(out, Need{"unbound:" + target, "unbound", target, fmt.Sprintf("%d agents · %s", len(rs), strings.Join(names, ", ")), oldest})
	}
	for _, id := range needTaskOrder(st, scopeRoot) {
		t := st.Tasks[id]
		if model.Contains(t.Workflow.Success, t.Task.Status) || model.Contains(t.Workflow.Dropped, t.Task.Status) {
			continue
		}
		cp, ok := st.Checkpoints[st.CheckpointHeads[id]]
		if !ok || now.Sub(cp.At) > 5*24*time.Hour {
			text := "no saved progress"
			if ok {
				text = "unsaved " + Age(cp.At, now)
			}
			out = append(out, Need{"unsaved:" + id, "unsaved", id, text, t.CreatedAt})
		}
	}
	rank := func(s string) int {
		switch s {
		case "completion":
			return 0
		case "decision", "artifact":
			return 1
		case "uncertain":
			return 2
		case "agent":
			return 3
		case "unbound":
			return 4
		case "link":
			return 5
		}
		return 6
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

// Age renders a compact relative timestamp shared by TUI and status output.
func Age(at, now time.Time) string {
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
