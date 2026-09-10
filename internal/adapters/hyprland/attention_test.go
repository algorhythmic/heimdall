package hyprland

import (
	"strings"
	"testing"
	"time"

	"heimdall/internal/model"
)

func TestAttentionFramesEmitDebouncedSpanAndResetOnGap(t *testing.T) {
	now := time.Now().UTC()
	source := model.DesktopSource{ID: model.NewID(), Active: true, Epoch: strings.Repeat("a", 64)}
	first := model.WindowIdentity{SourceEpoch: source.Epoch, StableID: "18000001"}
	second := model.WindowIdentity{SourceEpoch: source.Epoch, StableID: "18000002"}
	snapshot := &model.DesktopSnapshot{ID: model.NewID(), CapturedAt: now, Windows: []model.DesktopWindow{{Identity: first, Class: "foot", Title: "one", WorkspaceID: 1}, {Identity: second, Class: "foot", Title: "two", WorkspaceID: 1}}, Workspaces: []model.DesktopWorkspace{{ID: 1, Name: "one", MonitorName: "HDMI-A-1"}}}
	o := New()
	o.source, o.status.Snapshot = source, snapshot
	if got := o.attentionFrameLocked("activewindowv2", first.StableID, 1, now); got != nil {
		t.Fatal("opened span emitted")
	}
	if got := o.attentionFrameLocked("activewindowv2", second.StableID, 2, now.Add(time.Second)); got != nil {
		t.Fatal("short span emitted")
	}
	o.attentionFrameLocked("activewindowv2", first.StableID, 3, now.Add(2*time.Second))
	got := o.attentionFrameLocked("activewindowv2", second.StableID, 4, now.Add(5*time.Second))
	if got == nil || got.Surface == nil || got.Surface.Window != first || got.Surface.DurationSeconds != 3 {
		t.Fatal("missing compositor span", got)
	}
	o.resetAttentionLocked()
	if got := o.attentionFrameLocked("activewindowv2", first.StableID, 5, now.Add(6*time.Second)); got != nil {
		t.Fatal("gap fabricated a span")
	}
}

func TestAttentionWorkspaceSpan(t *testing.T) {
	now := time.Now().UTC()
	source := model.DesktopSource{ID: model.NewID(), Active: true, Epoch: strings.Repeat("b", 64)}
	o := New()
	o.source = source
	o.status.Snapshot = &model.DesktopSnapshot{ID: model.NewID(), CapturedAt: now, Workspaces: []model.DesktopWorkspace{{ID: 1, Name: "one", MonitorName: "HDMI-A-1"}, {ID: 2, Name: "two", MonitorName: "HDMI-A-1"}}}
	o.attentionFrameLocked("workspacev2", "1,one", 1, now)
	got := o.attentionFrameLocked("workspacev2", "2,two", 2, now.Add(3*time.Second))
	if got == nil || got.Workspace == nil || got.Workspace.CompositorWorkspaceID != 1 {
		t.Fatal("missing workspace span", got)
	}
}
