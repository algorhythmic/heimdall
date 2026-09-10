package browser

import (
	"context"
	"heimdall/internal/model"
	"heimdall/internal/surface"
	"sort"
	"time"
)

type ActiveFocus struct {
	Source                string                `json:"source,omitempty"`
	Profile               string                `json:"profile"`
	Epoch                 string                `json:"epoch"`
	Sequence              int64                 `json:"sequence"`
	TabID                 int                   `json:"tab_id"`
	WindowID              int                   `json:"window_id"`
	SurfaceID             string                `json:"surface_id,omitempty"`
	Window                *model.WindowIdentity `json:"window,omitempty"`
	CompositorWorkspaceID int                   `json:"compositor_workspace_id,omitempty"`
}
type ActiveGap struct {
	Profile string `json:"profile"`
	Sensor  string `json:"sensor,omitempty"`
	Reason  string `json:"reason"`
}
type ActiveState struct {
	Status string       `json:"status"`
	Target string       `json:"target,omitempty"`
	Focus  *ActiveFocus `json:"focus,omitempty"`
	Gaps   []ActiveGap  `json:"gaps"`
}

// Active asks for bounded fresh readback using the existing observation-only
// challenge protocol. Persisted timestamps alone can never select an active task.
func (s *Service) Active(ctx context.Context) (ActiveState, error) {
	st, err := s.Store.State(ctx)
	if err != nil {
		return ActiveState{}, err
	}
	deadline := time.Now().Add(4 * time.Second)
	if s.Runtime == nil {
		return s.withCompositor(ctx, st, selectActive(st, nil), deadline)
	}
	rt := s.Runtime
	rt.mu.Lock()
	now := rt.Clock()
	if rt.observations == nil {
		rt.observations = map[string]observationDemand{}
	}
	for id, d := range rt.observations {
		if elapsed := now.Sub(d.Started); elapsed < 0 || elapsed >= 5*time.Second {
			delete(rt.observations, id)
		}
	}
	requested := map[string]bool{}
	ids := []string{}
	for id := range st.Browsers {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		p := st.Browsers[id]
		if !p.Paired || p.VerificationProtocol != 1 || s.fresh(p, 0, now) {
			continue
		}
		d, exists := rt.observations[id]
		if !exists && len(rt.observations) >= 128 {
			continue
		}
		if d.Epoch != p.Epoch || d.Connection != p.Connection {
			d = observationDemand{}
		}
		// Preserve workspace/action references from a concurrent observation demand.
		d.Epoch, d.Connection, d.Started = p.Epoch, p.Connection, now
		if d.After < st.LastEventID {
			d.After = st.LastEventID
		}
		rt.observations[id] = d
		requested[id] = true
	}
	rt.mu.Unlock()
	wait, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()
	for {
		st, err = s.Store.State(ctx)
		if err != nil {
			return ActiveState{}, err
		}
		rt.mu.Lock()
		fresh := map[string]bool{}
		ready := true
		for id, p := range st.Browsers {
			fresh[id] = s.fresh(p, 0, rt.Clock())
			if requested[id] && !fresh[id] {
				ready = false
			}
		}
		rt.mu.Unlock()
		result := selectActive(st, fresh)
		if ready {
			return s.withCompositor(ctx, st, result, deadline)
		}
		select {
		case <-ctx.Done():
			return ActiveState{}, ctx.Err()
		case <-wait.Done():
			return s.withCompositor(ctx, st, result, deadline)
		case <-ticker.C:
		}
	}
}

func selectActive(st model.State, fresh map[string]bool) ActiveState {
	result := ActiveState{Status: "unknown", Gaps: []ActiveGap{}}
	profiles := []string{}
	for id := range st.Browsers {
		profiles = append(profiles, id)
	}
	sort.Strings(profiles)
	candidates := []ActiveFocus{}
	owners := []string{}
	ambiguous := false
	observed := false
	for _, id := range profiles {
		p := st.Browsers[id]
		if !p.Paired {
			continue
		}
		reason := ""
		if p.VerificationProtocol != 1 {
			reason = "readback_unsupported"
		} else if !fresh[id] {
			reason = "fresh_focus_unavailable"
		}
		if reason != "" {
			result.Gaps = append(result.Gaps, ActiveGap{Profile: id, Reason: reason})
			continue
		}
		observed = true
		if p.FocusedWindow < 1 {
			continue
		}
		tabs := []model.BrowserTab{}
		for _, tab := range p.Tabs {
			if tab.Active && tab.WindowID == p.FocusedWindow {
				tabs = append(tabs, tab)
			}
		}
		if len(tabs) != 1 || tabs[0].NavigationPending {
			result.Gaps = append(result.Gaps, ActiveGap{Profile: id, Reason: "focused_tab_unavailable"})
			ambiguous = true
			continue
		}
		tab := tabs[0]
		f := ActiveFocus{Source: "browser", Profile: id, Epoch: p.Epoch, Sequence: p.LastSequence, TabID: tab.ID, WindowID: tab.WindowID}
		if identity, err := surface.Identify(tab.URL); err == nil {
			f.SurfaceID = identity.ID
		} else {
			result.Gaps = append(result.Gaps, ActiveGap{Profile: id, Reason: "focused_content_unresolved"})
		}
		candidates = append(candidates, f)
		target := ""
		a := st.Actions[tab.OwnerID]
		b := a.Intent.Browser
		m := st.WorkspaceManifests[st.WorkspaceHeads[a.Intent.Target]]
		task, exists := st.Tasks[a.Intent.Target]
		if exists && b != nil && b.Action == "open" && b.Profile == p.ID && b.Epoch == p.Epoch && a.DeliveryID != "" &&
			m.ID != "" && m.ID == a.Intent.ManifestID && m.Target == task.Task.ID && m.TaskRevision == task.Revision && a.Intent.TaskRevision == task.Revision {
			for _, desired := range m.Surfaces {
				if desired.ID == a.Intent.SurfaceID && desired.Kind == "browser" {
					target = task.Task.ID
				}
			}
		}
		owners = append(owners, target)
	}
	if ambiguous || len(candidates) > 1 {
		result.Status = "ambiguous"
		return result
	}
	if len(candidates) == 1 {
		result.Focus = &candidates[0]
		result.Target = owners[0]
		result.Status = "unbound"
		if result.Target != "" {
			result.Status = "active"
		}
	} else if len(result.Gaps) == 0 && observed {
		result.Status = "none"
	}
	return result
}

func (s *Service) withCompositor(ctx context.Context, st model.State, browser ActiveState, deadline time.Time) (ActiveState, error) {
	if browser.Status == "active" || browser.Status == "unbound" || browser.Status == "ambiguous" {
		return browser, nil
	}
	if len(browser.Gaps) == 0 {
		browser.Gaps = append(browser.Gaps, ActiveGap{Sensor: "browser", Reason: "no_fresh_tab_focus"})
	}
	if s.Compositor == nil {
		browser.Gaps = append(browser.Gaps, ActiveGap{Sensor: "hyprland", Reason: "compositor_unavailable"})
		browser.Status = "unknown"
		return browser, nil
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		browser.Gaps = append(browser.Gaps, ActiveGap{Sensor: "hyprland", Reason: "four_second_budget_exhausted"})
		return browser, nil
	}
	read, cancel := context.WithTimeout(ctx, remaining)
	defer cancel()
	status, err := s.Compositor.ReadFocused(read)
	if err != nil || !status.Fresh || !status.FocusKnown {
		browser.Gaps = append(browser.Gaps, ActiveGap{Sensor: "hyprland", Reason: "fresh_focus_unavailable"})
		browser.Status = "unknown"
		return browser, nil
	}
	result := selectCompositorActive(st, status.FocusedWindow, status.Snapshot)
	result.Gaps = append(browser.Gaps, result.Gaps...)
	return result, nil
}
func selectCompositorActive(st model.State, focused *model.WindowIdentity, snapshot *model.DesktopSnapshot) ActiveState {
	result := ActiveState{Status: "none", Gaps: []ActiveGap{}}
	if focused == nil {
		return result
	}
	workspaceID := 0
	if snapshot != nil {
		for _, w := range snapshot.Windows {
			if w.Identity == *focused {
				workspaceID = w.WorkspaceID
				break
			}
		}
	}
	f := &ActiveFocus{Source: "hyprland", Window: focused, CompositorWorkspaceID: workspaceID}
	targets := map[string]bool{}
	for target := range st.Tasks {
		for _, owned := range model.OwnedWindows(st, target) {
			if owned.Window == *focused {
				targets[target] = true
			}
		}
	}
	if len(targets) == 0 {
		result.Status, result.Focus = "unbound", f
		return result
	}
	if len(targets) > 1 {
		result.Status, result.Focus = "ambiguous", nil
		return result
	}
	for target := range targets {
		result.Status, result.Target, result.Focus = "active", target, f
	}
	return result
}
