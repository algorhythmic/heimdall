package actions_test

import (
	"context"
	"encoding/json"
	"heimdall/internal/actions"
	"heimdall/internal/authz"
	"heimdall/internal/browser"
	"heimdall/internal/model"
	"heimdall/internal/testdesktop"
	"strings"
	"testing"
	"time"
)

type externalBrowserReader struct {
	f    *pairFixture
	tabs []model.BrowserTab
	live bool
}

func (r *externalBrowserReader) ObserveWorkspace(ctx context.Context, target string, surfaces []string, now time.Time) error {
	done := make(chan error, 1)
	go func() { done <- r.f.b.ObserveWorkspace(ctx, target, surfaces, now) }()
	var poll browser.Reply
	for i := 0; i < 100; i++ {
		r.f.now = time.Now().UTC()
		poll = r.f.send(r.f.message("poll"))
		if poll.Challenge != nil {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if poll.Challenge == nil {
		return <-done
	}
	ids := []int{}
	instances := []model.BrowserInstance{}
	for _, tab := range r.tabs {
		ids = append(ids, tab.ID)
		if tab.ID == 7 {
			instances = append(instances, model.BrowserInstance{TabID: 7, ActionRef: *r.f.a.BrowserRef()})
		}
	}
	m := r.f.message("readback")
	m.ChallengeID = poll.Challenge.ID
	stable, complete, focus := true, true, 3
	m.Stable = &stable
	m.Complete = &complete
	m.FocusedWindow = &focus
	r.f.now = time.Now().UTC()
	m.ObservedAt = r.f.now.Format(time.RFC3339Nano)
	r.f.sequence++
	m.Sequence = r.f.sequence
	m.Tabs = r.tabs
	m.PresentTabs = ids
	m.Instances = instances
	m.EventGeneration = &r.f.generation
	r.f.send(m)
	return <-done
}
func (r *externalBrowserReader) RecoveryFresh(p model.BrowserProfile, after int64, now time.Time) bool {
	return r.live && r.f.b.RecoveryFresh(p, after, now)
}
func TestExternalBrowserExactScopeReadbackAndLease(t *testing.T) {
	f := pairingSetup(t)
	f.observe()
	f.read(true, nil)
	f.observe()
	f.read(true, nil)
	poll := f.send(f.message("poll"))
	if len(poll.Continuations) != 1 {
		t.Fatal(poll)
	}
	result := f.message("command_result")
	result.Result = &browser.OperationResult{OperationID: f.a.Intent.ID, ActionRef: f.a.BrowserRef(), ContinuationID: poll.Continuations[0].ID, Status: "succeeded", TabID: 7, WindowID: 3, URL: f.a.Intent.Browser.URL}
	f.send(result)
	tab := model.BrowserTab{ID: 7, WindowID: 3, URL: f.a.Intent.Browser.URL, Title: "Owned fixture", Active: true, LoadStatus: "complete"}
	f.read(false, []model.BrowserTab{tab})
	w := testdesktop.Window("18000001", "0x100", 1, "planning")
	w["title"] = "Owned fixture - Chromium"
	f.fake.Set("clients", []map[string]any{w})
	f.fake.Set("activewindow", w)
	token := strings.Repeat("e", 64)
	grant := model.NewID()
	if _, err := (authz.Service{Store: f.e.Store}).Execute(f.ctx, authz.Request{Version: 1, ID: grant, Op: "grant.issue", Grant: &authz.IssueInput{Name: "Browser WCU", Target: "alpha", ActionWrite: true, TokenHash: authz.HashToken(token), ExpiresAt: time.Now().Add(time.Hour)}}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	f.b.Runtime.Clock = time.Now
	reader := &externalBrowserReader{f: f, tabs: []model.BrowserTab{tab}, live: true}
	service := actions.ExternalService{Store: f.e.Store, Observer: f.service.Observer, Browser: reader}
	req := actions.IntentRequest{Version: 1, ID: model.NewID(), Target: "alpha", SurfaceID: f.surface, Purpose: "Verify owned URL", Expected: model.ActionPostcondition{Kind: "owned_tab_url", URL: tab.URL, RedirectPolicy: "exact", LoadCondition: "complete"}}
	reader.tabs = append(reader.tabs, model.BrowserTab{ID: 9, WindowID: 3, URL: "https://example.test/foreign", Title: "Foreign"})
	if _, err := service.Register(f.ctx, req, token); err == nil {
		t.Fatal("foreign tab expanded input scope")
	}
	reader.tabs = []model.BrowserTab{tab}
	raw, err := service.Register(f.ctx, req, token)
	if err != nil {
		t.Fatal(err)
	}
	var registered actions.Registration
	json.Unmarshal(raw, &registered)
	if registered.Action.Intent.External.Browser == nil {
		t.Fatal("browser not pinned")
	}
	if _, err = service.Report(f.ctx, actions.ReportRequest{Version: 1, ID: model.NewID(), Target: "alpha", IntentID: req.ID, Step: 1, Outcome: "succeeded"}, token); err != nil {
		t.Fatal(err)
	}
	if err = service.Observe(f.ctx, req.ID); err != nil {
		t.Fatal(err)
	}
	a := f.action(req.ID)
	if a.Verification != "matched" || a.Observation.Browser == nil || a.Observation.Native.Window.Title != "" {
		t.Fatal("browser outcome or title scope", a.Verification)
	}
	reader.live = false
	if err = service.Observe(f.ctx, req.ID); err != nil {
		t.Fatal(err)
	}
	if f.action(req.ID).Verification != "unknown" {
		t.Fatal("lost runtime lease verified")
	}
	reader.live = true
	reader.tabs[0].URL = "https://example.test/changed"
	if err = service.Observe(f.ctx, req.ID); err != nil {
		t.Fatal(err)
	}
	if f.action(req.ID).Verification != "not_matched" {
		t.Fatal("URL drift ignored")
	}
	before, _ := f.e.Store.State(f.ctx)
	after, err := f.e.Store.Replay(f.ctx)
	if err != nil || model.ContentDigest(before) != model.ContentDigest(after) {
		t.Fatal("replay", err)
	}
}
