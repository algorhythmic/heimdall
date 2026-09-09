package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/actions"
	"heimdall/internal/adapters/application"
	"heimdall/internal/adapters/herdr"
	"heimdall/internal/adapters/hyprland"
	"heimdall/internal/core"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"heimdall/internal/testdesktop"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"
)

type applicationFake struct {
	f            *operationFixture
	launches     int
	pid          model.ApplicationProcess
	readStart    string
	effect       func(string)
	beforeCommit func()
	afterCommit  func()
	lost         bool
}
type applicationPrepared struct {
	a        *applicationFake
	attempt  string
	observed hyprland.Status
}

func (a *applicationFake) Prepare(ctx context.Context, _ model.DesktopSource, _ model.ApplicationSpec, attempt string, _ *model.SessionBinding) (hyprland.PreparedCommand, error) {
	o, err := a.f.service.Observer.Read(ctx, true)
	return &applicationPrepared{a, attempt, o}, err
}
func (a *applicationFake) Process(int) (model.ApplicationProcess, error) {
	p := a.pid
	if a.readStart != "" {
		p.Start = a.readStart
	}
	return p, nil
}
func (p *applicationPrepared) Observation() hyprland.Status { return p.observed }
func (p *applicationPrepared) Check() error {
	return p.a.f.service.Observer.CheckCapture(p.observed.Snapshot.ID, p.observed.Snapshot.CapturedAt)
}
func (p *applicationPrepared) Close() error { return nil }
func (p *applicationPrepared) Send(_ context.Context, commit func() error) hyprland.DispatchReceipt {
	if p.a.beforeCommit != nil {
		p.a.beforeCommit()
	}
	if err := commit(); err != nil {
		return hyprland.DispatchReceipt{Detail: err.Error()}
	}
	if p.a.afterCommit != nil {
		p.a.afterCommit()
	}
	p.a.launches++
	if p.a.effect != nil {
		p.a.effect(p.attempt)
	}
	if p.a.lost {
		return hyprland.DispatchReceipt{Submitted: true, Detail: "Synthetic lost process receipt"}
	}
	pin := p.a.pid
	return hyprland.DispatchReceipt{Submitted: true, Acknowledged: true, Process: &pin}
}

func applicationFixture(t *testing.T) (*operationFixture, *applicationFake, ApplicationRequest) {
	t.Helper()
	if runtime.GOOS != "linux" {
		t.Skip("Linux recipe filesystem fixture")
	}
	f, viewport, fake, source := viewportSetup(t)
	_, m := f.manifest("alpha")
	bind := viewportRequest(t, f, viewport, m, source.ID)
	if _, err := viewport.Execute(f.ctx, bind, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	s := &OperationService{Store: f.e.Store, Observer: viewport.Observer, Previews: &PreviewService{Store: f.e.Store, Observer: viewport.Observer}}
	d := &operationDispatcher{t: t, service: s, fake: fake, receipt: hyprland.DispatchReceipt{Submitted: true, Acknowledged: true}}
	s.Dispatcher = d
	o := &operationFixture{fixture: f, service: s, fake: fake, dispatcher: d, previews: map[string]PreviewRequest{}, windows: map[string]string{"alpha": "18000001"}}
	dir := t.TempDir()
	exe := filepath.Join(dir, "reviewed-executable")
	if err := os.WriteFile(exe, []byte("synthetic executable, never executed"), 0700); err != nil {
		t.Fatal(err)
	}
	digest, err := application.Digest(exe)
	if err != nil {
		t.Fatal(err)
	}
	r := ApplicationRequest{Version: 1, ID: model.NewID(), Target: "alpha", ExpectedTaskRevision: 1, ManifestID: m.ID, SurfaceID: m.Surfaces[0].ID, Previous: "none", Spec: &model.ApplicationSpec{Adapter: "foot", Executable: exe, ExecutableDigest: digest, Command: exe, CommandDigest: digest, Argv: []string{}, Cwd: dir, ClosePolicy: "leave_open"}}
	if _, err := f.s.ReviewApplication(f.ctx, r, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	st, _ := f.e.Store.State(f.ctx)
	snap := SnapshotRequest{Version: 1, ID: model.NewID(), Op: "capture", Target: "alpha", Previous: "none", ExpectedTaskRevision: 1, ManifestID: m.ID, SourceID: source.ID, InputDigest: model.SnapshotInputDigest(st, "alpha")}
	if _, err := (&SnapshotService{Store: f.e.Store, Observer: viewport.Observer}).Execute(f.ctx, snap, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	o.previews["alpha"] = PreviewRequest{Version: 1, Target: "alpha", ManifestID: m.ID, SnapshotID: snap.ID}
	a := &applicationFake{f: o, pid: model.ApplicationProcess{PID: 444, Start: "12345"}}
	a.effect = func(attempt string) {
		w := testdesktop.Window("18000009", "0x99", 1, "planning")
		w["pid"], w["class"] = a.pid.PID, model.ApplicationClass(attempt)
		fake.Set("clients", []map[string]any{w})
	}
	s.Applications = a
	fake.Windows()
	return o, a, r
}

func TestApplicationLaunchRebindAndNoDuplicate(t *testing.T) {
	f, a, r := applicationFixture(t)
	request := f.requestOperation("alpha", "open")
	op := f.queue(request)
	if len(op.ActionIDs) != 1 || len(op.Unsupported) != 0 {
		t.Fatal(op)
	}
	if err := f.service.Step(f.ctx); err != nil {
		t.Fatal(err)
	}
	st, _ := f.e.Store.State(f.ctx)
	action := st.Actions[op.ActionIDs[0]]
	if a.launches != 1 || action.Verification != "matched" || st.WorkspaceOperations[op.Intent.ID].Outcome != "matched" {
		t.Fatalf("launch %d action %+v operation %+v", a.launches, action, st.WorkspaceOperations[op.Intent.ID])
	}
	b := st.ViewportBindings[st.ViewportHeads[r.SurfaceID]]
	if b.Version != 3 || b.ApplicationActionID != action.Intent.ID || b.Window.StableID != "18000009" {
		t.Fatal("new runtime not rebound", b)
	}
	if !model.Contains([]string{"resident", "partial"}, st.WorkspaceSlots[op.Slot].Status) {
		t.Fatal("new window lost capacity", st.WorkspaceSlots)
	}
	f.queue(request)
	if err := f.service.Step(f.ctx); err != nil {
		t.Fatal(err)
	}
	second := f.queue(f.requestOperation("alpha", "open"))
	current, _ := f.e.Store.State(f.ctx)
	if current.Actions[second.ActionIDs[0]].Intent.Native.Kind != "focus" || a.launches != 1 {
		t.Fatal("repeat open relaunched terminal")
	}
	before, _ := f.e.Store.State(f.ctx)
	replayed, err := f.e.Store.Replay(f.ctx)
	if err != nil || !reflect.DeepEqual(before, replayed) || a.launches != 1 {
		t.Fatal("replay changed application state", err)
	}
}

func TestApplicationAmbiguousWrongProcessAndLostReceipt(t *testing.T) {
	for _, kind := range []string{"ambiguous", "wrong_pid", "reused_pid", "lost_receipt"} {
		t.Run(kind, func(t *testing.T) {
			f, a, r := applicationFixture(t)
			a.lost = kind == "lost_receipt"
			original := a.effect
			a.effect = func(attempt string) {
				original(attempt)
				w := testdesktop.Window("18000009", "0x99", 1, "planning")
				w["pid"], w["class"] = a.pid.PID, model.ApplicationClass(attempt)
				switch kind {
				case "ambiguous":
					second := testdesktop.Window("1800000a", "0x9a", 1, "planning")
					second["pid"], second["class"] = a.pid.PID, model.ApplicationClass(attempt)
					f.fake.Set("clients", []map[string]any{w, second})
				case "wrong_pid":
					w["pid"] = 999
					f.fake.Set("clients", []map[string]any{w})
				}
			}
			op := f.queue(f.requestOperation("alpha", "open"))
			if kind == "reused_pid" {
				a.readStart = "99999"
			}
			if err := f.service.Step(f.ctx); err != nil {
				t.Fatal(err)
			}
			for i := 0; i < 9; i++ {
				if err := f.service.Step(f.ctx); err != nil {
					t.Fatal(err)
				}
			}
			st, _ := f.e.Store.State(f.ctx)
			if st.Actions[op.ActionIDs[0]].Verification == "matched" || st.ViewportBindings[st.ViewportHeads[r.SurfaceID]].Version == 3 || a.launches != 1 {
				t.Fatal("unproven window gained ownership", kind, st.Actions[op.ActionIDs[0]])
			}
			if _, err := f.service.Queue(f.ctx, f.requestOperation("alpha", "open"), "cli"); err == nil {
				t.Fatal("uncertain launch allowed duplicate")
			}
		})
	}
}

func TestApplicationRecipePinsAndClosePolicy(t *testing.T) {
	f, a, r := applicationFixture(t)
	st, _ := f.e.Store.State(f.ctx)
	before, _ := json.Marshal(st)
	bad := r
	bad.ID = model.NewID()
	bad.Target = "beta"
	if _, err := f.s.ReviewApplication(f.ctx, bad, "cli", f.now); err == nil {
		t.Fatal("cross-task recipe accepted")
	}
	st, _ = f.e.Store.State(f.ctx)
	after, _ := json.Marshal(st)
	if string(before) != string(after) {
		t.Fatal("rejected recipe changed state")
	}
	request := f.requestOperation("alpha", "open")
	r.ID, r.Previous = model.NewID(), r.ID
	r.Spec.ClosePolicy = "graceful_session_end"
	if _, err := f.s.ReviewApplication(f.ctx, r, "cli", time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err := f.service.Queue(f.ctx, request, "cli"); err == nil {
		t.Fatal("recipe change retained stale preview")
	}
	f.queue(f.requestOperation("alpha", "open"))
	if err := f.service.Step(f.ctx); err != nil {
		t.Fatal(err)
	}
	f.dispatcher.effect = func(hyprland.Command) { f.fake.Windows() }
	op := f.queue(f.requestOperation("alpha", "close"))
	if len(op.ActionIDs) != 1 || op.CloseSnapshotID == "" {
		t.Fatal("reviewed close did not retain membership", op)
	}
	if err := f.service.Step(f.ctx); err != nil {
		t.Fatal(err)
	}
	st, _ = f.e.Store.State(f.ctx)
	if st.WorkspaceOperations[op.Intent.ID].Outcome != "matched" || a.launches != 1 {
		t.Fatal("graceful close failed", st.WorkspaceOperations[op.Intent.ID])
	}
}

func TestApplicationAbsenceRecheckedBeforeLaunch(t *testing.T) {
	f, a, _ := applicationFixture(t)
	op := f.queue(f.requestOperation("alpha", "open"))
	f.fake.Windows("18000001")
	if err := f.service.Step(f.ctx); err != nil {
		t.Fatal(err)
	}
	st, _ := f.e.Store.State(f.ctx)
	if a.launches != 0 || st.Actions[op.ActionIDs[0]].Execution != "refused" {
		t.Fatal(fmt.Sprint("surviving window launched again ", st.Actions[op.ActionIDs[0]]))
	}
}

func TestApplicationCancellationAndStaleRecipeAfterLaunch(t *testing.T) {
	for _, kind := range []string{"cancel", "retire"} {
		t.Run(kind, func(t *testing.T) {
			f, a, r := applicationFixture(t)
			op := f.queue(f.requestOperation("alpha", "open"))
			a.afterCommit = func() {
				if kind == "cancel" {
					current, _ := f.service.Show(f.ctx, "alpha", op.Intent.ID)
					_, err := f.service.Control(f.ctx, OperationControl{Version: 1, ID: model.NewID(), Target: "alpha", OperationID: op.Intent.ID, ExpectedRevision: current.Revision, Reason: "Stop after durable launch boundary"}, "cancel", "cli")
					if err != nil {
						t.Fatal(err)
					}
				} else {
					retired := r
					retired.ID, retired.Previous, retired.Spec = model.NewID(), r.ID, nil
					if _, err := f.s.ReviewApplication(f.ctx, retired, "cli", time.Now()); err != nil {
						t.Fatal(err)
					}
				}
			}
			for i := 0; i < 10; i++ {
				if err := f.service.Step(f.ctx); err != nil {
					t.Fatal(err)
				}
			}
			st, _ := f.e.Store.State(f.ctx)
			action := st.Actions[op.ActionIDs[0]]
			if a.launches != 1 {
				t.Fatal("cancel or changed recipe repeated input")
			}
			if kind == "cancel" {
				if action.Verification != "matched" || st.WorkspaceOperations[op.Intent.ID].Outcome != "cancelled" {
					t.Fatal("cancel erased observable launched view", action)
				}
			} else if action.Verification != "unknown" || action.VerificationAttempts != 8 {
				t.Fatal("stale launch proof did not exhaust bounded observation", action)
			}
		})
	}
}

func TestApplicationRestartAfterEffectBeforeReceipt(t *testing.T) {
	f, a, r := applicationFixture(t)
	op := f.queue(f.requestOperation("alpha", "open"))
	effect := a.effect
	a.effect = func(attempt string) { effect(attempt); panic("simulated coordinator loss after process creation") }
	func() {
		defer func() {
			if recover() == nil {
				t.Fatal("missing simulated interruption")
			}
		}()
		_ = f.service.Step(f.ctx)
	}()
	st, _ := f.e.Store.State(f.ctx)
	action := st.Actions[op.ActionIDs[0]]
	if action.Execution != "dispatching" || action.Report != nil || a.launches != 1 {
		t.Fatal("incorrect interrupted launch boundary", action)
	}
	dir := f.e.Dir
	f.e.Close()
	engine, err := core.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { engine.Close() })
	f.e = engine
	f.s.Store = engine.Store
	f.service.Store = engine.Store
	f.service.Previews.Store = engine.Store
	if err := (actions.Service{Store: engine.Store}).Recover(f.ctx, time.Now()); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 10; i++ {
		if err := f.service.Step(f.ctx); err != nil {
			t.Fatal(err)
		}
	}
	st, _ = engine.Store.State(f.ctx)
	action = st.Actions[op.ActionIDs[0]]
	if a.launches != 1 || action.Execution != "uncertain" || action.Report != nil || action.Verification != "unknown" || st.ViewportBindings[st.ViewportHeads[r.SurfaceID]].Version == 3 {
		t.Fatal("restart relaunched or adopted an unproven process", action)
	}
	replay, err := engine.Store.Replay(f.ctx)
	if err != nil || !reflect.DeepEqual(st, replay) {
		t.Fatal("interrupted launch replay drift", err)
	}
}

func TestApplicationHerdrSurvivalAndDetach(t *testing.T) {
	f, a, r := applicationFixture(t)
	h := &fakeHerdr{observation: herdr.Observation{Locator: model.SessionLocator{Adapter: "herdr", Environment: "local", Host: strings.Repeat("a", 64), SourceEpoch: strings.Repeat("b", 64), SessionID: "/tmp/synthetic-herdr.sock", WorkspaceID: "w1", PaneID: "w1:p1", Platform: "linux", Cwd: r.Spec.Cwd}, Herdr: model.HerdrIdentity{Protocol: 20, ServerVersion: "0.8.2", TerminalID: "terminal-1", TabID: "w1:t1", ShellPID: 123, ShellStart: "12345"}}}
	b, err := (HerdrService{Store: f.e.Store, Adapter: h}).Bind(f.ctx, HerdrBindRequest{Version: 1, ID: model.NewID(), Target: r.Target, SurfaceID: r.SurfaceID, ManifestID: r.ManifestID, Previous: "none", ExpectedTaskRevision: 1, Socket: h.observation.Locator.SessionID, PaneID: h.observation.Locator.PaneID}, "cli", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	var binding model.SessionBinding
	if err := json.Unmarshal(b, &binding); err != nil {
		t.Fatal(err)
	}
	r.ID, r.Previous = model.NewID(), r.ID
	r.Spec.Adapter, r.Spec.SessionBindingID, r.Spec.ClosePolicy = "foot-herdr", binding.ID, "detach"
	if _, err := f.s.ReviewApplication(f.ctx, r, "cli", time.Now()); err != nil {
		t.Fatal(err)
	}
	f.service.Previews.Herdr = h
	op := f.queue(f.requestOperation("alpha", "open"))
	if err := f.service.Step(f.ctx); err != nil {
		t.Fatal(err)
	}
	st, _ := f.e.Store.State(f.ctx)
	if st.Actions[op.ActionIDs[0]].Verification != "matched" {
		t.Fatal(st.Actions[op.ActionIDs[0]])
	}
	f.dispatcher.effect = func(hyprland.Command) { f.fake.Windows() }
	op = f.queue(f.requestOperation("alpha", "close"))
	if err := f.service.Step(f.ctx); err != nil {
		t.Fatal(err)
	}
	st, _ = f.e.Store.State(f.ctx)
	if st.WorkspaceOperations[op.Intent.ID].Outcome != "matched" || st.Actions[op.ActionIDs[0]].Observation.Native.SessionDigest == "" {
		t.Fatal("detach lost original session proof")
	}
	h.observation.Herdr.ShellStart = "99999"
	expired := f.queue(f.requestOperation("alpha", "open"))
	if len(expired.ActionIDs) != 0 || len(expired.Unsupported) != 1 || a.launches != 1 {
		t.Fatal("expired process was resurrected", expired)
	}
}

func TestApplicationReducersRejectForgedProofsWithoutMutation(t *testing.T) {
	f, _, _ := applicationFixture(t)
	f.queue(f.requestOperation("alpha", "open"))
	if err := f.service.Step(f.ctx); err != nil {
		t.Fatal(err)
	}
	events, err := f.e.Store.Events(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	st := model.Empty()
	checked := 0
	for _, e := range events {
		bad := e
		var raw map[string]any
		if err := json.Unmarshal(e.Payload, &raw); err != nil {
			t.Fatal(err)
		}
		mutated := false
		switch {
		case e.Subject == "application":
			raw["target"] = "beta"
			mutated = true
		case e.Subject == "viewport" && raw["version"] == float64(3):
			raw["application_action_id"] = model.NewID()
			mutated = true
		case e.Subject == "action" && e.Verb == "transitioned" && raw["kind"] == "report":
			raw["report"].(map[string]any)["native"].(map[string]any)["process"].(map[string]any)["pid"] = 0
			mutated = true
		case e.Subject == "action" && e.Verb == "transitioned" && raw["kind"] == "verify":
			raw["observation"].(map[string]any)["native"].(map[string]any)["launch_candidates"] = 2
			mutated = true
		}
		if mutated {
			bad.Payload, _ = json.Marshal(raw)
			before := model.Clone(st)
			if store.Apply(&st, bad) == nil || !reflect.DeepEqual(before, st) {
				t.Fatal("forged application proof accepted or changed projection", e.Subject, e.Verb)
			}
			checked++
		}
		if err := store.Apply(&st, e); err != nil {
			t.Fatal(err)
		}
	}
	if checked != 4 {
		t.Fatal("proof fixtures incomplete", checked)
	}
}
