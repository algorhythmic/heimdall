package actions_test

import (
	"encoding/json"
	"heimdall/internal/actions"
	"heimdall/internal/browser"
	"heimdall/internal/model"
	"reflect"
	"testing"
	"time"
)

func verifiedSetup(t *testing.T) *fixture {
	f := setup(t)
	f.autoRead = false
	f.b = *browser.NewService(f.e.Store)
	f.b.Runtime.Clock = func() time.Time { return f.now }
	h := f.message("hello")
	h.ExtensionVersion = "0.4.0"
	h.ActionProtocol = 1
	h.VerificationProtocol = 1
	f.send(h)
	return f
}
func (f *fixture) challenged(tabs []model.BrowserTab, present []int, instances []model.BrowserInstance, stable, complete bool, focus int) browser.Message {
	f.t.Helper()
	p := f.send(f.message("poll"))
	if p.Challenge == nil || len(p.Commands) != 0 {
		f.t.Fatal("new readback challenge required", p)
	}
	m := f.message("readback")
	m.ChallengeID = p.Challenge.ID
	m.Stable = &stable
	m.Complete = &complete
	m.FocusedWindow = &focus
	m.ObservedAt = f.now.Format(time.RFC3339Nano)
	f.sequence++
	m.Sequence = f.sequence
	m.Tabs = tabs
	m.PresentTabs = present
	m.Instances = instances
	return m
}
func (f *fixture) verifiedOpen() (model.ActionRecord, model.BrowserTab, model.BrowserInstance) {
	f.t.Helper()
	a := f.queue(f.request())
	f.send(f.challenged(nil, nil, nil, true, true, -1))
	p := f.send(f.message("poll"))
	if len(p.Commands) != 1 {
		f.t.Fatal(p)
	}
	result := f.message("command_result")
	result.Result = &browser.OperationResult{ActionRef: a.BrowserRef(), OperationID: a.Intent.ID, Status: "succeeded", TabID: 7, WindowID: 3, URL: a.Intent.Browser.URL}
	f.send(result)
	tab := model.BrowserTab{ID: 7, WindowID: 3, URL: a.Intent.Browser.URL, Title: "Same title", Active: true, LoadStatus: "complete"}
	instance := model.BrowserInstance{TabID: 7, ActionRef: *a.BrowserRef()}
	f.send(f.challenged([]model.BrowserTab{tab}, []int{7}, []model.BrowserInstance{instance}, true, true, 3))
	if got := f.action(a.Intent.ID); got.Verification != "matched" || got.Execution != "api_reported" {
		f.t.Fatal(got)
	}
	return a, tab, instance
}
func TestChallengedBrowserFreshnessAndReplay(t *testing.T) {
	f := verifiedSetup(t)
	a := f.queue(f.request())
	// A newly received offline capture has no challenged readback authority.
	f.inventory(nil)
	m := f.challenged(nil, nil, nil, true, true, -1)
	before, _ := f.e.Store.State(f.ctx)
	bad := m
	bad.ChallengeID = model.NewID()
	if _, err := f.b.Handle(f.ctx, bad, f.now); err == nil {
		t.Fatal("wrong nonce accepted")
	}
	after, _ := f.e.Store.State(f.ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed nonce changed state")
	}
	f.send(m)
	poll := f.message("poll")
	p := f.send(poll)
	if len(p.Commands) != 1 {
		t.Fatal(p)
	}
	f.now = f.now.Add(6 * time.Second)
	if _, err := f.b.Handle(f.ctx, poll, f.now); err == nil {
		t.Fatal("expired cached delivery accepted")
	}
	f.b = *browser.NewService(f.e.Store)
	f.b.Runtime.Clock = func() time.Time { return f.now }
	if _, err := f.b.Handle(f.ctx, m, f.now); err == nil {
		t.Fatal("old daemon challenge accepted")
	}
	if err := f.a.Recover(f.ctx, f.now); err != nil {
		t.Fatal(err)
	}
	if f.action(a.Intent.ID).Execution != "uncertain" {
		t.Fatal("restart lost uncertainty")
	}
	before, _ = f.e.Store.State(f.ctx)
	replayed, err := f.e.Store.Replay(f.ctx)
	if err != nil || !reflect.DeepEqual(before, replayed) {
		t.Fatal("replay", err)
	}
}
func TestBrowserPostconditionsAndExplicitReconciliation(t *testing.T) {
	for _, kind := range []string{"navigate", "focus", "move", "close"} {
		t.Run(kind, func(t *testing.T) {
			f := verifiedSetup(t)
			owner, tab, instance := f.verifiedOpen()
			r := f.request()
			r.Browser.Action = kind
			r.Browser.TabID = tab.ID
			r.Browser.OwnerID = owner.Intent.ID
			r.Browser.ExpectedURL = tab.URL
			r.Browser.URL = ""
			if kind == "navigate" {
				r.Browser.URL = "https://example.test/next"
				r.Browser.LoadCondition = "complete"
			}
			if kind == "move" {
				r.Browser.WindowID = 9
			}
			a := f.queue(r)
			f.send(f.challenged([]model.BrowserTab{tab}, []int{7}, []model.BrowserInstance{instance}, true, true, 3))
			p := f.send(f.message("poll"))
			if len(p.Commands) != 1 {
				t.Fatal(p)
			}
			report := f.message("command_result")
			report.Result = &browser.OperationResult{ActionRef: a.BrowserRef(), OperationID: a.Intent.ID, Status: "succeeded"}
			f.send(report)
			// The API result itself has no independent success status.
			if f.action(a.Intent.ID).Verification != "pending" {
				t.Fatal("API result verified itself")
			}
			focus := 3
			if kind == "focus" {
				focus = 9
			}
			f.send(f.challenged([]model.BrowserTab{tab}, []int{7}, []model.BrowserInstance{instance}, true, true, focus))
			failed := f.action(a.Intent.ID)
			if failed.Verification != "not_matched" {
				t.Fatal(kind, failed)
			}
			c := actions.CancelRequest{Version: 1, ID: model.NewID(), Target: "alpha", ActionID: a.Intent.ID, ExpectedRevision: failed.Revision, Reason: "Observe corrected outcome without retrying input"}
			receipt, err := f.a.Reconcile(f.ctx, c, "cli", f.now)
			if err != nil {
				t.Fatal(err)
			}
			var restored model.ActionRecord
			json.Unmarshal(receipt, &restored)
			if restored.Intent.AttemptID != a.Intent.AttemptID {
				t.Fatal("reconcile made new attempt")
			}
			tabs, present, instances := []model.BrowserTab{tab}, []int{7}, []model.BrowserInstance{instance}
			if kind == "navigate" {
				tabs[0].URL = r.Browser.URL
			}
			if kind == "move" {
				tabs[0].WindowID = 9
			}
			if kind == "close" {
				tabs = nil
				present = nil
				instances = nil
			}
			f.send(f.challenged(tabs, present, instances, true, true, 3))
			if f.action(a.Intent.ID).Verification != "matched" {
				t.Fatal(f.action(a.Intent.ID))
			}
			if retry, err := f.a.Reconcile(f.ctx, c, "cli", f.now); err != nil || string(retry) != string(receipt) {
				t.Fatal("reconcile retry", err)
			}
			st, _ := f.e.Store.State(f.ctx)
			replayed, err := f.e.Store.Replay(f.ctx)
			if err != nil || !reflect.DeepEqual(st, replayed) {
				t.Fatal("verified replay", err)
			}
			if st.Tasks["alpha"].Task.Status != "active" {
				t.Fatal("browser metadata completed task")
			}
		})
	}
}
func TestCloseOutsideURLCoverageAndPartialReadbackStayUnknown(t *testing.T) {
	f := verifiedSetup(t)
	owner, tab, instance := f.verifiedOpen()
	r := f.request()
	r.Browser.Action = "close"
	r.Browser.URL = ""
	r.Browser.ExpectedURL = tab.URL
	r.Browser.TabID = tab.ID
	r.Browser.OwnerID = owner.Intent.ID
	a := f.queue(r)
	f.send(f.challenged([]model.BrowserTab{tab}, []int{7}, []model.BrowserInstance{instance}, true, true, 3))
	f.send(f.message("poll"))
	result := f.message("command_result")
	result.Result = &browser.OperationResult{OperationID: a.Intent.ID, ActionRef: a.BrowserRef(), Status: "succeeded"}
	f.send(result)
	f.send(f.challenged(nil, []int{7}, []model.BrowserInstance{instance}, true, true, 3))
	if f.action(a.Intent.ID).Verification != "unknown" {
		t.Fatal("filtered URL counted as closure")
	}
	f.send(f.challenged(nil, nil, nil, true, false, 3))
	if f.action(a.Intent.ID).Verification != "unknown" {
		t.Fatal("partial census counted as closure")
	}
	f.send(f.challenged(nil, nil, nil, false, true, 3))
	if f.action(a.Intent.ID).Verification != "unknown" {
		t.Fatal("unstable census counted as closure")
	}
	f.send(f.challenged(nil, nil, nil, true, true, 3))
	if f.action(a.Intent.ID).Verification != "matched" {
		t.Fatal("complete absence not verified")
	}
}
func TestMonotonicBrowserDeadlineCannotBeExtendedByWallClock(t *testing.T) {
	f := verifiedSetup(t)
	f.queue(f.request())
	m := f.challenged(nil, nil, nil, true, true, -1)
	mono := f.now
	f.b.Runtime.Clock = func() time.Time { return mono }
	mono = mono.Add(6 * time.Second)
	if _, err := f.b.Handle(f.ctx, m, f.now); err == nil {
		t.Fatal("frozen wall clock extended monotonic challenge")
	}
}

func TestUnsettledAndMismatchedUncertainBrowserAttemptsRemainHeld(t *testing.T) {
	f := verifiedSetup(t)
	a := f.queue(f.request())
	f.send(f.challenged(nil, nil, nil, true, true, -1))
	f.send(f.message("poll"))
	// A read before the journal result is recovered cannot settle the attempt.
	f.send(f.challenged(nil, nil, nil, true, true, -1))
	if f.action(a.Intent.ID).Verification != "unknown" {
		t.Fatal("missing API settlement inferred")
	}
	report := f.message("command_result")
	report.Result = &browser.OperationResult{ActionRef: a.BrowserRef(), OperationID: a.Intent.ID, Status: "uncertain", TabID: 7, WindowID: 3, URL: a.Intent.Browser.URL}
	f.send(report)
	for i := 0; i < 8; i++ {
		before, _ := f.e.Store.State(f.ctx)
		report.ID = model.NewID()
		f.send(report)
		after, _ := f.e.Store.State(f.ctx)
		if !reflect.DeepEqual(before, after) {
			t.Fatal("same retained result generated another transition")
		}
		f.send(f.challenged(nil, nil, nil, true, true, -1))
	}
	a = f.action(a.Intent.ID)
	if a.Verification != "not_matched" || !model.ActionHolds(a) {
		t.Fatal("negative readback released uncertain side effect", a)
	}
	if p := f.send(f.message("poll")); p.Challenge != nil || len(p.Commands) > 0 {
		t.Fatal("bounded reconciliation kept polling", p)
	}
	if _, err := f.a.Queue(f.ctx, f.request(), "cli", f.now); err == nil {
		t.Fatal("new ID bypassed uncertain attempt")
	}
}
func TestReadbackRejectsForeignInstancesAndStaleSequences(t *testing.T) {
	f := verifiedSetup(t)
	owner, tab, instance := f.verifiedOpen()
	r := f.request()
	r.Browser.Action = "focus"
	r.Browser.URL = ""
	r.Browser.TabID = tab.ID
	r.Browser.OwnerID = owner.Intent.ID
	r.Browser.ExpectedURL = tab.URL
	f.queue(r)
	m := f.challenged([]model.BrowserTab{tab}, []int{7}, []model.BrowserInstance{instance}, true, true, 3)
	before, _ := f.e.Store.State(f.ctx)
	for _, mode := range []string{"target", "attempt", "duplicate", "census", "sequence"} {
		raw, _ := json.Marshal(m)
		var bad browser.Message
		json.Unmarshal(raw, &bad)
		switch mode {
		case "target":
			bad.Instances[0].ActionRef.Target = "beta"
		case "attempt":
			bad.Instances[0].ActionRef.AttemptID = model.NewID()
		case "duplicate":
			bad.Instances = append(bad.Instances, bad.Instances[0])
		case "census":
			bad.PresentTabs = nil
		case "sequence":
			bad.Sequence = 1
		}
		if _, err := f.b.Handle(f.ctx, bad, f.now); err == nil {
			t.Fatal(mode, "accepted")
		}
		after, _ := f.e.Store.State(f.ctx)
		if !reflect.DeepEqual(before, after) {
			t.Fatal(mode, "changed state")
		}
	}
	f.send(m)
}

func TestReportedBrowserOutcomeBecomesUnknownAfterEpochLoss(t *testing.T) {
	for _, unpair := range []bool{false, true} {
		t.Run(map[bool]string{false: "epoch", true: "unpair"}[unpair], func(t *testing.T) {
			f := verifiedSetup(t)
			a := f.queue(f.request())
			f.send(f.challenged(nil, nil, nil, true, true, -1))
			f.send(f.message("poll"))
			r := f.message("command_result")
			r.Result = &browser.OperationResult{ActionRef: a.BrowserRef(), OperationID: a.Intent.ID, Status: "succeeded", TabID: 7, WindowID: 3, URL: a.Intent.Browser.URL}
			f.send(r)
			if unpair {
				if _, err := f.b.Control(f.ctx, browser.Control{ID: model.NewID(), Action: "unpair", Profile: f.profile}, f.now); err != nil {
					t.Fatal(err)
				}
			} else {
				f.epoch = model.NewID()
				f.connection = model.NewID()
				h := f.message("hello")
				h.ExtensionVersion = "0.4.0"
				h.ActionProtocol = 1
				h.VerificationProtocol = 1
				f.send(h)
			}
			a = f.action(a.Intent.ID)
			if a.Execution != "api_reported" || a.Verification != "unknown" || !model.ActionHolds(a) {
				t.Fatal(a)
			}
			st, _ := f.e.Store.State(f.ctx)
			replay, err := f.e.Store.Replay(f.ctx)
			if err != nil || !reflect.DeepEqual(st, replay) {
				t.Fatal("source-loss replay", err)
			}
		})
	}
}

func TestFreshOpenCannotDuplicateAnOwnedLogicalSurface(t *testing.T) {
	f := verifiedSetup(t)
	_, tab, instance := f.verifiedOpen()
	a := f.queue(f.request())
	f.send(f.challenged([]model.BrowserTab{tab}, []int{tab.ID}, []model.BrowserInstance{instance}, true, true, tab.WindowID))
	if p := f.send(f.message("poll")); len(p.Commands) != 0 {
		t.Fatal("second open delivered")
	}
	if f.action(a.Intent.ID).Execution != "refused" {
		t.Fatal("existing owned surface not refused")
	}
}
