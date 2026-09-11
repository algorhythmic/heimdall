package tui

import (
	"context"
	"encoding/json"
	"errors"
	"heimdall/internal/browser"
	"heimdall/internal/conversation"
	"heimdall/internal/model"
	"strings"
	"testing"
	"time"
)

func TestObservedRowsUseExactOwnershipAndExposeGaps(t *testing.T) {
	f := newFixture(t)
	f.a.data.State = observedState(f.a.data.State, f.now)
	lines := f.a.observedLines("alpha")
	text := flattenWorkspaceLines(lines)
	for _, want := range []string{"browser · https://example.test/current", "gap invalid_pointer", "tier       1 attention", "tier       0 existence", "historical", "present", "\\u001b", "\\u202e"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q in %s", want, text)
		}
	}
	if strings.Contains(text, "foreign.example") || strings.Contains(text, "\x1b") || strings.Contains(text, "\u202e") {
		t.Fatal("foreign or unsafe observation rendered", text)
	}
	d := f.a.newDialog("observed", "observed", "alpha")
	d.lines = lines
	f.a.Draw()
	if rendered := f.a.Text(); !strings.Contains(rendered, "invalid_pointer") || strings.Contains(rendered, "\x1b") {
		t.Fatal("rendered observed rows lost gap or escaped control", rendered)
	}
	f.a.modal = nil
	f.a.screen.SetSize(45, 14)
	f.a.Draw()
	if got := len(strings.Split(strings.TrimSuffix(f.a.Text(), "\n"), "\n")); got != 14 {
		t.Fatal("narrow render did not fit", got)
	}
}

func TestActiveReadDialogVerdictsAndUnavailable(t *testing.T) {
	f := newFixture(t)
	responses := []browser.ActiveState{
		{Status: "active", Target: "alpha", Focus: &browser.ActiveFocus{Profile: "profile", Epoch: "epoch", TabID: 3, WindowID: 4, SurfaceID: "surface"}},
		{Status: "unbound", Focus: &browser.ActiveFocus{Profile: "profile", Epoch: "epoch", TabID: 3, WindowID: 4}},
		{Status: "ambiguous", Gaps: []browser.ActiveGap{{Profile: "profile", Reason: "focused_tab_unavailable"}}},
		{Status: "unknown", Gaps: []browser.ActiveGap{{Profile: "profile", Reason: "fresh_focus_unavailable"}}},
		{Status: "none"},
	}
	for _, response := range responses {
		response := response
		called := false
		f.a.call = func(ctx context.Context, method, path string, body any) ([]byte, error) {
			called = method == "GET" && path == "/state?active=1" && body == nil
			return json.Marshal(response)
		}
		f.a.openActive()
		waitResult(t, f.a)
		if !called || f.a.modal.err != "" {
			t.Fatal("active read was not an authenticated state query", response.Status, f.a.modal.err)
		}
		text := flattenWorkspaceLines(f.a.modal.lines)
		if !strings.Contains(text, response.Status) {
			t.Fatal("missing verdict", response.Status, text)
		}
		if response.Status == "active" && (!strings.Contains(text, "task       alpha") || !strings.Contains(text, "locator")) {
			t.Fatal("active dialog omitted task or locator", text)
		}
		f.a.modal = nil
	}
	f.a.call = func(context.Context, string, string, any) ([]byte, error) {
		return nil, errors.New("dial unix: connection refused")
	}
	f.a.openActive()
	waitResult(t, f.a)
	text := flattenWorkspaceLines(f.a.modal.lines)
	if !strings.Contains(text, "daemon_unavailable") || f.a.modal.err == "" {
		t.Fatal("daemon gap was not visible", text, f.a.modal.err)
	}
}

func observedState(st model.State, now time.Time) model.State {
	profile, current, stale := "profile", "epoch-current", "epoch-stale"
	alpha, beta := st.Tasks["alpha"], st.Tasks["beta"]
	alpha.Revision, beta.Revision = 7, 7
	st.Tasks["alpha"], st.Tasks["beta"] = alpha, beta
	st.WorkspaceHeads["alpha"], st.WorkspaceHeads["beta"] = "manifest-alpha", "manifest-beta"
	st.WorkspaceManifests["manifest-alpha"] = model.WorkspaceManifest{ID: "manifest-alpha", Target: "alpha", TaskRevision: 7, Surfaces: []model.DesiredSurface{{ID: "surface-current", Kind: "browser"}, {ID: "surface-gap", Kind: "browser"}, {ID: "surface-stale", Kind: "browser"}}}
	st.WorkspaceManifests["manifest-beta"] = model.WorkspaceManifest{ID: "manifest-beta", Target: "beta", TaskRevision: 7, Surfaces: []model.DesiredSurface{{ID: "surface-foreign", Kind: "browser"}}}
	addAction := func(id, target, manifest, surface, epoch string, tab int) {
		st.Actions[id] = model.ActionRecord{Intent: model.ActionIntent{ID: id, Target: target, TaskRevision: 7, ManifestID: manifest, SurfaceID: surface, Browser: &model.BrowserIntent{Profile: profile, Epoch: epoch, Action: "open"}}, DeliveryID: "delivery-" + id, Report: &model.ActionReport{Status: "succeeded", TabID: tab}}
	}
	addAction("owned-current", "alpha", "manifest-alpha", "surface-current", current, 1)
	addAction("owned-gap", "alpha", "manifest-alpha", "surface-gap", current, 3)
	addAction("owned-stale", "alpha", "manifest-alpha", "surface-stale", stale, 2)
	addAction("foreign", "beta", "manifest-beta", "surface-foreign", current, 4)
	st.Browsers[profile] = model.BrowserProfile{ID: profile, Epoch: current, Paired: true, Tabs: []model.BrowserTab{{ID: 1, WindowID: 10, OwnerID: "owned-current"}, {ID: 3, WindowID: 10, OwnerID: "owned-gap"}, {ID: 4, WindowID: 10, OwnerID: "foreign"}}}
	observation := func(epoch string, tab int, surface, pointer, title, gap string) model.ObservedSurfaceContainer {
		return model.ObservedSurfaceContainer{Present: true, Observation: model.BrowserSurfaceObservation{Profile: profile, Epoch: epoch, TabID: tab, WindowID: 10, SurfaceID: surface, Pointer: pointer, Title: title, ObservedAt: now.Add(-time.Minute), Gap: gap}}
	}
	st.SurfaceContainers = map[string]model.ObservedSurfaceContainer{
		"current": observation(current, 1, "surface-current", "https://example.test/current", "Current \\u001b \\u202e", ""),
		"gap":     observation(current, 3, "", "not-a-url", "Invalid", "invalid_pointer"),
		"stale":   observation(stale, 2, "surface-stale", "https://example.test/stale", "Stale", ""),
		"foreign": observation(current, 4, "surface-foreign", "https://foreign.example/", "Foreign", ""),
	}
	st.ObservedSurfaces = map[string]model.ObservedSurface{
		"surface-current": {ID: "surface-current", Kind: "browser", NormalizedPointer: "https://example.test/current"},
		"surface-stale":   {ID: "surface-stale", Kind: "browser", NormalizedPointer: "https://example.test/stale"},
		"surface-foreign": {ID: "surface-foreign", Kind: "browser", NormalizedPointer: "https://foreign.example/"},
	}
	st.SurfaceFocusSpans = map[string]model.SurfaceFocusSpan{profile: {BrowserFocusSpan: model.BrowserFocusSpan{TabID: 1, WindowID: 10, StartedAt: now.Add(-2 * time.Minute), EndedAt: now.Add(-time.Minute), DurationSeconds: 60}, Profile: profile, Epoch: current}}
	return st
}

func TestConversationAndSensorLines(t *testing.T) {
	f := newFixture(t)
	st := f.a.data.State
	now := f.now.UTC()
	st.Conversations["c1"] = model.Conversation{Started: conversation.Started{ID: "c1", Kind: "claude_code", StartedAt: now.Add(-time.Hour),
		Task: &conversation.TaskRef{Target: "alpha", Revision: 1}},
		CurrentDescription: &conversation.Description{Kind: "recap", Availability: "available", RetainedDigest: "abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234abcd1234"},
		LastObservation:    conversation.Observation{SourceTime: now}}
	st.Conversations["c2"] = model.Conversation{Started: conversation.Started{ID: "c2", Kind: "claude_code", StartedAt: now.Add(-2 * time.Hour), Task: &conversation.TaskRef{Target: "other", Revision: 1}}}
	st.SensorHealth["session:root1"] = model.SensorStatus{Version: 1, Sensor: "session:root1", Status: "degraded", Reason: "inventory_failed", At: f.now.Add(-time.Minute)}
	st.SensorHealth["herdr:x"] = model.SensorStatus{Version: 1, Sensor: "herdr:x", Status: "healthy", At: f.now}
	f.a.data.State = st
	text := flattenWorkspaceLines(f.a.conversationLines("alpha"))
	if !strings.Contains(text, "recap available") || strings.Contains(text, "c2") {
		t.Fatal("conversation lines wrong", text)
	}
	sensors := flattenWorkspaceLines(f.a.sensorLines())
	if !strings.Contains(sensors, "session:root1 degraded") || strings.Contains(sensors, "herdr:x") {
		t.Fatal("sensor lines wrong", sensors)
	}
}
