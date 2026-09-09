package actions_test

import (
	"encoding/json"
	"heimdall/internal/actions"
	"heimdall/internal/adapters/hyprland"
	"heimdall/internal/browser"
	"heimdall/internal/model"
	"heimdall/internal/testdesktop"
	"heimdall/internal/workspace"
	"reflect"
	"strings"
	"testing"
	"time"
)

type pairFixture struct {
	*fixture
	service    *workspace.BrowserPairingService
	fake       *testdesktop.Fake
	source     string
	a          model.ActionRecord
	generation int64
}

func pairingSetup(t *testing.T) *pairFixture {
	f := verifiedSetup(t)
	f.now = time.Now().UTC()
	h := f.message("hello")
	h.ExtensionVersion = "0.5.0"
	h.ActionProtocol = 1
	h.VerificationProtocol = 1
	h.PairingProtocol = 1
	h.ExtensionID = strings.Repeat("a", 32)
	f.send(h)
	fake := testdesktop.New()
	observer := hyprland.New()
	observer.Connector = fake.Connect
	t.Cleanup(observer.Close)
	viewport := &workspace.ViewportService{Store: f.e.Store, Observer: observer}
	source := model.NewID()
	_, err := viewport.Execute(f.ctx, workspace.ViewportRequest{Version: 1, ID: source, Op: "select", Previous: "none", Source: &workspace.SourceInput{SocketDir: "/synthetic/hypr", Host: strings.Repeat("c", 64), Epoch: strings.Repeat("a", 64)}}, "cli", f.now)
	if err != nil {
		t.Fatal(err)
	}
	f.b.AssociationCheck = func(p model.BrowserAssociation) error { return observer.Check(p.SnapshotID) }
	service := &workspace.BrowserPairingService{Store: f.e.Store, Browser: &f.b, Observer: observer, Clock: time.Now}
	r := f.request()
	r.Version = 2
	r.Browser.Pairing = &model.BrowserPairingIntent{Version: 1, SourceID: source, SourceEpoch: strings.Repeat("a", 64), PreviousViewport: "none"}
	a := f.queue(r)
	pf := &pairFixture{fixture: f, service: service, fake: fake, source: source, a: a, generation: 1}
	pf.read(false, nil)
	poll := f.send(f.message("poll"))
	if len(poll.Commands) != 1 {
		t.Fatal(poll)
	}
	ready := f.message("pairing_ready")
	ready.PairReady = &model.BrowserPairReady{ActionRef: *a.BrowserRef(), MarkerTabID: 7, WindowID: 3}
	f.send(ready)
	pf.windows(1)
	pf.read(true, nil)
	return pf
}
func (f *pairFixture) windows(count int) {
	rows := []map[string]any{}
	for i := 0; i < count; i++ {
		w := testdesktop.Window([]string{"18000001", "18000002"}[i], []string{"0x100", "0x200"}[i], 1, "planning")
		w["title"] = model.BrowserPairNativeTitle(f.a.Intent.ID)
		rows = append(rows, w)
	}
	f.fake.Set("clients", rows)
}
func (f *pairFixture) read(marker bool, tabs []model.BrowserTab) {
	f.t.Helper()
	f.now = time.Now().UTC()
	ids := []int{}
	instances := []model.BrowserInstance{}
	if marker || len(tabs) > 0 {
		ids = []int{7}
		instances = []model.BrowserInstance{{TabID: 7, ActionRef: *f.a.BrowserRef()}}
	}
	m := f.challenged(tabs, ids, instances, true, true, 3)
	m.EventGeneration = &f.generation
	if marker {
		m.Markers = []model.BrowserMarker{{ActionRef: *f.a.BrowserRef(), TabID: 7, WindowID: 3, Active: true}}
	}
	f.send(m)
}
func (f *pairFixture) observe() {
	f.t.Helper()
	if err := f.service.Observe(f.ctx, f.a.Intent.ID); err != nil {
		f.t.Fatal(err)
	}
}
func TestBrowserNativePairingTwoReadsContinuationAndReplay(t *testing.T) {
	f := pairingSetup(t)
	f.observe()
	a := f.action(f.a.Intent.ID)
	if a.Pairing.Probe == nil || a.Pairing.AssociationID != "" {
		t.Fatal(a)
	}
	if err := f.service.Observe(f.ctx, a.Intent.ID); err == nil {
		t.Fatal("reused first browser read")
	}
	f.read(true, nil)
	f.observe()
	a = f.action(a.Intent.ID)
	if a.Pairing.AssociationID == "" {
		t.Fatal("association missing")
	}
	before, _ := f.e.Store.State(f.ctx)
	if before.ViewportBindings[before.ViewportHeads[f.surface]].BrowserAssociationID != a.Pairing.AssociationID {
		t.Fatal("binding not atomic")
	}
	f.read(true, nil)
	poll := f.message("poll")
	p := f.send(poll)
	if len(p.Continuations) != 1 || len(p.Commands) != 0 {
		t.Fatal(p)
	}
	// A new poll can request readback but can never redeliver this continuation.
	next := f.send(f.message("poll"))
	if len(next.Continuations) != 0 {
		t.Fatal("continuation repeated")
	}
	result := f.message("command_result")
	result.Result = &browser.OperationResult{OperationID: a.Intent.ID, ActionRef: a.BrowserRef(), ContinuationID: p.Continuations[0].ID, Status: "succeeded", TabID: 7, WindowID: 3, URL: a.Intent.Browser.URL}
	f.send(result)
	f.read(false, []model.BrowserTab{{ID: 7, WindowID: 3, URL: a.Intent.Browser.URL, Active: true}})
	a = f.action(a.Intent.ID)
	if a.Verification != "matched" || a.Execution != "api_reported" {
		t.Fatal(a)
	}
	state, _ := f.e.Store.State(f.ctx)
	reads, _ := f.fake.Count()
	if _, err := f.e.Replay(f.ctx); err != nil {
		t.Fatal(err)
	}
	after, _ := f.e.Store.State(f.ctx)
	got, _ := f.fake.Count()
	if !reflect.DeepEqual(state, after) || reads != got {
		t.Fatal("replay changed state or read adapter")
	}
	// Rebinding source invalidates current association status without deleting history.
	proof := state.BrowserAssociations[a.Pairing.AssociationID]
	w := model.DesktopWindow{Identity: proof.Window, Title: "Private unrelated page", PID: 999, Class: "Chromium"}
	b := state.ViewportBindings[state.ViewportHeads[f.surface]]
	clean, ok := model.ScopedBrowserWindow(state, b, w)
	if !ok || clean.Title != "" || clean.PID != 0 || clean.Class != "" {
		t.Fatal("browser content leaked", clean)
	}
}
func TestBrowserPairingAmbiguityABAAndCancellation(t *testing.T) {
	t.Run("duplicate nonce", func(t *testing.T) {
		f := pairingSetup(t)
		f.windows(2)
		if err := f.service.Observe(f.ctx, f.a.Intent.ID); err == nil {
			t.Fatal("ambiguous native window bound")
		}
		if f.action(f.a.Intent.ID).Pairing.Probe != nil {
			t.Fatal("ambiguous probe persisted")
		}
	})
	t.Run("event generation ABA", func(t *testing.T) {
		f := pairingSetup(t)
		f.observe()
		first := f.action(f.a.Intent.ID).Pairing.Probe.ID
		f.generation += 2
		f.read(true, nil)
		f.observe()
		a := f.action(f.a.Intent.ID)
		if a.Pairing.AssociationID != "" || a.Pairing.Probe.ID == first || a.Pairing.ProbeAttempts != 2 {
			t.Fatal("ABA change inherited proof", a)
		}
		f.read(true, nil)
		f.observe()
	})
	t.Run("cancel before continuation", func(t *testing.T) {
		f := pairingSetup(t)
		f.observe()
		f.read(true, nil)
		f.observe()
		a := f.action(f.a.Intent.ID)
		_, err := f.aService().Cancel(f.ctx, actions.CancelRequest{Version: 1, ID: model.NewID(), Target: "alpha", ActionID: a.Intent.ID, ExpectedRevision: a.Revision, Reason: "cancel pairing"}, "cli", time.Now().UTC())
		if err != nil {
			t.Fatal(err)
		}
		f.read(true, nil)
		p := f.send(f.message("poll"))
		if len(p.Continuations) > 0 {
			t.Fatal("cancelled pairing continued")
		}
		f.inventory(nil)
		f.read(false, nil)
		if model.ActionHolds(f.action(a.Intent.ID)) {
			t.Fatal("explicitly closed marker did not release hold")
		}
	})
	t.Run("source changed", func(t *testing.T) {
		f := pairingSetup(t)
		f.observe()
		f.read(true, nil)
		f.fake.ChangeEpoch()
		f.fake.Disconnect()
		if err := f.service.Observe(f.ctx, f.a.Intent.ID); err == nil {
			t.Fatal("source restart accepted")
		}
	})
}
func (f *pairFixture) aService() actions.Service { return f.fixture.a }
func TestPairingResultCannotForgeContinuation(t *testing.T) {
	f := pairingSetup(t)
	m := f.message("command_result")
	m.Result = &browser.OperationResult{OperationID: f.a.Intent.ID, ActionRef: f.a.BrowserRef(), ContinuationID: model.NewID(), Status: "succeeded", TabID: 7, WindowID: 3, URL: f.a.Intent.Browser.URL}
	if _, err := f.b.Handle(f.ctx, m, f.now); err == nil {
		t.Fatal("unissued continuation result accepted")
	}
	raw, _ := json.Marshal(f.action(f.a.Intent.ID))
	if strings.Contains(string(raw), "api_reported") {
		t.Fatal("forged success persisted")
	}
}

func TestPairingRestartBeforeAndAfterContinuation(t *testing.T) {
	for _, delivered := range []bool{false, true} {
		t.Run(map[bool]string{false: "before delivery", true: "after delivery"}[delivered], func(t *testing.T) {
			f := pairingSetup(t)
			f.observe()
			f.read(true, nil)
			f.observe()
			f.read(true, nil)
			if delivered {
				p := f.send(f.message("poll"))
				if len(p.Continuations) != 1 {
					t.Fatal(p)
				}
			}
			if err := f.fixture.a.Recover(f.ctx, time.Now().UTC()); err != nil {
				t.Fatal(err)
			}
			f.b = *browser.NewService(f.e.Store)
			f.b.Runtime.Clock = func() time.Time { return f.now }
			f.b.AssociationCheck = func(p model.BrowserAssociation) error { return f.service.Observer.Check(p.SnapshotID) }
			f.connection = model.NewID()
			h := f.message("hello")
			h.ExtensionVersion = "0.5.0"
			h.ActionProtocol = 1
			h.VerificationProtocol = 1
			h.PairingProtocol = 1
			h.ExtensionID = strings.Repeat("a", 32)
			f.send(h)
			f.read(true, nil)
			if delivered {
				before := f.action(f.a.Intent.ID)
				_ = f.service.Tick(f.ctx)
				after := f.action(f.a.Intent.ID)
				if before.Pairing.ContinuationID != after.Pairing.ContinuationID || len(f.send(f.message("poll")).Continuations) > 0 {
					t.Fatal("delivered continuation retried")
				}
				return
			}
			f.observe()
			if f.action(f.a.Intent.ID).Pairing.AssociationID != "" {
				t.Fatal("old connection proof survived")
			}
			f.read(true, nil)
			f.observe()
			f.read(true, nil)
			p := f.send(f.message("poll"))
			if len(p.Continuations) != 1 {
				t.Fatal("known undelivered continuation did not recover", p)
			}
		})
	}
}
