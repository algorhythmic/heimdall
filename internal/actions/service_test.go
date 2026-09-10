package actions_test

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/actions"
	"heimdall/internal/adapters/hyprland"
	"heimdall/internal/browser"
	"heimdall/internal/core"
	"heimdall/internal/model"
	"heimdall/internal/testdesktop"
	"heimdall/internal/workspace"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fixture struct {
	autoRead                                      bool
	t                                             *testing.T
	ctx                                           context.Context
	e                                             *core.Engine
	a                                             actions.Service
	b                                             browser.Service
	now                                           time.Time
	profile, epoch, connection, manifest, surface string
	sequence                                      int64
}

func setup(t *testing.T, done ...model.Done) *fixture {
	e, err := core.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	f := &fixture{t: t, ctx: context.Background(), e: e, a: actions.Service{Store: e.Store}, b: *browser.NewService(e.Store), autoRead: true, now: time.Now().UTC(), profile: model.NewID(), epoch: model.NewID(), connection: model.NewID(), surface: model.NewID()}
	f.b.Runtime.Clock = func() time.Time { return f.now }
	for _, target := range []string{"alpha", "beta"} {
		task := model.Task{ID: target, Title: target, Type: "project", Status: "active"}
		if target == "alpha" && len(done) > 0 {
			task.Done = done[0]
		}
		if _, err = e.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "add", Task: &task}, "cli", f.now); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := (workspace.Service{Store: e.Store}).Execute(f.ctx, workspace.Request{Version: 1, ID: model.NewID(), Op: "workspace.accept", Target: "alpha", ExpectedTaskRevision: 1, Manifest: &workspace.ManifestInput{Previous: "none", Name: "Planning", Surfaces: []model.DesiredSurface{{ID: f.surface, Kind: "browser", Label: "Planning browser", Required: true, RestorePolicy: "manual"}}}}, "cli", f.now)
	if err != nil {
		t.Fatal(err)
	}
	var m model.WorkspaceManifest
	json.Unmarshal(raw, &m)
	f.manifest = m.ID
	hello := f.message("hello")
	hello.ExtensionVersion = "0.3.0"
	hello.ActionProtocol = 1
	hello.VerificationProtocol = 1
	f.send(hello)
	if _, err = f.b.Control(f.ctx, browser.Control{ID: model.NewID(), Action: "pair", Profile: f.profile}, f.now); err != nil {
		t.Fatal(err)
	}
	f.inventory(nil)
	return f
}
func (f *fixture) message(kind string) browser.Message {
	return browser.Message{V: 1, Type: kind, ID: model.NewID(), Profile: f.profile, Epoch: f.epoch, Connection: f.connection}
}
func (f *fixture) send(m browser.Message) browser.Reply {
	f.t.Helper()
	raw, err := f.b.Handle(f.ctx, m, f.now)
	if err != nil {
		f.t.Fatal(err)
	}
	var r browser.Reply
	if err = json.Unmarshal(raw, &r); err != nil {
		f.t.Fatal(err)
	}
	return r
}
func (f *fixture) inventory(tabs []model.BrowserTab) {
	m := f.message("inventory")
	f.sequence++
	m.Sequence = f.sequence
	m.ObservedAt = f.now.Format(time.RFC3339Nano)
	complete, focus := true, 1
	m.Complete = &complete
	m.FocusedWindow = &focus
	m.Tabs = tabs
	f.send(m)
	f.prepareReadback()
}
func (f *fixture) request() actions.Request {
	f.t.Helper()
	v, err := f.a.Context(f.ctx, "alpha")
	if err != nil {
		f.t.Fatal(err)
	}
	return actions.Request{Version: 1, ID: model.NewID(), Target: "alpha", ExpectedTaskRevision: v.TaskRevision, ManifestID: f.manifest, SurfaceID: f.surface, ContextDigest: v.ContextDigest, Browser: model.BrowserIntent{Profile: f.profile, Epoch: f.epoch, Action: "open", URL: "https://example.test/"}}
}
func (f *fixture) queue(r actions.Request) model.ActionRecord {
	f.t.Helper()
	raw, err := f.a.Queue(f.ctx, r, "cli", f.now)
	if err != nil {
		f.t.Fatal(err)
	}
	var a model.ActionRecord
	json.Unmarshal(raw, &a)
	f.prepareReadback()
	return a
}
func (f *fixture) action(id string) model.ActionRecord {
	f.t.Helper()
	a, err := f.a.Show(f.ctx, "alpha", id)
	if err != nil {
		f.t.Fatal(err)
	}
	return a
}
func (f *fixture) cancel(a model.ActionRecord) model.ActionRecord {
	f.t.Helper()
	raw, err := f.a.Cancel(f.ctx, actions.CancelRequest{Version: 1, ID: model.NewID(), Target: "alpha", ActionID: a.Intent.ID, ExpectedRevision: a.Revision, Reason: "Explicit cancellation"}, "cli", f.now)
	if err != nil {
		f.t.Fatal(err)
	}
	var result model.ActionRecord
	json.Unmarshal(raw, &result)
	return result
}

func TestActionIntentDispatchInterruptionLateResultAndReplay(t *testing.T) {
	f := setup(t)
	request := f.request()
	queued := f.queue(request)
	if queued.Execution != "queued" || queued.Verification != "pending" || !reflect.DeepEqual(queued, f.action(request.ID)) {
		t.Fatal(queued, f.action(request.ID))
	}
	first, _ := f.a.Queue(f.ctx, request, "cli", f.now)
	retry, _ := f.a.Queue(f.ctx, request, "cli", f.now.Add(time.Hour))
	if string(first) != string(retry) {
		t.Fatal("exact queue retry changed original attempt")
	}
	poll := f.message("poll")
	reply := f.send(poll)
	if len(reply.Commands) != 1 || !reflect.DeepEqual(reply.Commands[0].ActionRef, queued.BrowserRef()) {
		t.Fatal(reply)
	}
	if a := f.action(request.ID); a.Execution != "dispatching" || a.DeliveryID != poll.ID {
		t.Fatal(a)
	}
	if repeat := f.send(poll); !reflect.DeepEqual(reply, repeat) {
		t.Fatal("same transport request lost original receipt")
	}
	if len(f.send(f.message("poll")).Commands) != 0 {
		t.Fatal("new poll redelivered uncertain input")
	}
	if err := f.a.Recover(f.ctx, f.now); err != nil {
		t.Fatal(err)
	}
	a := f.action(request.ID)
	if a.Execution != "uncertain" || a.UncertainSince.IsZero() {
		t.Fatal(a)
	}
	if _, err := f.a.Queue(f.ctx, f.request(), "cli", f.now); err == nil {
		t.Fatal("new request bypassed unresolved surface")
	}
	a = f.cancel(a)
	if a.Execution != "uncertain" || !a.CancelRequested || !model.ActionHolds(a) {
		t.Fatal("in-flight cancellation asserted no effect", a)
	}
	if _, err := f.b.Handle(f.ctx, poll, f.now); err == nil {
		t.Fatal("cancelled cached delivery returned commands")
	}
	result := f.message("command_result")
	result.Result = &browser.OperationResult{OperationID: request.ID, ActionRef: queued.BrowserRef(), Status: "succeeded", TabID: 2, WindowID: 2, URL: request.Browser.URL}
	f.send(result)
	a = f.action(request.ID)
	if a.Execution != "api_reported" || a.Verification != "pending" || a.UncertainSince.IsZero() || !a.CancelRequested || !model.ActionHolds(a) {
		t.Fatal("late API report erased uncertainty or verified outcome", a)
	}
	if _, err := f.a.Show(f.ctx, "beta", request.ID); err == nil {
		t.Fatal("foreign action read")
	}
	history, err := f.e.Store.ActionHistory(f.ctx, "alpha", request.ID, 0, 100)
	if err != nil || len(history) != 5 {
		t.Fatal(len(history), err)
	}
	page, err := f.e.Store.ActionHistory(f.ctx, "alpha", request.ID, history[1].ID, 1)
	if err != nil || len(page) != 1 || page[0].ID >= history[1].ID {
		t.Fatal(page, err)
	}
	before, _ := f.e.Store.State(f.ctx)
	replayed, err := f.e.Store.Replay(f.ctx)
	if err != nil || !reflect.DeepEqual(before, replayed) {
		t.Fatal("replay changed action history", err)
	}
	if replayed.Tasks["alpha"].Task.Status != "active" {
		t.Fatal("browser API result completed task")
	}
}

func TestActionQueuedCancellationAndDeliveryGuards(t *testing.T) {
	for _, mode := range []string{"cancel", "task", "unpair", "age", "restart"} {
		t.Run(mode, func(t *testing.T) {
			f := setup(t)
			r := f.request()
			a := f.queue(r)
			if mode == "cancel" {
				cancelled := f.cancel(a)
				if cancelled.Execution != "cancelled" || model.ActionHolds(cancelled) {
					t.Fatal(cancelled)
				}
				f.queue(f.request())
				return
			}
			if mode == "age" {
				f.now = f.now.Add(6 * time.Second)
				if len(f.send(f.message("poll")).Commands) != 0 || f.action(r.ID).Execution != "queued" {
					t.Fatal("stale inventory dispatched")
				}
				f.inventory(nil)
				if len(f.send(f.message("poll")).Commands) != 1 {
					t.Fatal("fresh inventory did not unblock queue")
				}
				return
			}
			poll := f.message("poll")
			f.send(poll)
			switch mode {
			case "task":
				st, _ := f.e.Store.State(f.ctx)
				task := st.Tasks["alpha"].Task
				task.Title = "Changed"
				if _, err := f.e.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "update", Task: &task}, "cli", f.now); err != nil {
					t.Fatal(err)
				}
			case "unpair":
				if _, err := f.b.Control(f.ctx, browser.Control{ID: model.NewID(), Action: "unpair", Profile: f.profile}, f.now); err != nil {
					t.Fatal(err)
				}
			case "restart":
				f.connection = model.NewID()
				hello := f.message("hello")
				hello.ExtensionVersion = "0.3.0"
				hello.ActionProtocol = 1
				hello.VerificationProtocol = 1
				f.send(hello)
			}
			if _, err := f.b.Handle(f.ctx, poll, f.now); err == nil {
				t.Fatal("cached delivery survived authority/context/connection change")
			}
		})
	}
}

func TestActionForgedReportsOldTransportAndLegacyScope(t *testing.T) {
	f := setup(t)
	r := f.request()
	a := f.queue(r)
	f.send(f.message("poll"))
	for _, alter := range []func(*browser.OperationResult){func(r *browser.OperationResult) { r.ActionRef = nil }, func(r *browser.OperationResult) { r.ActionRef.AttemptID = model.NewID() }, func(r *browser.OperationResult) { r.ActionRef.Target = "beta" }, func(r *browser.OperationResult) { r.OperationID = model.NewID() }} {
		m := f.message("command_result")
		m.Result = &browser.OperationResult{OperationID: r.ID, ActionRef: a.BrowserRef(), Status: "succeeded", TabID: 2, WindowID: 2, URL: r.Browser.URL}
		alter(m.Result)
		before, _ := f.e.Store.State(f.ctx)
		if _, err := f.b.Handle(f.ctx, m, f.now); err == nil {
			t.Fatal("forged result accepted")
		}
		after, _ := f.e.Store.State(f.ctx)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("forged result changed state")
		}
	}
	if _, err := f.a.Queue(f.ctx, r, "client:"+model.NewID(), f.now); err == nil {
		t.Fatal("scoped client acquired action authority")
	}
	if _, err := f.b.Control(f.ctx, browser.Control{ID: r.ID, Action: "open", Profile: f.profile, Epoch: f.epoch, URL: r.Browser.URL}, f.now); err == nil {
		t.Fatal("legacy command overwrote shared action ID")
	}
	legacy := actions.Legacy(model.BrowserOperation{ID: model.NewID(), Status: "succeeded"})
	if legacy.Execution != "api_reported" || legacy.Verification != "unsupported" || legacy.Scope != "unscoped_legacy_browser" {
		t.Fatal(legacy)
	}
}

func TestActionRefusalPreventsCachedRedelivery(t *testing.T) {
	f := setup(t)
	r := f.request()
	a := f.queue(r)
	poll := f.message("poll")
	f.send(poll)
	m := f.message("command_result")
	m.Result = &browser.OperationResult{OperationID: r.ID, ActionRef: a.BrowserRef(), Status: "refused", Detail: "Operator paused before input"}
	f.send(m)
	if model.ActionHolds(f.action(r.ID)) {
		t.Fatal("known pre-input refusal retained lock")
	}
	if _, err := f.b.Handle(f.ctx, poll, f.now); err == nil {
		t.Fatal("cached delivery reissued an already refused attempt")
	}
	f.queue(f.request())
}

func TestActionReferencesProtectSnapshotPayloadUntilKnownCancellation(t *testing.T) {
	f := setup(t)
	observer := hyprland.New()
	fake := testdesktop.New()
	observer.Connector = fake.Connect
	t.Cleanup(observer.Close)
	viewport := workspace.ViewportService{Store: f.e.Store, Observer: observer}
	source := workspace.ViewportRequest{Version: 1, ID: model.NewID(), Op: "select", Previous: "none", Source: &workspace.SourceInput{SocketDir: "/synthetic/hypr", Host: strings.Repeat("c", 64), Epoch: strings.Repeat("a", 64)}}
	if _, err := viewport.Execute(f.ctx, source, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	snapshots := workspace.SnapshotService{Store: f.e.Store, Observer: observer}
	st, _ := f.e.Store.State(f.ctx)
	capture := workspace.SnapshotRequest{Version: 1, ID: model.NewID(), Op: "capture", Target: "alpha", Previous: "none", ExpectedTaskRevision: 1, ManifestID: f.manifest, SourceID: source.ID, InputDigest: model.SnapshotInputDigest(st, "alpha")}
	if _, err := snapshots.Execute(f.ctx, capture, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	r := f.request()
	r.SnapshotID = capture.ID
	queued := f.queue(r)
	unpin := workspace.SnapshotRequest{Version: 1, ID: model.NewID(), Op: "unpin", Target: "alpha", Previous: capture.ID, SnapshotID: capture.ID, Reason: "Release explicit manual pin"}
	if _, err := snapshots.Execute(f.ctx, unpin, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	prune := workspace.SnapshotRequest{Version: 1, ID: model.NewID(), Op: "prune", Target: "alpha", Previous: "none", SnapshotIDs: []string{capture.ID}, Reason: "Prune released point"}
	if _, err := snapshots.Execute(f.ctx, prune, "cli", f.now); err == nil {
		t.Fatal("unfinished action lost referenced payload")
	}
	f.cancel(queued)
	if _, err := snapshots.Execute(f.ctx, prune, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	point, err := f.e.Store.WorkspacePoint(f.ctx, "alpha", capture.ID)
	if err != nil || !point.Pruned {
		t.Fatal(point, err)
	}
	// The old queue receipt remains historical even after its point is removed.
	if _, err := f.a.Queue(f.ctx, r, "cli", f.now); err != nil {
		t.Fatal("inert queue retry reread removed payload", err)
	}
	before, _ := f.e.Store.State(f.ctx)
	after, err := f.e.Store.Replay(f.ctx)
	if err != nil || !reflect.DeepEqual(before, after) {
		t.Fatal("snapshot/action reference replay", err)
	}
}

func TestActionDeadlineSweepDoesNotRedispatchOrGrowWhenIdle(t *testing.T) {
	for _, dispatch := range []bool{false, true} {
		t.Run(fmt.Sprint(dispatch), func(t *testing.T) {
			f := setup(t)
			r := f.request()
			f.queue(r)
			if dispatch {
				f.send(f.message("poll"))
			}
			if err := f.a.Sweep(f.ctx, f.now.Add(31*time.Second)); err != nil {
				t.Fatal(err)
			}
			a := f.action(r.ID)
			if dispatch {
				if a.Execution != "uncertain" || !model.ActionHolds(a) {
					t.Fatal(a)
				}
			} else if a.Execution != "refused" || model.ActionHolds(a) {
				t.Fatal(a)
			}
			before, _ := f.e.Store.Events(f.ctx)
			if err := f.a.Sweep(f.ctx, f.now.Add(time.Hour)); err != nil {
				t.Fatal(err)
			}
			after, _ := f.e.Store.Events(f.ctx)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("idle coordinator grew the event log")
			}
		})
	}
}

// Existing action-journal tests now run on the current challenged transport.
// Supply a synthetic read only for queued intents; outcome checks stay separate.
func (f *fixture) prepareReadback() {
	if !f.autoRead {
		return
	}
	st, _ := f.e.Store.State(f.ctx)
	queued := false
	for _, a := range st.Actions {
		if a.Execution == "queued" && a.Intent.Browser.Profile == f.profile {
			queued = true
		}
	}
	if !queued {
		return
	}
	reply := f.send(f.message("poll"))
	if reply.Challenge == nil {
		f.t.Fatal("queued intent needs challenge", reply)
	}
	p := st.Browsers[f.profile]
	tabs := p.Tabs
	instances := []model.BrowserInstance{}
	for i, t := range tabs {
		if a, ok := st.Actions[t.OwnerID]; ok {
			instances = append(instances, model.BrowserInstance{TabID: t.ID, ActionRef: *a.BrowserRef()})
		}
		tabs[i].OwnerID = ""
	}
	present := []int{}
	for _, t := range tabs {
		present = append(present, t.ID)
	}
	complete, stable, focus := true, true, p.FocusedWindow
	m := f.message("readback")
	m.ChallengeID = reply.Challenge.ID
	m.Complete = &complete
	m.Stable = &stable
	m.FocusedWindow = &focus
	m.ObservedAt = f.now.Format(time.RFC3339Nano)
	f.sequence++
	m.Sequence = f.sequence
	m.Tabs = tabs
	m.PresentTabs = present
	m.Instances = instances
	f.send(m)
}
