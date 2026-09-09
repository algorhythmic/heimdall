package workspace

import (
	"encoding/json"
	"heimdall/internal/adapters/hyprland"
	"heimdall/internal/model"
	"heimdall/internal/testdesktop"
	"os"
	"reflect"
	"strings"
	"testing"
)

func viewportSetup(t *testing.T) (*fixture, *ViewportService, *testdesktop.Fake, ViewportRequest) {
	t.Helper()
	f := setup(t)
	fake := testdesktop.New()
	observer := hyprland.New()
	observer.Connector = fake.Connect
	t.Cleanup(observer.Close)
	s := &ViewportService{Store: f.e.Store, Observer: observer}
	r := ViewportRequest{Version: 1, ID: model.NewID(), Op: "select", Previous: "none", Source: &SourceInput{SocketDir: "/synthetic/hypr", Host: strings.Repeat("c", 64), Epoch: strings.Repeat("a", 64)}}
	if _, err := s.Execute(f.ctx, r, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	return f, s, fake, r
}
func viewportRequest(t *testing.T, f *fixture, s *ViewportService, m model.WorkspaceManifest, source string) ViewportRequest {
	t.Helper()
	v, err := s.Observer.Read(f.ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	return ViewportRequest{Version: 1, ID: model.NewID(), Op: "bind", Previous: "none", Target: m.Target, ExpectedTaskRevision: m.TaskRevision, Binding: &ViewportInput{ManifestID: m.ID, SurfaceID: m.Surfaces[0].ID, SourceID: source, SnapshotID: v.Snapshot.ID, Window: &v.Snapshot.Windows[0].Identity}}
}
func TestViewportExplicitPairingLifecycleAndReplay(t *testing.T) {
	f, s, fake, source := viewportSetup(t)
	_, a := f.manifest("alpha")
	_, b := f.manifest("beta")
	session := f.binding(a)
	f.send(session)
	r := viewportRequest(t, f, s, a, source.ID)
	r.Binding.SessionBindingID = session.ID
	accepted, err := s.Execute(f.ctx, r, "cli", f.now)
	if err != nil {
		t.Fatal(err)
	}
	reject := func(request ViewportRequest) {
		t.Helper()
		before, _ := f.e.Store.State(f.ctx)
		if _, err := s.Execute(f.ctx, request, "cli", f.now); err == nil {
			t.Fatal("invalid binding accepted", request)
		}
		after, _ := f.e.Store.State(f.ctx)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("refused binding changed state")
		}
	}
	other := viewportRequest(t, f, s, b, source.ID)
	reject(other) // same native identity, distinct task
	v, err := s.View(f.ctx, "alpha", true)
	if err != nil || v.Surfaces[0].Status != "observed" || v.Surfaces[0].Session.ID != session.ID || !model.Contains(v.Surfaces[0].Issues, "terminal_pane_attachment_not_verified") {
		t.Fatal(v, err)
	}
	v, err = s.View(f.ctx, "beta", false)
	if err != nil || v.Surfaces[0].Window != nil || v.Surfaces[0].Status != "unowned" {
		t.Fatal("unmatched duplicate title gained ownership", v, err)
	}
	// A pane declaration may move independently of the outer window; the old
	// join becomes visibly stale instead of silently following another pane.
	moved := model.Clone(session)
	moved.ID = model.NewID()
	moved.Session.Previous = session.ID
	moved.Session.Locator.PaneID = "pane-two"
	f.send(moved)
	v, _ = s.View(f.ctx, "alpha", false)
	if !model.Contains(v.Surfaces[0].Issues, "session_binding_changed") || v.Surfaces[0].Session != nil {
		t.Fatal("pane move retained current attachment claim")
	}
	stale := model.Clone(r)
	stale.ID = model.NewID()
	stale.Previous = r.ID
	reject(stale)
	fake.Set("clients", []map[string]any{testdesktop.Window("18000001", "0x64", 2, "review"), testdesktop.Window("18000002", "0x65", 1, "planning")})
	v, _ = s.View(f.ctx, "alpha", true)
	if v.Surfaces[0].Window.WorkspaceID != 2 {
		t.Fatal("window move not reconciled")
	}
	stale = viewportRequest(t, f, s, b, source.ID)
	fake.Windows("18000009")
	reject(stale)
	v, _ = s.View(f.ctx, "alpha", true)
	if v.Surfaces[0].Status != "missing" || v.Surfaces[0].Window != nil {
		t.Fatal("reused address inherited ownership")
	}
	// Exact retry is historical and must not probe or rebind a replacement.
	beforeReads, _ := fake.Count()
	retry, err := s.Execute(f.ctx, r, "cli", f.now)
	afterReads, _ := fake.Count()
	if err != nil || string(retry) != string(accepted) || beforeReads != afterReads {
		t.Fatal("retry was not inert", err)
	}
	unbind := model.Clone(r)
	unbind.ID = model.NewID()
	unbind.Op = "unbind"
	unbind.Previous = r.ID
	unbind.Binding = &ViewportInput{ManifestID: a.ID, SurfaceID: a.Surfaces[0].ID}
	if _, err = s.Execute(f.ctx, unbind, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	stop := ViewportRequest{Version: 1, ID: model.NewID(), Op: "stop", Previous: source.ID}
	if _, err = s.Execute(f.ctx, stop, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	state, _ := f.e.Store.State(f.ctx)
	beforeReads, _ = fake.Count()
	replay, err := f.e.Store.Replay(f.ctx)
	afterReads, _ = fake.Count()
	if err != nil || !reflect.DeepEqual(state, replay) || beforeReads != afterReads {
		t.Fatal("replay dispatched reads or changed state", err)
	}
	retry, err = s.Execute(f.ctx, r, "cli", f.now)
	if err != nil || string(retry) != string(accepted) {
		t.Fatal("receipt changed after stop/replay", err)
	}
	if _, err = s.Execute(f.ctx, r, "browser", f.now); err == nil {
		t.Fatal("browser granted desktop authority")
	}
}
func TestViewportBrowserGateManifestRemovalAndSourceRestart(t *testing.T) {
	f, s, fake, source := viewportSetup(t)
	mr, m := f.manifest("alpha")
	r := viewportRequest(t, f, s, m, source.ID)
	if _, err := s.Execute(f.ctx, r, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	// Even a generic session-free window cannot be removed from desired
	// membership while its explicit viewport ownership remains active.
	mr.ID = model.NewID()
	mr.Manifest.Previous = m.ID
	mr.Manifest.Surfaces = []model.DesiredSurface{}
	f.reject(mr)
	br := f.request("workspace.accept", "beta")
	br.Manifest = &ManifestInput{Previous: "none", Name: "Browser", Surfaces: []model.DesiredSurface{{ID: model.NewID(), Kind: "browser", Label: "Browser", RestorePolicy: "manual"}}}
	var bm model.WorkspaceManifest
	json.Unmarshal(f.send(br), &bm)
	bindBrowser := viewportRequest(t, f, s, bm, source.ID)
	live, _ := s.Observer.Read(f.ctx, true)
	bindBrowser.Binding.Window = &live.Snapshot.Windows[1].Identity
	if _, err := s.Execute(f.ctx, bindBrowser, "cli", f.now); err == nil || !strings.Contains(err.Error(), "C13") {
		t.Fatal("browser title pairing accepted", err)
	}
	fake.ChangeEpoch()
	fake.Disconnect()
	v, _ := s.View(f.ctx, "alpha", true)
	if v.Fresh || v.Surfaces[0].Window != nil || v.Surfaces[0].Status != "unavailable" {
		t.Fatal("source restart inherited old ownership")
	}
	next := model.Clone(source)
	next.ID = model.NewID()
	next.Previous = source.ID
	next.Source.Epoch = strings.Repeat("b", 64)
	if _, err := s.Execute(f.ctx, next, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	v, _ = s.View(f.ctx, "alpha", true)
	if !v.Fresh || v.Surfaces[0].Window != nil {
		t.Fatal("new source adopted old stable ID")
	}
	// Restarting just the daemon on the same compositor retains identity after
	// a new subscription/bootstrap; no title matching or task ownership edits.
	s.Observer.Close()
	if err := s.Restore(f.ctx); err != nil {
		t.Fatal(err)
	}
	v, _ = s.View(f.ctx, "alpha", true)
	if !v.Fresh || v.Surfaces[0].Window != nil {
		t.Fatal("restart selection mismatch")
	}
}
func TestViewportRequestStrictness(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/viewport/requests-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var requests []json.RawMessage
	if err = json.Unmarshal(raw, &requests); err != nil {
		t.Fatal(err)
	}
	for _, r := range requests {
		if _, err := DecodeViewport(r); err != nil {
			t.Fatal("frozen request refused", err)
		}
	}
	for _, raw := range []string{`{"version":1,"id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","op":"stop","previous":"none","unknown":true}`, `{"version":2,"id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","op":"stop","previous":"none"}`, `{"version":1,"id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","op":"stop","previous":""}`} {
		if _, err := DecodeViewport([]byte(raw)); err == nil {
			t.Fatal("bad envelope accepted")
		}
	}
}
