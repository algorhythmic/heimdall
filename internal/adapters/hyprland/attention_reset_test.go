package hyprland

import (
	"strings"
	"testing"
	"time"

	"heimdall/internal/model"
)

func TestAttentionReconfigureDiscardsOpenSpan(t *testing.T) {
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
	// Source reselection (the same path connect/failure use) discards the open
	// interval without inventing a blur; the next switch starts fresh.
	reselected := model.DesktopSource{ID: model.NewID(), Active: true, Epoch: source.Epoch}
	o.Configure(reselected)
	o.mu.Lock()
	o.status.Snapshot = snapshot
	o.mu.Unlock()
	if got := o.attentionFrameLocked("activewindowv2", second.StableID, 2, now.Add(10*time.Second)); got != nil {
		t.Fatal("discarded span emitted after reconfigure", got)
	}
	if got := o.attentionFrameLocked("activewindowv2", first.StableID, 3, now.Add(14*time.Second)); got == nil || got.Surface == nil || got.Surface.Window != second || got.Surface.DurationSeconds != 4 {
		t.Fatal("fresh span after reconfigure missing", got)
	}
	// An unknown window (not in the last inventory) closes the open span and
	// opens nothing.
	if got := o.attentionFrameLocked("activewindowv2", "18000009", 4, now.Add(17*time.Second)); got == nil || got.Surface == nil || got.Surface.Window != first {
		t.Fatal("switch to unknown window did not close the open span", got)
	}
	if got := o.attentionFrameLocked("activewindowv2", first.StableID, 5, now.Add(20*time.Second)); got != nil {
		t.Fatal("unknown window credited with a span", got)
	}
}
