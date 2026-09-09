package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/actions"
	"heimdall/internal/adapters/hyprland"
	"heimdall/internal/core"
	"heimdall/internal/model"
	"heimdall/internal/testdesktop"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type operationFixture struct {
	*fixture
	service    *OperationService
	fake       *testdesktop.Fake
	dispatcher *operationDispatcher
	previews   map[string]PreviewRequest
	windows    map[string]string
}
type operationDispatcher struct {
	t           *testing.T
	service     *OperationService
	fake        *testdesktop.Fake
	commands    []hyprland.Command
	effect      func(hyprland.Command)
	afterCommit func()
	receipt     hyprland.DispatchReceipt
}
type operationPrepared struct {
	d        *operationDispatcher
	command  hyprland.Command
	observed hyprland.Status
	used     bool
}

func (d *operationDispatcher) Prepare(ctx context.Context, source model.DesktopSource, command hyprland.Command) (hyprland.PreparedCommand, error) {
	o, err := d.service.Observer.Read(ctx, true)
	if err != nil {
		return nil, err
	}
	return &operationPrepared{d: d, command: command, observed: o}, nil
}
func (p *operationPrepared) Observation() hyprland.Status { return p.observed }
func (p *operationPrepared) Check() error {
	return p.d.service.Observer.CheckCapture(p.observed.Snapshot.ID, p.observed.Snapshot.CapturedAt)
}
func (p *operationPrepared) Close() error { return nil }
func (p *operationPrepared) Send(ctx context.Context, commit func() error) hyprland.DispatchReceipt {
	if p.used {
		p.d.t.Fatal("prepared native action repeated")
	}
	p.used = true
	if err := commit(); err != nil {
		return hyprland.DispatchReceipt{Detail: err.Error()}
	}
	st, _ := p.d.service.Store.State(ctx)
	found := false
	for _, a := range st.Actions {
		if a.Intent.Native != nil && a.Execution == "dispatching" && *a.Intent.Native.Window == p.command.Window {
			found = true
			op := st.WorkspaceOperations[a.Intent.Native.OperationID]
			if op.DiffID == "" || (p.command.Kind == "close" && op.CloseSnapshotID == "") {
				p.d.t.Fatal("input preceded durable diff and close snapshot")
			}
		}
	}
	if !found {
		p.d.t.Fatal("input preceded durable dispatch")
	}
	p.d.commands = append(p.d.commands, p.command)
	if p.d.afterCommit != nil {
		p.d.afterCommit()
	}
	if p.d.effect != nil {
		p.d.effect(p.command)
	}
	return p.d.receipt
}
func newOperationFixture(t *testing.T, targets ...string) *operationFixture {
	t.Helper()
	f, viewport, fake, source := viewportSetup(t)
	s := &OperationService{Store: f.e.Store, Observer: viewport.Observer, Previews: &PreviewService{Store: f.e.Store, Observer: viewport.Observer}}
	d := &operationDispatcher{t: t, service: s, fake: fake, receipt: hyprland.DispatchReceipt{Submitted: true, Acknowledged: true, Detail: "Synthetic acknowledgment"}}
	s.Dispatcher = d
	o := &operationFixture{fixture: f, service: s, fake: fake, dispatcher: d, previews: map[string]PreviewRequest{}, windows: map[string]string{}}
	ids := []string{}
	for i, target := range targets {
		id := fmt.Sprintf("180000%02x", i+1)
		ids = append(ids, id)
		o.windows[target] = id
	}
	fake.Windows(ids...)
	for _, target := range targets {
		st, _ := f.e.Store.State(f.ctx)
		if st.Tasks[target].Revision == 0 {
			task := model.Task{ID: target, Title: target, Type: "project", Status: "active", Done: model.Done{Text: "Manual review"}}
			if _, err := f.e.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "add", Task: &task}, "cli", f.now); err != nil {
				t.Fatal(err)
			}
		}
		r := f.request("workspace.accept", target)
		r.Manifest = &ManifestInput{Previous: "none", Name: target, Surfaces: []model.DesiredSurface{{ID: model.NewID(), Kind: "native", Label: "Synthetic view", Required: true, RestorePolicy: "manual"}}}
		var m model.WorkspaceManifest
		json.Unmarshal(f.send(r), &m)
		bind := viewportRequest(t, f, viewport, m, source.ID)
		bind.Binding.Window = &model.WindowIdentity{SourceEpoch: source.Source.Epoch, StableID: o.windows[target]}
		if _, err := viewport.Execute(f.ctx, bind, "cli", f.now); err != nil {
			t.Fatal(err)
		}
		st, _ = f.e.Store.State(f.ctx)
		snapshot := SnapshotRequest{Version: 1, ID: model.NewID(), Op: "capture", Target: target, Previous: "none", ExpectedTaskRevision: m.TaskRevision, ManifestID: m.ID, SourceID: source.ID, InputDigest: model.SnapshotInputDigest(st, target)}
		if _, err := (&SnapshotService{Store: f.e.Store, Observer: viewport.Observer}).Execute(f.ctx, snapshot, "cli", f.now); err != nil {
			t.Fatal(err)
		}
		o.previews[target] = PreviewRequest{Version: 1, Target: target, ManifestID: m.ID, SnapshotID: snapshot.ID}
	}
	fake.Set("activewindow", map[string]any{})
	return o
}
func (f *operationFixture) requestOperation(target, kind string) OperationRequest {
	f.t.Helper()
	v, err := f.service.Previews.Build(f.ctx, f.previews[target], time.Now().UTC())
	if err != nil {
		f.t.Fatal(err)
	}
	ids := []string{}
	for _, row := range v.Surfaces {
		if row.Membership != "removed" {
			ids = append(ids, row.SurfaceID)
		}
	}
	return OperationRequest{Version: 1, ID: model.NewID(), Kind: kind, Preview: v, SurfaceIDs: ids}
}
func (f *operationFixture) queue(r OperationRequest) model.WorkspaceOperation {
	f.t.Helper()
	raw, err := f.service.Queue(f.ctx, r, "cli")
	if err != nil {
		f.t.Fatal(err)
	}
	var op model.WorkspaceOperation
	if err = json.Unmarshal(raw, &op); err != nil {
		f.t.Fatal(err)
	}
	return op
}
func (f *operationFixture) state() model.State {
	f.t.Helper()
	st, err := f.e.Store.State(f.ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	return st
}
func (f *operationFixture) step() {
	f.t.Helper()
	if err := f.service.Step(f.ctx); err != nil {
		f.t.Fatal(err)
	}
}

func TestWorkspaceOperationCloseJournalReplayAndUnownedWindow(t *testing.T) {
	f := newOperationFixture(t, "alpha")
	f.fake.Windows(f.windows["alpha"], "18000999")
	r := f.requestOperation("alpha", "close")
	op := f.queue(r)
	f.dispatcher.effect = func(c hyprland.Command) { f.fake.Windows("18000999") }
	f.step()
	st := f.state()
	done := st.WorkspaceOperations[op.Intent.ID]
	if done.Status != "complete" || done.Outcome != "matched" || len(st.WorkspaceSlots) != 0 || len(f.dispatcher.commands) != 1 {
		t.Fatal(done, st.WorkspaceSlots, f.dispatcher.commands)
	}
	a := st.Actions[op.ActionIDs[0]]
	if a.Execution != "api_reported" || a.Verification != "matched" || a.Observation.Native.Window != nil {
		t.Fatal(a)
	}
	point, err := f.e.Store.WorkspacePoint(f.ctx, "alpha", done.CloseSnapshotID)
	if err != nil || point.Payload == nil || len(point.Payload.Surfaces) != 1 || point.Payload.Surfaces[0].Window.Identity.StableID != f.windows["alpha"] {
		t.Fatal(point, err)
	}
	if st.SnapshotHeads["alpha"].ID != done.CloseSnapshotID || st.Tasks["alpha"].Task.Status != "active" {
		t.Fatal("close changed completion or lost pre-close head")
	}
	events, _ := f.e.Store.Events(f.ctx)
	diffs, closed := 0, 0
	for _, e := range events {
		if e.Subject == "workspace" && e.Verb == "diffed" {
			diffs++
		}
		if e.Subject == "workspace" && e.Verb == "closed" {
			closed++
		}
	}
	if diffs != 1 || closed != 1 {
		t.Fatal(diffs, closed)
	}
	reads, _ := f.fake.Count()
	replayed, err := f.e.Store.Replay(f.ctx)
	after, _ := f.fake.Count()
	if err != nil || !reflect.DeepEqual(st, replayed) || reads != after {
		t.Fatal("replay changed state or observed desktop", err)
	}
	f.service.Observer.Close()
	retry, err := f.service.Queue(f.ctx, r, "cli")
	if err != nil || len(retry) == 0 || len(f.dispatcher.commands) != 1 {
		t.Fatal("retry dispatched or needed live desktop", err)
	}
}

func TestWorkspaceOperationAlreadyAbsentClosePreservesCompleteHead(t *testing.T) {
	f := newOperationFixture(t, "alpha")
	head := f.state().SnapshotHeads["alpha"].ID
	f.fake.Windows()
	op := f.queue(f.requestOperation("alpha", "close"))
	f.step()
	st := f.state()
	if len(op.ActionIDs) != 0 || len(f.dispatcher.commands) != 0 || st.WorkspaceOperations[op.Intent.ID].Outcome != "matched" || st.SnapshotHeads["alpha"].ID != head {
		t.Fatal("empty close erased saved layout or sent input", st.WorkspaceOperations)
	}
	point, err := f.e.Store.WorkspacePoint(f.ctx, "alpha", op.CloseSnapshotID)
	if err != nil || point.Point.Published || point.Point.Coverage != "partial" {
		t.Fatal(point, err)
	}
}

func TestWorkspaceOperationExistingOverflowIsReportedAndNeverEvicted(t *testing.T) {
	f := newOperationFixture(t, "alpha", "beta", "gamma", "delta", "epsilon")
	f.fake.Windows(f.windows["alpha"], f.windows["beta"], f.windows["gamma"], f.windows["delta"])
	if _, err := f.service.Queue(f.ctx, f.requestOperation("epsilon", "open"), "cli"); err == nil || !strings.Contains(err.Error(), "alpha (resident)") || !strings.Contains(err.Error(), "delta (resident)") {
		t.Fatal("capacity refusal omitted existing residents", err)
	}
	if len(f.dispatcher.commands) != 0 {
		t.Fatal("capacity refusal evicted a view")
	}
	op := f.queue(f.requestOperation("alpha", "close"))
	if len(f.state().WorkspaceSlots) != 4 {
		t.Fatal("existing overflow dropped from census")
	}
	f.dispatcher.effect = func(hyprland.Command) { f.fake.Windows(f.windows["beta"], f.windows["gamma"], f.windows["delta"]) }
	f.step()
	st := f.state()
	if st.WorkspaceOperations[op.Intent.ID].Outcome != "matched" || len(st.WorkspaceSlots) != 3 || len(f.dispatcher.commands) != 1 {
		t.Fatal(st.WorkspaceOperations, st.WorkspaceSlots)
	}
}

func TestWorkspaceOperationAckIsNotFocusOrClosure(t *testing.T) {
	for _, kind := range []string{"focus", "close"} {
		t.Run(kind, func(t *testing.T) {
			f := newOperationFixture(t, "alpha")
			op := f.queue(f.requestOperation("alpha", kind))
			for range 8 {
				f.step()
			}
			st := f.state()
			a := st.Actions[op.ActionIDs[0]]
			if a.Execution != "api_reported" || a.Verification != "not_matched" || st.WorkspaceOperations[op.Intent.ID].Outcome != "partial" || len(st.WorkspaceSlots) != 1 {
				t.Fatal(a, st.WorkspaceOperations, st.WorkspaceSlots)
			}
			f.step()
			if len(f.dispatcher.commands) != 1 {
				t.Fatal("API refusal retried input")
			}
		})
	}
}

func TestWorkspaceOperationLostAckReconcilesAbsenceWithoutRedispatch(t *testing.T) {
	f := newOperationFixture(t, "alpha")
	f.dispatcher.receipt.Acknowledged = false
	op := f.queue(f.requestOperation("alpha", "close"))
	f.step()
	st := f.state()
	a := st.Actions[op.ActionIDs[0]]
	if a.Execution != "uncertain" || !model.ActionHolds(a) || len(st.WorkspaceSlots) != 1 {
		t.Fatal(a, st.WorkspaceSlots)
	}
	f.fake.Windows()
	current := st.WorkspaceOperations[op.Intent.ID]
	if _, err := f.service.Control(f.ctx, OperationControl{Version: 1, ID: model.NewID(), Target: "alpha", OperationID: op.Intent.ID, ExpectedRevision: current.Revision, Reason: "Observe existing attempt"}, "reconcile", "cli"); err != nil {
		t.Fatal(err)
	}
	f.step()
	st = f.state()
	a = st.Actions[op.ActionIDs[0]]
	if a.Execution != "uncertain" || a.Verification != "matched" || st.WorkspaceOperations[op.Intent.ID].Outcome != "matched" || len(st.WorkspaceSlots) != 0 || len(f.dispatcher.commands) != 1 {
		t.Fatal(a, st.WorkspaceOperations, st.WorkspaceSlots)
	}
}

func TestWorkspaceOperationCancellationAfterDispatchRetainsResident(t *testing.T) {
	f := newOperationFixture(t, "alpha")
	op := f.queue(f.requestOperation("alpha", "close"))
	f.dispatcher.afterCommit = func() {
		current := f.state().WorkspaceOperations[op.Intent.ID]
		if _, err := f.service.Control(f.ctx, OperationControl{Version: 1, ID: model.NewID(), Target: "alpha", OperationID: op.Intent.ID, ExpectedRevision: current.Revision, Reason: "Stop new input"}, "cancel", "cli"); err != nil {
			t.Fatal(err)
		}
	}
	for range 8 {
		f.step()
	}
	st := f.state()
	a := st.Actions[op.ActionIDs[0]]
	if !a.CancelRequested || a.Report == nil || len(st.WorkspaceSlots) != 1 || st.WorkspaceOperations[op.Intent.ID].Outcome != "cancelled" {
		t.Fatal(a, st.WorkspaceOperations, st.WorkspaceSlots)
	}
}

func TestWorkspaceOperationStaleOrForgedPreviewIsAtomic(t *testing.T) {
	f := newOperationFixture(t, "alpha")
	r := f.requestOperation("alpha", "close")
	before := f.state()
	for _, mutate := range []func(*OperationRequest){func(r *OperationRequest) { r.Preview.Token = "forged" }, func(r *OperationRequest) { r.SurfaceIDs = append(r.SurfaceIDs, r.SurfaceIDs[0]) }, func(r *OperationRequest) { r.Preview.SourceEpoch = "changed" }} {
		bad := model.Clone(r)
		mutate(&bad)
		if _, err := f.service.Queue(f.ctx, bad, "cli"); err == nil {
			t.Fatal("accepted forged review")
		}
		if !reflect.DeepEqual(before, f.state()) {
			t.Fatal("rejected operation leaked events or residency")
		}
	}
	f.fake.Windows("18009999")
	if _, err := f.service.Queue(f.ctx, r, "cli"); err == nil {
		t.Fatal("accepted changed desktop")
	}
}

func TestWorkspaceOperationCapacityRaceAndExplicitSwap(t *testing.T) {
	f := newOperationFixture(t, "alpha", "beta", "gamma", "delta")
	f.fake.Windows(f.windows["alpha"], f.windows["beta"])
	a, b := f.requestOperation("gamma", "open"), f.requestOperation("delta", "open")
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, r := range []OperationRequest{a, b} {
		wg.Add(1)
		go func() { defer wg.Done(); _, err := f.service.Queue(f.ctx, r, "cli"); errs <- err }()
	}
	wg.Wait()
	close(errs)
	success := 0
	for err := range errs {
		if err == nil {
			success++
		}
	}
	if success != 1 || len(f.state().WorkspaceSlots) != 3 {
		t.Fatal("capacity race", success, f.state().WorkspaceSlots)
	}
	// A fresh second review still cannot steal the reserved slot.
	st := f.state()
	other := "gamma"
	for _, op := range st.WorkspaceOperations {
		if op.Intent.Target == other {
			other = "delta"
		}
	}
	if _, err := f.service.Queue(f.ctx, f.requestOperation(other, "open"), "cli"); err == nil {
		t.Fatal("capacity exceeded")
	}
	// Settle the unsupported launch, then explicitly name alpha as swap victim.
	f.step()
	r := f.requestOperation(other, "open")
	swap := f.requestOperation("alpha", "close").Preview
	r.Swap = &swap
	op := f.queue(r)
	f.dispatcher.effect = func(c hyprland.Command) {
		if c.Window.StableID != f.windows["alpha"] {
			t.Fatal("wrong swap victim")
		}
		f.fake.Windows(f.windows["beta"])
	}
	f.step()
	st = f.state()
	done := st.WorkspaceOperations[op.Intent.ID]
	if done.Outcome != "partial" || done.Status != "complete" || len(f.dispatcher.commands) != 1 || len(st.WorkspaceSlots) != 1 {
		t.Fatal(done, st.WorkspaceSlots, f.dispatcher.commands)
	}
	if st.WorkspaceSlots[2].Target != "beta" {
		t.Fatal("unselected resident affected", st.WorkspaceSlots)
	}
}

func TestWorkspaceOperationRecoveryNeverRepeatsDispatchedInput(t *testing.T) {
	f := newOperationFixture(t, "alpha")
	op := f.queue(f.requestOperation("alpha", "close"))
	// Stop after the durable boundary by making the receipt transaction fail.
	f.dispatcher.afterCommit = func() { f.service.Clock = func() time.Time { return time.Time{} } }
	if err := f.service.dispatch(f.ctx, f.state().Actions[op.ActionIDs[0]]); err == nil {
		t.Fatal("invalid report time accepted")
	}
	f.service.Clock = nil
	if err := (actions.Service{Store: f.e.Store}).Recover(f.ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	f.fake.Windows()
	f.step()
	a := f.state().Actions[op.ActionIDs[0]]
	if a.Execution != "uncertain" || a.Verification != "matched" || len(f.dispatcher.commands) != 1 {
		t.Fatal(a, f.dispatcher.commands)
	}
}
