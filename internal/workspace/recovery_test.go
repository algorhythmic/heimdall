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
	"sync"
	"testing"
	"time"
)

func verifyRecovery(t *testing.T, f *operationFixture, r RecoveryRequest) RecoveryReport {
	t.Helper()
	v, err := f.service.Previews.Verify(f.ctx, r, time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestRecoveryRejectsConcurrentInputChangeWithoutDispatch(t *testing.T) {
	f := newOperationFixture(t, "alpha")
	var once sync.Once
	f.fake.Hook = func(command string) {
		if command == "clients" {
			once.Do(func() {
				task := f.state().Tasks["alpha"].Task
				task.Title = "Changed during readback"
				_, err := f.e.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "update", Target: "alpha", Task: &task}, "cli", time.Now().UTC())
				if err != nil {
					t.Fatal(err)
				}
			})
		}
	}
	_, err := f.service.Previews.Verify(f.ctx, RecoveryRequest{Version: 1, Target: "alpha", PlacementPolicy: "saved"}, time.Now().UTC())
	if err == nil || len(f.dispatcher.commands) != 0 {
		t.Fatal("mixed-scope report published", err)
	}
}

func TestRecoveryFreshReadOnlyAndWrongWorkspace(t *testing.T) {
	f := newOperationFixture(t, "alpha")
	r := RecoveryRequest{Version: 1, Target: "alpha", PlacementPolicy: "saved"}
	before := f.state()
	events, _ := f.e.Store.Events(f.ctx)
	v := verifyRecovery(t, f, r)
	if !v.Full || v.Outcome != "verified" || !v.Surfaces[0].Full || v.Digest == "" {
		t.Fatalf("%+v\n%+v", v, v.Surfaces)
	}
	afterEvents, _ := f.e.Store.Events(f.ctx)
	if !reflect.DeepEqual(before, f.state()) || !reflect.DeepEqual(events, afterEvents) || len(f.dispatcher.commands) != 0 {
		t.Fatal("verification mutated state or dispatched")
	}
	f.fake.Set("clients", []map[string]any{testdesktop.Window(f.windows["alpha"], "0x64", 2, "review")})
	v = verifyRecovery(t, f, r)
	if v.Full || v.Surfaces[0].WorkspaceMembership.Status != "not_matched" || v.Outcome != "partial" {
		t.Fatal(v)
	}
	f.fake.Windows("18000999")
	v = verifyRecovery(t, f, r)
	if v.Full || v.Surfaces[0].Existence.Status != "not_matched" {
		t.Fatal("lookalike adopted", v)
	}
	f.fake.ChangeEpoch()
	v = verifyRecovery(t, f, r)
	if v.Full || v.Coverage.Status != "unknown" {
		t.Fatal("source loss became success", v)
	}
}

func TestRecoveryCloseMustBeObservedNotJustAcknowledged(t *testing.T) {
	f := newOperationFixture(t, "alpha")
	op := f.queue(f.requestOperation("alpha", "close"))
	f.step() // ACK but the fake application refuses to disappear.
	r := RecoveryRequest{Version: 1, Target: "alpha", OperationID: op.Intent.ID, PlacementPolicy: "saved"}
	v := verifyRecovery(t, f, r)
	if v.Full || v.Surfaces[0].Existence.Status != "not_matched" {
		t.Fatal("ACK implied close", v)
	}
	f.fake.Windows()
	f.step()
	v = verifyRecovery(t, f, r)
	if !v.Full || v.Surfaces[0].Expected != "absent" || len(f.dispatcher.commands) != 1 {
		t.Fatal(v, f.dispatcher.commands)
	}
	f.fake.Windows(f.windows["alpha"])
	v = verifyRecovery(t, f, r)
	if v.Full || v.Surfaces[0].Existence.Status != "not_matched" {
		t.Fatal("old matched result overrode live existence", v)
	}
	r.Target = "beta"
	if _, err := f.service.Previews.Verify(f.ctx, r, time.Now()); err == nil {
		t.Fatal("cross-task operation accepted")
	}
}

func recoveryPlanFixture(t *testing.T) (model.State, store.PointView, RecoveryRequest, []string, hyprland.Status) {
	f := newOperationFixture(t, "alpha")
	st := f.state()
	point, err := f.e.Store.WorkspacePoint(f.ctx, "alpha", st.SnapshotHeads["alpha"].ID)
	if err != nil {
		t.Fatal(err)
	}
	o, err := f.service.Observer.Read(f.ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	ids := []string{st.WorkspaceManifests[st.WorkspaceHeads["alpha"]].Surfaces[0].ID}
	return st, point, RecoveryRequest{Version: 1, Target: "alpha", PlacementPolicy: "saved"}, ids, o
}

func TestRecoveryCoverageScopeAndWindowStateNegatives(t *testing.T) {
	st, point, r, ids, o := recoveryPlanFixture(t)
	for _, tc := range []struct {
		name   string
		change func(*model.State, *store.PointView, *hyprland.Status)
	}{
		{"stale coverage", func(_ *model.State, _ *store.PointView, o *hyprland.Status) { o.Fresh = false }},
		{"old capture", func(_ *model.State, _ *store.PointView, o *hyprland.Status) {
			o.Snapshot.CapturedAt = time.Now().Add(-3 * time.Second)
		}},
		{"future capture", func(_ *model.State, _ *store.PointView, o *hyprland.Status) {
			o.Snapshot.CapturedAt = time.Now().Add(time.Minute)
		}},
		{"unknown saved coverage", func(_ *model.State, p *store.PointView, _ *hyprland.Status) { p.Payload.Coverage = "partial" }},
		{"pruned snapshot", func(_ *model.State, p *store.PointView, _ *hyprland.Status) { p.Payload = nil; p.Pruned = true }},
		{"foreign ownership", func(s *model.State, _ *store.PointView, _ *hyprland.Status) {
			b := s.ViewportBindings[s.ViewportHeads[ids[0]]]
			b.Target = "beta"
			s.ViewportBindings[b.ID] = b
		}},
		{"stale revision", func(s *model.State, _ *store.PointView, _ *hyprland.Status) {
			task := s.Tasks["alpha"]
			task.Revision++
			s.Tasks["alpha"] = task
		}},
		{"hidden", func(_ *model.State, _ *store.PointView, o *hyprland.Status) { o.Snapshot.Windows[0].Hidden = true }},
		{"offscreen", func(_ *model.State, _ *store.PointView, o *hyprland.Status) {
			o.Snapshot.Windows[0].At = [2]int{-9000, -9000}
		}},
		{"window state changed", func(_ *model.State, _ *store.PointView, o *hyprland.Status) { o.Snapshot.Windows[0].Fullscreen = 1 }},
		{"display scale changed", func(_ *model.State, _ *store.PointView, o *hyprland.Status) { o.Snapshot.Monitors[0].Scale = 2 }},
		{"ambiguous identity", func(_ *model.State, _ *store.PointView, o *hyprland.Status) {
			o.Snapshot.Windows = append(o.Snapshot.Windows, o.Snapshot.Windows[0])
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, p, live := model.Clone(st), model.Clone(point), model.Clone(o)
			tc.change(&s, &p, &live)
			v := planRecovery(s, p, r, ids, "open", live, nil, nil, time.Now().UTC())
			if v.Full {
				t.Fatal("invalid proof became full recovery", v)
			}
		})
	}
	v := planRecovery(st, point, r, []string{}, "open", o, nil, nil, time.Now().UTC())
	if v.Full {
		t.Fatal("empty selection claimed recovery")
	}
}

func TestRecoveryFallbackIsExplicitClampedAndNeverDispatched(t *testing.T) {
	st, point, r, ids, o := recoveryPlanFixture(t)
	point.Payload.Surfaces[0].Window.Floating = true
	w := &o.Snapshot.Windows[0]
	w.Floating, w.At, w.Size = true, [2]int{1920, 0}, [2]int{600, 500}
	o.Snapshot.Monitors[0] = model.DesktopMonitor{ID: 0, Name: "FALLBACK", X: 1920, Width: 600, Height: 500, Scale: 1}
	o.Snapshot.Workspaces[0].MonitorName = "FALLBACK"
	v := planRecovery(st, point, r, ids, "open", o, nil, nil, time.Now().UTC())
	if v.Full || v.Surfaces[0].FallbackApplied || v.Surfaces[0].Placement.Status != "unknown" {
		t.Fatal(v)
	}
	r.PlacementPolicy, r.FallbackMonitor = "named-monitor-clamp", "FALLBACK"
	v = planRecovery(st, point, r, ids, "open", o, nil, nil, time.Now().UTC())
	row := v.Surfaces[0]
	if v.Full || v.Outcome != "degraded" || !row.FallbackApplied || row.ExpectedPlacement.At != w.At || row.ExpectedPlacement.Size != w.Size {
		t.Fatal(v, row)
	}
	w.At[0]++
	v = planRecovery(st, point, r, ids, "open", o, nil, nil, time.Now().UTC())
	if v.Outcome != "partial" {
		t.Fatal("unapplied fallback claimed success", v)
	}
	point.Payload.Surfaces[0].Window.Floating = false
	v = planRecovery(st, point, r, ids, "open", o, nil, nil, time.Now().UTC())
	if v.Surfaces[0].FallbackApplied || v.Full {
		t.Fatal("tiled fallback invented", v)
	}
}

func TestRecoveryApplicationUncertaintyAndProcessReuse(t *testing.T) {
	f, a, _ := applicationFixture(t)
	op := f.queue(f.requestOperation("alpha", "open"))
	f.step()
	f.service.Previews.Process = a.Process
	r := RecoveryRequest{Version: 1, Target: "alpha", OperationID: op.Intent.ID, PlacementPolicy: "saved"}
	v := verifyRecovery(t, f, r)
	if v.Full || v.Outcome != "degraded" || v.Surfaces[0].Ownership.Status != "matched" {
		t.Fatal(v)
	}
	a.readStart = "54321"
	v = verifyRecovery(t, f, r)
	if v.Full || v.Surfaces[0].Ownership.Status != "unknown" {
		t.Fatal("reused process accepted", v)
	}
	st, point, request, ids, o := recoveryPlanFixture(t)
	m := st.WorkspaceManifests[st.WorkspaceHeads["alpha"]]
	m.Surfaces[0].Kind = "terminal"
	st.WorkspaceManifests[m.ID] = m
	b := st.ViewportBindings[st.ViewportHeads[ids[0]]]
	b.SessionBindingID = model.NewID()
	st.ViewportBindings[b.ID] = b
	st.SessionHeads[ids[0]] = b.SessionBindingID
	for _, status := range []string{"current", "changed", "unavailable"} {
		v = planRecovery(st, point, request, ids, "open", o, map[string]SessionCheck{ids[0]: {Status: status}}, nil, time.Now().UTC())
		if v.Full || v.Surfaces[0].Application.Status == "matched" {
			t.Fatal("session survival implied attachment", v)
		}
		if status == "changed" && v.Surfaces[0].Application.Status != "not_matched" {
			t.Fatal(v)
		}
	}
	m.Surfaces[0].Kind = "editor"
	st.WorkspaceManifests[m.ID] = m
	v = planRecovery(st, point, request, ids, "open", o, nil, nil, time.Now().UTC())
	if v.Full || v.Surfaces[0].Application.Status != "unsupported" {
		t.Fatal("editor existence implied buffers", v)
	}
}

func TestRecoveryRequestAndScopedSerialization(t *testing.T) {
	for _, r := range []RecoveryRequest{{}, {Version: 1, Target: "alpha", PlacementPolicy: "auto"}, {Version: 1, Target: "alpha", PlacementPolicy: "named-monitor-clamp"}, {Version: 1, Target: "alpha", PlacementPolicy: "saved", FallbackMonitor: "silent"}, {Version: 1, Target: "alpha", PlacementPolicy: "saved", OperationID: model.NewID(), SnapshotID: model.NewID()}} {
		if r.Validate() == nil {
			t.Fatal("invalid request", r)
		}
	}
	f := newOperationFixture(t, "alpha")
	other := testdesktop.Window("18000999", "0x99", 1, "planning")
	other["title"], other["class"], other["pid"] = "FOREIGN_SECRET", "SECRET_CLASS", 99999
	f.fake.Set("clients", []map[string]any{testdesktop.Window(f.windows["alpha"], "0x64", 1, "planning"), other})
	v := verifyRecovery(t, f, RecoveryRequest{Version: 1, Target: "alpha", PlacementPolicy: "saved"})
	raw, _ := json.Marshal(v)
	for _, secret := range []string{"SECRET", "Same title", "99999", "18000999", "\"pid\""} {
		if strings.Contains(string(raw), secret) {
			t.Fatal("unscoped metadata leaked", string(raw))
		}
	}
}
