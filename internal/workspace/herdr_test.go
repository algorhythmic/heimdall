package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"heimdall/internal/adapters/herdr"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestHerdrRequestFixtures(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/workspace/herdr-requests.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		Bind    HerdrBindRequest    `json:"bind"`
		Publish HerdrPublishRequest `json:"publish"`
	}
	if err := model.StrictJSON(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	if err := fixtures.Bind.Validate(); err != nil {
		t.Fatal(err)
	}
	if err := fixtures.Publish.Validate(); err != nil {
		t.Fatal(err)
	}
	var r HerdrBindRequest
	if err := model.StrictJSON([]byte(`{"version":1,"actor":"cli","herdr":{"protocol":20}}`), &r); err == nil {
		t.Fatal("caller-supplied observation accepted")
	}
}

type fakeHerdr struct {
	observation           herdr.Observation
	observeErr, reportErr error
	observations, reports int
	tokens                map[string]string
}

func (a *fakeHerdr) Observe(context.Context, string, string, string) (herdr.Observation, error) {
	a.observations++
	return model.Clone(a.observation), a.observeErr
}
func (a *fakeHerdr) Report(_ context.Context, _ herdr.Observation, _ string, _ int64, _ string, tokens map[string]string, _ int, _ bool) error {
	a.reports++
	a.tokens = tokens
	return a.reportErr
}

func setupHerdr(t *testing.T) (*fixture, HerdrService, *fakeHerdr, HerdrBindRequest) {
	f := setup(t)
	_, m := f.manifest("alpha")
	a := &fakeHerdr{observation: herdr.Observation{Locator: model.SessionLocator{Adapter: "herdr", Environment: "local", Host: strings.Repeat("a", 64), SourceEpoch: strings.Repeat("b", 64), SessionID: "/tmp/synthetic-herdr.sock", WorkspaceID: "w1", PaneID: "w1:p1", Platform: "linux", Cwd: "/synthetic"}, Herdr: model.HerdrIdentity{Protocol: 20, ServerVersion: "0.8.2", TerminalID: "terminal-1", TabID: "w1:t1", ShellPID: 123, ShellStart: "12345"}}}
	r := HerdrBindRequest{Version: 1, ID: model.NewID(), Target: "alpha", SurfaceID: m.Surfaces[0].ID, ManifestID: m.ID, Previous: "none", ExpectedTaskRevision: m.TaskRevision, Socket: a.observation.Locator.SessionID, PaneID: a.observation.Locator.PaneID}
	return f, HerdrService{Store: f.e.Store, Adapter: a}, a, r
}

func TestHerdrBindingRefreshPublishReplayAndRetry(t *testing.T) {
	f, s, a, r := setupHerdr(t)
	bind, err := s.Bind(f.ctx, r, "cli", f.now)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := f.e.Store.State(f.ctx)
	check, err := s.Refresh(f.ctx, r.Target, r.SurfaceID, r.ID, f.now)
	if err != nil || check.Status != "current" {
		t.Fatal(check, err)
	}
	after, _ := f.e.Store.State(f.ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("refresh persisted live state")
	}
	p := HerdrPublishRequest{Version: 1, ID: model.NewID(), Target: r.Target, SurfaceID: r.SurfaceID, BindingID: r.ID}
	metadata, err := s.Publish(f.ctx, p, "cli", f.now)
	if err != nil || a.reports != 1 || a.tokens["heimdall_task"] != "alpha" {
		t.Fatal(err, a.tokens)
	}
	a.observeErr = errors.New("offline")
	if retry, err := s.Bind(f.ctx, r, "cli", f.now); err != nil || string(retry) != string(bind) {
		t.Fatal("binding retry observed source again", err)
	}
	if retry, err := s.Publish(f.ctx, p, "cli", f.now); err != nil || string(retry) != string(metadata) || a.reports != 1 {
		t.Fatal("publish retry repeated metadata", err)
	}
	calls := a.observations
	before, _ = f.e.Store.State(f.ctx)
	replayed, err := f.e.Store.Replay(f.ctx)
	if err != nil || !reflect.DeepEqual(before, replayed) || calls != a.observations || a.reports != 1 {
		t.Fatal("replay changed state or invoked adapter", err)
	}
	if replayed.Tasks["alpha"].Task.Status != "active" {
		t.Fatal("metadata completed task")
	}
	check, err = s.Refresh(f.ctx, r.Target, r.SurfaceID, r.ID, f.now)
	if err != nil || check.Status != "stale" {
		t.Fatal(check, err)
	}
	if _, err = s.Refresh(f.ctx, "beta", r.SurfaceID, r.ID, f.now); err == nil {
		t.Fatal("cross-task refresh")
	}
	if _, err = s.Bind(f.ctx, r, "client:"+model.NewID(), f.now); err == nil {
		t.Fatal("client bind")
	}
	if _, err = s.Publish(f.ctx, p, "client:"+model.NewID(), f.now); err == nil {
		t.Fatal("client publish")
	}
}

func TestHerdrStaleAndUnconfirmedPublication(t *testing.T) {
	f, s, a, r := setupHerdr(t)
	if _, err := s.Bind(f.ctx, r, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	p := HerdrPublishRequest{Version: 1, ID: model.NewID(), Target: r.Target, SurfaceID: r.SurfaceID, BindingID: r.ID}
	a.observation.Herdr.TerminalID = "replacement"
	if _, err := s.Publish(f.ctx, p, "cli", f.now); !errors.Is(err, store.ErrConflict) || a.reports != 0 {
		t.Fatal("published to replacement", err)
	}
	a.observation.Herdr.TerminalID = "terminal-1"
	a.reportErr = errors.New("response lost")
	raw, err := s.Publish(f.ctx, p, "cli", f.now)
	if err != nil {
		t.Fatal(err)
	}
	var result MetadataResult
	json.Unmarshal(raw, &result)
	if result.Status != "unconfirmed" || a.reports != 1 {
		t.Fatal(result, a.reports)
	}
	a.reportErr = nil
	if retry, err := s.Publish(f.ctx, p, "cli", f.now); err != nil || string(retry) != string(raw) || a.reports != 1 {
		t.Fatal("uncertain receipt resent", err)
	}
	unbound := f.request("session.unbind", "alpha")
	unbound.Session = &SessionInput{Previous: r.ID, ManifestID: r.ManifestID, SurfaceID: r.SurfaceID}
	f.send(unbound)
	check, err := s.Refresh(f.ctx, r.Target, r.SurfaceID, "", f.now)
	if err != nil || !model.Contains(check.Issues, "session_unbound") {
		t.Fatal(check, err)
	}
	p.ID = model.NewID()
	if _, err := s.Publish(f.ctx, p, "cli", f.now); err == nil || a.reports != 1 {
		t.Fatal("unbound publication", err)
	}
}

func TestMovedTerminalCannotBeAdoptedByAnotherTask(t *testing.T) {
	f, s, a, r := setupHerdr(t)
	if _, err := s.Bind(f.ctx, r, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	_, m := f.manifest("beta")
	a.observation.Locator.WorkspaceID = "w2"
	a.observation.Locator.PaneID = "w2:p3"
	a.observation.Herdr.TabID = "w2:t1"
	r.ID = model.NewID()
	r.Target = "beta"
	r.SurfaceID = m.Surfaces[0].ID
	r.ManifestID = m.ID
	r.ExpectedTaskRevision = m.TaskRevision
	r.PaneID = "w2:p3"
	before, _ := f.e.Store.State(f.ctx)
	if _, err := s.Bind(f.ctx, r, "cli", f.now); err == nil {
		t.Fatal("same terminal adopted via moved public pane ID")
	}
	after, _ := f.e.Store.State(f.ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("rejected bind leaked state")
	}
}

func TestHerdrEventVersionAndObservationCannotBeForgedThroughGenericRequests(t *testing.T) {
	f, s, _, r := setupHerdr(t)
	raw, err := s.Bind(f.ctx, r, "cli", f.now)
	if err != nil {
		t.Fatal(err)
	}
	var b model.SessionBinding
	json.Unmarshal(raw, &b)
	for _, mutate := range []func(*model.SessionBinding){func(b *model.SessionBinding) { b.Version = 1 }, func(b *model.SessionBinding) { b.Version = 99 }, func(b *model.SessionBinding) { b.Herdr = nil }, func(b *model.SessionBinding) { b.Herdr.Protocol = 21 }, func(b *model.SessionBinding) { b.Locator.Environment = "remote" }} {
		copy := model.Clone(b)
		mutate(&copy)
		if err := model.ValidSessionBinding(copy); err == nil {
			t.Fatal("forged observation record")
		}
	}
	generic := f.request("session.bind", "alpha")
	generic.Session = &SessionInput{Previous: r.ID, ManifestID: r.ManifestID, SurfaceID: r.SurfaceID, Locator: b.Locator}
	f.reject(generic)
}
