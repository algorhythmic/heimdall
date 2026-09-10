package browser

import (
	"context"
	"heimdall/internal/model"
	"reflect"
	"strings"
	"testing"
	"time"
)

// ownedWindowState binds one compositor window to every listed task through a
// current reviewed manifest and an active viewport binding, the same exact
// ownership that recovery and action verification use.
func ownedWindowState(taskIDs []string, window model.WindowIdentity, sourceID string) model.State {
	st := model.Empty()
	st.DesktopSources[sourceID] = model.DesktopSource{ID: sourceID, Active: true, Epoch: window.SourceEpoch}
	st.DesktopSourceHead = sourceID
	for _, id := range taskIDs {
		mID, sID, bID := model.NewID(), model.NewID(), model.NewID()
		st.Tasks[id] = model.TaskRecord{Task: model.Task{ID: id, Title: id, Type: "task", Status: "open"}, Revision: 1}
		st.WorkspaceHeads[id] = mID
		st.WorkspaceManifests[mID] = model.WorkspaceManifest{Version: 1, ID: mID, Target: id, TaskRevision: 1, Surfaces: []model.DesiredSurface{{ID: sID, Kind: "terminal", Label: "term"}}}
		st.ViewportHeads[sID] = bID
		w := window
		st.ViewportBindings[bID] = model.ViewportBinding{Version: 1, ID: bID, Target: id, TaskRevision: 1, ManifestID: mID, SurfaceID: sID, Active: true, SourceID: sourceID, Window: &w}
	}
	return st
}

func TestCompositorActiveSelectionUsesExactWindowOwnership(t *testing.T) {
	sourceID := model.NewID()
	window := model.WindowIdentity{SourceEpoch: strings.Repeat("a", 64), StableID: "18000001"}
	st := ownedWindowState([]string{"alpha"}, window, sourceID)
	got := selectCompositorActive(st, &window, nil)
	if got.Status != "active" || got.Target != "alpha" || got.Focus == nil || got.Focus.Source != "hyprland" || got.Focus.Window == nil || *got.Focus.Window != window {
		t.Fatal("owned window not selected", got)
	}
	other := model.WindowIdentity{SourceEpoch: window.SourceEpoch, StableID: "18000002"}
	got = selectCompositorActive(st, &other, nil)
	if got.Status != "unbound" || got.Target != "" || got.Focus == nil || got.Focus.Window == nil || *got.Focus.Window != other {
		t.Fatal("unowned window not reported unbound", got)
	}
	st = ownedWindowState([]string{"alpha", "beta"}, window, sourceID)
	got = selectCompositorActive(st, &window, nil)
	if got.Status != "ambiguous" || got.Target != "" || got.Focus != nil {
		t.Fatal("window owned by two tasks not ambiguous", got)
	}
	// A stale task revision withdraws ownership; focus alone never binds.
	st = ownedWindowState([]string{"alpha"}, window, sourceID)
	r := st.Tasks["alpha"]
	r.Revision = 2
	st.Tasks["alpha"] = r
	if got = selectCompositorActive(st, &window, nil); got.Status != "unbound" {
		t.Fatal("stale manifest still selected a task", got)
	}
	if got = selectCompositorActive(st, nil, nil); got.Status != "none" || got.Focus != nil {
		t.Fatal("absent focused window must report none", got)
	}
}

func TestCompositorFallbackOnlyWhenBrowserFocusIsUnavailable(t *testing.T) {
	ctx := context.Background()
	st := model.Empty()
	deadline := time.Now().Add(time.Second)
	browserActive := ActiveState{Status: "active", Target: "alpha", Focus: &ActiveFocus{Source: "browser", Profile: model.NewID(), TabID: 1, WindowID: 1}, Gaps: []ActiveGap{}}
	got, err := (&Service{}).withCompositor(ctx, st, browserActive, deadline)
	if err != nil || !reflect.DeepEqual(got, browserActive) {
		t.Fatal("fresh browser focus must be returned untouched", got, err)
	}
	got, err = (&Service{}).withCompositor(ctx, st, ActiveState{Status: "none", Gaps: []ActiveGap{}}, deadline)
	if err != nil || got.Status != "unknown" {
		t.Fatal("missing compositor must be unknown", got, err)
	}
	if !reflect.DeepEqual(got.Gaps, []ActiveGap{{Sensor: "browser", Reason: "no_fresh_tab_focus"}, {Sensor: "hyprland", Reason: "compositor_unavailable"}}) {
		t.Fatal("gaps must name both sensors", got.Gaps)
	}
}
