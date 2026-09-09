package workspace

import (
	"encoding/json"
	"heimdall/internal/adapters/hyprland"
	"heimdall/internal/core"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"heimdall/internal/testdesktop"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestPreviewHerdrReadbackAndSourceReselection(t *testing.T) {
	f, herdr, adapter, bind := setupHerdr(t)
	if _, err := herdr.Bind(f.ctx, bind, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	fake := testdesktop.New()
	observer := hyprland.New()
	observer.Connector = fake.Connect
	t.Cleanup(observer.Close)
	viewport := &ViewportService{Store: f.e.Store, Observer: observer}
	source := ViewportRequest{Version: 1, ID: model.NewID(), Op: "select", Previous: "none", Source: &SourceInput{SocketDir: "/synthetic/hypr", Host: strings.Repeat("c", 64), Epoch: strings.Repeat("a", 64)}}
	if _, err := viewport.Execute(f.ctx, source, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	st, _ := f.e.Store.State(f.ctx)
	r := viewportRequest(t, f, viewport, st.WorkspaceManifests[bind.ManifestID], source.ID)
	r.Binding.SessionBindingID = bind.ID
	if _, err := viewport.Execute(f.ctx, r, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	point := capturePoint(t, f, &SnapshotService{Store: f.e.Store, Observer: observer})
	f.now = time.Now().UTC()
	s := &PreviewService{Store: f.e.Store, Observer: observer, Herdr: adapter}
	request := PreviewRequest{Version: 1, Target: "alpha", ManifestID: bind.ManifestID, SnapshotID: point.ID}
	fake.Windows()
	v := mustPreview(t, f, s, request)
	if v.Surfaces[0].Disposition != "reattach" || v.Surfaces[0].SessionStatus != "current" || adapter.reports != 0 {
		t.Fatal(v.Surfaces, adapter.reports)
	}
	fake.ChangeEpoch()
	v = mustPreview(t, f, s, request)
	if v.Fresh {
		t.Fatal("compositor source silently reselected")
	}
	next := model.Clone(source)
	next.ID = model.NewID()
	next.Previous = source.ID
	next.Source.Epoch = strings.Repeat("b", 64)
	if _, err := viewport.Execute(f.ctx, next, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	v = mustPreview(t, f, s, request)
	if !v.Fresh || v.Surfaces[0].Disposition != "reattach" || !model.Contains(v.Issues, "saved_source_epoch_changed") {
		t.Fatal(v)
	}
	adapter.observation.Herdr.TerminalID = "reused-pane-new-terminal"
	v = mustPreview(t, f, s, request)
	if v.Surfaces[0].Disposition != "review-required" || v.Surfaces[0].SessionStatus == "current" {
		t.Fatal("changed session retained attachment proposal", v.Surfaces)
	}
}

func TestPreviewDecoderRejectsUnknownAuthorityAndNullPolicy(t *testing.T) {
	raw := []byte(`{"version":1,"target":"alpha","manifest_id":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa","snapshot_id":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}`)
	if _, err := DecodePreview(raw); err != nil {
		t.Fatal(err)
	}
	for _, extra := range []string{`,"actor":"cli"`, `,"actions":[]`, `,"max_snapshot_age_seconds":null`, `,"max_snapshot_age_seconds":-1`} {
		if _, err := DecodePreview(append(append([]byte{}, raw[:len(raw)-1]...), []byte(extra+"}")...)); err == nil {
			t.Fatal("accepted unknown authority/policy", extra)
		}
	}
}

func previewSetup(t *testing.T) (*fixture, *PreviewService, *SnapshotService, *ViewportService, *testdesktop.Fake, PreviewRequest) {
	t.Helper()
	f, viewport, fake, source := viewportSetup(t)
	_, m := f.manifest("alpha")
	if _, err := viewport.Execute(f.ctx, viewportRequest(t, f, viewport, m, source.ID), "cli", f.now); err != nil {
		t.Fatal(err)
	}
	snapshots := &SnapshotService{Store: f.e.Store, Observer: viewport.Observer}
	point := capturePoint(t, f, snapshots)
	// The fixture's durable command time predates the live observer clock.
	f.now = time.Now().UTC()
	return f, &PreviewService{Store: f.e.Store, Observer: viewport.Observer}, snapshots, viewport, fake, PreviewRequest{Version: 1, Target: "alpha", ManifestID: m.ID, SnapshotID: point.ID}
}
func mustPreview(t *testing.T, f *fixture, s *PreviewService, r PreviewRequest) Preview {
	t.Helper()
	v, err := s.Build(f.ctx, r, f.now)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestPreviewDeterministicReadOnlyScopeAndStaleness(t *testing.T) {
	f, s, _, _, fake, r := previewSetup(t)
	before, _ := f.e.Store.State(f.ctx)
	events, _ := f.e.Store.Events(f.ctx)
	v := mustPreview(t, f, s, r)
	if !v.Fresh || v.ReviewRequired || v.Surfaces[0].Disposition != "leave-open" || v.UnownedLeftOpen != 1 {
		t.Fatalf("%+v", v)
	}
	list, err := s.List(f.ctx, "alpha", f.now)
	if err != nil || len(list.Surfaces) != 1 || list.Surfaces[0].Status != "observed" || len(list.Issues) != 0 {
		t.Fatal(list, err)
	}
	check, err := s.Validate(f.ctx, v, f.now.Add(time.Second))
	if err != nil || !check.Current || check.Preview.Digest != v.Digest {
		t.Fatal(check, err)
	}
	// Even foreign/unowned titles and PIDs changing do not change scoped plans.
	other := testdesktop.Window("18000002", "0x65", 1, "planning")
	other["title"], other["pid"], other["class"] = "FOREIGN-SECRET", 456, "secret-app"
	fake.Set("clients", []map[string]any{testdesktop.Window("18000001", "0x64", 1, "planning"), other})
	unchanged := mustPreview(t, f, s, r)
	raw, _ := json.Marshal(unchanged)
	if unchanged.Digest != v.Digest || strings.Contains(string(raw), "SECRET") || strings.Contains(string(raw), "Same title") || strings.Contains(string(raw), "pid") {
		t.Fatal("unscoped or descriptive identity entered plan", string(raw))
	}
	after, _ := f.e.Store.State(f.ctx)
	afterEvents, _ := f.e.Store.Events(f.ctx)
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(events, afterEvents) {
		t.Fatal("preview/list/validate changed durable state")
	}
	fake.Set("clients", []map[string]any{testdesktop.Window("18000001", "0x64", 2, "review"), other})
	moved := mustPreview(t, f, s, r)
	if moved.Surfaces[0].Disposition != "move" || !model.Contains(moved.Surfaces[0].Changes, "workspace_changed") {
		t.Fatal(moved.Surfaces)
	}
	check, err = s.Validate(f.ctx, v, f.now.Add(2*time.Second))
	if err != nil || check.Current || check.Issue != "preview_changed" {
		t.Fatal(check, err)
	}
	fake.Windows("18000009") // address reused; title identical, stable identity new
	missing := mustPreview(t, f, s, r)
	if missing.Surfaces[0].Disposition != "launch" || missing.Surfaces[0].Window != nil || len(missing.Surfaces[0].Requirements) == 0 {
		t.Fatal(missing.Surfaces)
	}
	r.Target = "beta"
	if _, err := s.Build(f.ctx, r, f.now); err == nil {
		t.Fatal("cross-task manifest/snapshot accepted")
	}
}

func TestPreviewArtifactSealExpiryAndRestart(t *testing.T) {
	f, s, _, _, _, r := previewSetup(t)
	v := mustPreview(t, f, s, r)
	for _, edit := range []func(*Preview){func(p *Preview) { p.ExpiresAt = p.ExpiresAt.Add(time.Hour) }, func(p *Preview) { p.Surfaces[0].Disposition = "launch" }, func(p *Preview) { p.ReviewRequired = true }, func(p *Preview) { p.Request.Target = "beta" }} {
		forged := model.Clone(v)
		edit(&forged)
		result, err := s.Validate(f.ctx, forged, f.now)
		if err != nil || result.Current || result.Issue != "preview_not_issued_by_this_daemon" {
			t.Fatal(result, err)
		}
	}
	for _, now := range []time.Time{f.now.Add(-time.Second), v.ExpiresAt} {
		result, _ := s.Validate(f.ctx, v, now)
		if result.Current || result.Issue != "preview_expired" {
			t.Fatal(result)
		}
	}
	restarted := &PreviewService{Store: f.e.Store, Observer: s.Observer}
	result, _ := restarted.Validate(f.ctx, v, f.now)
	if result.Current || result.Issue != "preview_not_issued_by_this_daemon" {
		t.Fatal(result)
	}
	if _, err := s.Build(f.ctx, PreviewRequest{}, f.now); err == nil {
		t.Fatal("missing immutable selections accepted")
	}
}

func TestPreviewDisplaySourceCoverageAndTiledLimits(t *testing.T) {
	f, s, _, _, fake, r := previewSetup(t)
	original := mustPreview(t, f, s, r)
	w := testdesktop.Window("18000001", "0x64", 1, "planning")
	w["at"] = []int{50, 20}
	fake.Set("clients", []map[string]any{w})
	v := mustPreview(t, f, s, r)
	if v.Surfaces[0].Disposition != "review-required" || !model.Contains(v.Surfaces[0].Issues, "tiled_split_geometry_not_restorable") {
		t.Fatal(v.Surfaces)
	}
	fake.Windows("18000001", "18000002")
	for _, scale := range []float64{1.25, 1.5} {
		fake.Set("monitors", []map[string]any{{"id": 0, "name": "SYNTHETIC-1", "x": 0, "y": 0, "width": 1920, "height": 1080, "scale": scale, "transform": 0, "activeWorkspace": map[string]any{"id": 1}}})
		v = mustPreview(t, f, s, r)
		if v.Surfaces[0].Disposition != "review-required" || !model.Contains(v.Surfaces[0].Issues, "display_geometry_changed") || v.Digest == original.Digest {
			t.Fatal(v.Surfaces)
		}
		original = v
	}
	fake.Set("clients", []map[string]any{testdesktop.Window("18000001", "0x64", 99, "unknown")})
	v = mustPreview(t, f, s, r)
	if v.Fresh || v.Surfaces[0].Window != nil || v.Surfaces[0].Disposition != "unavailable" {
		t.Fatal("inconsistent source produced observed plan", v)
	}
	fake.ChangeEpoch()
	v = mustPreview(t, f, s, r)
	if v.Fresh || !v.ReviewRequired {
		t.Fatal("source restart adopted automatically")
	}
}

func TestPreviewRetentionAndConcurrentInputChanges(t *testing.T) {
	f, s, snapshots, _, fake, r := previewSetup(t)
	first := mustPreview(t, f, s, r)
	second := capturePoint(t, f, snapshots)
	_, err := snapshots.Execute(f.ctx, SnapshotRequest{Version: 1, ID: model.NewID(), Op: "unpin", Target: "alpha", Previous: r.SnapshotID, SnapshotID: r.SnapshotID, Reason: "Release old point"}, "cli", f.now)
	if err != nil {
		t.Fatal(err)
	}
	v := mustPreview(t, f, s, r)
	if v.SnapshotProtected || !v.ReviewRequired || !model.Contains(v.Issues, "snapshot_not_pinned_or_current") {
		t.Fatal(v)
	}
	_, err = snapshots.Execute(f.ctx, SnapshotRequest{Version: 1, ID: model.NewID(), Op: "prune", Target: "alpha", Previous: "none", SnapshotIDs: []string{r.SnapshotID}, Reason: "Remove old payload"}, "cli", f.now)
	if err != nil {
		t.Fatal(err)
	}
	v = mustPreview(t, f, s, r)
	if !model.Contains(v.Issues, "snapshot_payload_unavailable") || v.Surfaces[0].Disposition != "review-required" {
		t.Fatal(v)
	}
	check, _ := s.Validate(f.ctx, first, f.now)
	if check.Current {
		t.Fatal("pruned preview remained current")
	}
	r.SnapshotID = second.ID
	called := false
	fake.Hook = func(command string) {
		if called {
			return
		}
		called = true
		st, _ := f.e.Store.State(f.ctx)
		task := st.Tasks["alpha"].Task
		task.Title = "Changed during IPC"
		_, err := f.e.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "update", Target: "alpha", Task: &task}, "cli", f.now)
		if err != nil {
			t.Error(err)
		}
	}
	if _, err = s.Build(f.ctx, r, f.now); err == nil {
		t.Fatal("mixed task revision published")
	}
}

func TestPreviewRefusesConcurrentPayloadPruning(t *testing.T) {
	f, s, snapshots, _, fake, r := previewSetup(t)
	capturePoint(t, f, snapshots) // the selected point is now only manually pinned
	called := false
	fake.Hook = func(string) {
		if called {
			return
		}
		called = true
		for _, request := range []SnapshotRequest{
			{Version: 1, ID: model.NewID(), Op: "unpin", Target: "alpha", Previous: r.SnapshotID, SnapshotID: r.SnapshotID, Reason: "Release selected history"},
			{Version: 1, ID: model.NewID(), Op: "prune", Target: "alpha", Previous: "none", SnapshotIDs: []string{r.SnapshotID}, Reason: "Prune during preview observation"},
		} {
			if _, err := snapshots.Execute(f.ctx, request, "cli", f.now); err != nil {
				t.Error(err)
			}
		}
	}
	if _, err := s.Build(f.ctx, r, f.now); err == nil {
		t.Fatal("published a point pruned during observation")
	}
}

func TestPreviewPureOwnershipRemovalAndReattachment(t *testing.T) {
	f, s, _, _, _, r := previewSetup(t)
	st, _ := f.e.Store.State(f.ctx)
	point, _ := f.e.Store.WorkspacePoint(f.ctx, "alpha", r.SnapshotID)
	observed, _ := s.Observer.Read(f.ctx, true)
	surface := point.Payload.Surfaces[0].SurfaceID
	oldBinding := st.ViewportBindings[st.ViewportHeads[surface]]
	delete(st.ViewportHeads, surface)
	other := model.Clone(oldBinding)
	other.Target = "beta"
	other.SurfaceID = model.NewID()
	other.ID = model.NewID()
	st.ViewportBindings[other.ID] = other
	st.ViewportHeads[other.SurfaceID] = other.ID
	v := planPreview(st, r, point, observed, nil, f.now)
	if v.Surfaces[0].Window != nil || v.Surfaces[0].Disposition != "review-required" || !model.Contains(v.Surfaces[0].Issues, "saved_window_owned_elsewhere_left_open") {
		t.Fatal(v.Surfaces)
	}
	raw, _ := json.Marshal(v)
	if strings.Contains(string(raw), "beta") || strings.Contains(string(raw), other.ID) {
		t.Fatal("foreign ownership leaked")
	}
	m := st.WorkspaceManifests[r.ManifestID]
	m.Surfaces = nil
	st.WorkspaceManifests[m.ID] = m
	v = planPreview(st, r, point, observed, nil, f.now)
	if len(v.Surfaces) != 1 || v.Surfaces[0].Membership != "removed" || v.Surfaces[0].Disposition != "leave-open" {
		t.Fatal(v.Surfaces)
	}
	st, _ = f.e.Store.State(f.ctx)
	observed.Snapshot.Windows = nil
	checks := map[string]SessionCheck{surface: {Status: "current"}}
	v = planPreview(st, r, point, observed, checks, f.now)
	if v.Surfaces[0].Disposition != "reattach" || len(v.Surfaces[0].Requirements) != 3 {
		t.Fatal(v.Surfaces)
	}
	checks[surface] = SessionCheck{Status: "stale"}
	v = planPreview(st, r, point, observed, checks, f.now)
	if v.Surfaces[0].Disposition != "launch" {
		t.Fatal(v.Surfaces)
	}
	m = st.WorkspaceManifests[r.ManifestID]
	m.Surfaces[0].Kind = "browser"
	st.WorkspaceManifests[m.ID] = m
	v = planPreview(st, r, store.PointView{}, observed, nil, f.now)
	if v.Surfaces[0].Disposition != "review-required" || !model.Contains(v.Surfaces[0].Issues, "browser_window_pairing_required") {
		t.Fatal(v.Surfaces)
	}
}
