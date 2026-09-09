package tui

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/continuity"
	"heimdall/internal/core"
	"heimdall/internal/daemon"
	"heimdall/internal/model"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/gdamore/tcell/v2"
)

type fixture struct {
	a       *App
	e       *core.Engine
	service continuity.Service
	call    Call
	now     time.Time
	ctx     context.Context
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	dir := t.TempDir()
	engine, err := core.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { engine.Close() })
	ctx := context.Background()
	now := time.Now().UTC()
	s := &daemon.Server{Engine: engine, Token: strings.Repeat("a", 64), Clock: func() time.Time { return now }}
	server := httptest.NewUnstartedServer(s)
	s.Host = server.Listener.Addr().String()
	server.Start()
	t.Cleanup(server.Close)
	call := func(ctx context.Context, method, path string, body any) ([]byte, error) {
		var r io.Reader
		if body != nil {
			b, _ := json.Marshal(body)
			r = bytes.NewReader(b)
		}
		req, _ := http.NewRequestWithContext(ctx, method, server.URL+path, r)
		req.Header.Set("Authorization", "Bearer "+s.Token)
		req.Header.Set("Content-Type", "application/json")
		resp, err := server.Client().Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("%d: %s", resp.StatusCode, raw)
		}
		return raw, nil
	}
	for _, id := range []string{"alpha", "beta"} {
		_, err := engine.Execute(ctx, core.Command{ID: model.NewID(), Op: "add", Task: &model.Task{ID: id, Title: id + " task", Type: "project", Status: "active", NextAction: "Continue " + id, Subtasks: []model.Step{{ID: "outline", Title: "Outline 漢字 e\u0301", Status: "open", Done: model.Done{Text: "Outline reviewed"}}}}}, "cli", now)
		if err != nil {
			t.Fatal(err)
		}
	}
	screen := tcell.NewSimulationScreen("UTF-8")
	screen.Init()
	screen.SetSize(140, 44)
	t.Cleanup(screen.Fini)
	a := New(screen, call, Options{DataDir: dir, Now: func() time.Time { return now }})
	a.ctx = ctx
	t.Cleanup(func() { a.workers.Wait() })
	f := &fixture{a, engine, continuity.Service{Store: engine.Store}, call, now, ctx}
	f.reload(t)
	return f
}
func (f *fixture) reload(t *testing.T) {
	t.Helper()
	v, err := fetch(f.ctx, f.call, f.a.selected)
	if err != nil {
		t.Fatal(err)
	}
	f.a.data = v
	f.a.connection = "ok"
	f.a.reconcile()
}
func waitResult(t *testing.T, a *App) {
	t.Helper()
	select {
	case r := <-a.results:
		a.apply(r)
	case <-time.After(5 * time.Second):
		t.Fatal("request did not finish")
	}
}
func (f *fixture) contract(t *testing.T, target string) string {
	t.Helper()
	st, _ := f.e.Store.State(f.ctx)
	rev := st.Tasks[rootOf(target)].Revision
	raw, err := f.service.Execute(f.ctx, continuity.Request{Version: 1, ID: model.NewID(), Op: "contract.accept", Target: target, ExpectedTaskRevision: &rev, Contract: &continuity.ContractInput{Previous: "none", Objective: "Review the exact work"}}, "cli", f.now)
	if err != nil {
		t.Fatal(err)
	}
	var c model.Contract
	json.Unmarshal(raw, &c)
	return c.ID
}
func (f *fixture) proposal(t *testing.T) model.ProgressProposal {
	c := f.contract(t, "alpha")
	st, _ := f.e.Store.State(f.ctx)
	raw, err := f.service.Progress(f.ctx, continuity.ProgressRequest{Version: 1, ID: model.NewID(), Target: "alpha", ExpectedTaskRevision: st.Tasks["alpha"].Revision, Proposal: &continuity.ProgressInput{Kind: "decision", Text: "Keep user edits \x1b]52;c;payload\a \u202e <literal>", ContractID: c}}, "cli", f.now)
	if err != nil {
		t.Fatal(err)
	}
	var p model.ProgressProposal
	json.Unmarshal(raw, &p)
	f.reload(t)
	return p
}
func runeKey(r rune) *tcell.EventKey { return tcell.NewEventKey(tcell.KeyRune, r, tcell.ModNone) }
func TestDashboardNavigationUnicodeAndSmallTerminals(t *testing.T) {
	f := newFixture(t)
	f.proposal(t)
	a := f.a
	a.expanded["alpha"] = true
	a.selected = "alpha#outline"
	f.reload(t)
	a.Draw()
	text := a.Text()
	for _, want := range []string{"needs you", "workstreams", "alpha", "outline", "漢字", "\\u001b", "\\u202e"} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q\n%s", want, text)
		}
	}
	if strings.Contains(text, "\x1b") || strings.Contains(text, "\u202e") {
		t.Fatal("terminal control leaked")
	}
	a.loading = true // keep navigation test independent of network timing
	a.key(runeKey('/'))
	for _, r := range "beta" {
		a.key(runeKey(r))
	}
	a.key(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	if a.selected != "beta" {
		t.Fatal(a.selected)
	}
	a.key(tcell.NewEventKey(tcell.KeyEscape, 0, 0))
	a.key(tcell.NewEventKey(tcell.KeyBacktab, 0, 0))
	if a.panel != 0 {
		t.Fatal("panel focus did not move")
	}
	a.key(runeKey('q'))
	if !a.quitting {
		t.Fatal("quit ignored")
	}
	for _, size := range [][2]int{{80, 24}, {45, 14}, {18, 5}, {180, 55}} {
		a.screen.(tcell.SimulationScreen).SetSize(size[0], size[1])
		a.Draw()
		if len(strings.Split(strings.TrimSuffix(a.Text(), "\n"), "\n")) != size[1] {
			t.Fatal("render did not fit resized terminal")
		}
	}
}
func TestReviewLostResponseRetainsExactRequestAndOneEvent(t *testing.T) {
	f := newFixture(t)
	p := f.proposal(t)
	a := f.a
	a.openProgress(need{ID: p.ID, Kind: "decision", Target: "alpha"})
	waitResult(t, a)
	if a.modal.progress == nil {
		t.Fatal(a.modal.err)
	}
	a.modal.fields[0].value = "Reviewed the frozen design"
	original := a.call
	lost := true
	var bodies [][]byte
	a.call = func(ctx context.Context, method, path string, body any) ([]byte, error) {
		raw, _ := json.Marshal(body)
		if method == "POST" {
			bodies = append(bodies, raw)
		}
		v, err := original(ctx, method, path, body)
		if err == nil && method == "POST" && lost {
			lost = false
			return nil, fmt.Errorf("response lost")
		}
		return v, err
	}
	a.reviewProgress(a.modal, "accepted")
	waitResult(t, a)
	d := a.modal
	if d == nil || d.pending == nil || d.err == "" {
		t.Fatal("uncertain request was discarded")
	}
	if _, err := os.Stat(d.journal); err != nil {
		t.Fatal(err)
	}
	// Closing/reopening after a process loss uses the durable original request.
	path := d.journal
	a.modal = nil
	if err := a.OpenRequest(path); err != nil {
		t.Fatal(err)
	}
	a.key(tcell.NewEventKey(tcell.KeyEnter, 0, 0))
	waitResult(t, a)
	if !bytes.Equal(bodies[0], bodies[1]) {
		t.Fatal("retry changed body")
	}
	st, _ := f.e.Store.State(f.ctx)
	if len(st.ProgressReviews) != 1 || len(st.Decisions) != 1 || st.Tasks["alpha"].Task.Status != "active" {
		t.Fatal("duplicate or completion side effect")
	}
}
func TestStaleReviewRefusedWithoutRefreshingModalPreconditions(t *testing.T) {
	f := newFixture(t)
	p := f.proposal(t)
	a := f.a
	a.openProgress(need{ID: p.ID, Kind: "decision", Target: "alpha"})
	waitResult(t, a)
	d := a.modal
	d.fields[0].value = "Reviewed earlier"
	task := d.state.Tasks["alpha"].Task
	task.Title = "Changed after inspection"
	_, err := f.e.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "update", Task: &task}, "cli", f.now)
	if err != nil {
		t.Fatal(err)
	}
	f.reload(t)
	if a.modal != d || d.fields[0].value != "Reviewed earlier" {
		t.Fatal("poll destroyed note")
	}
	a.reviewProgress(d, "accepted")
	waitResult(t, a)
	st, _ := f.e.Store.State(f.ctx)
	if len(st.ProgressReviews) != 0 || !strings.Contains(d.err, "409") {
		t.Fatal("stale review committed", d.err)
	}
}
func TestDraftSurvivesCloseAndConflictWithFrozenIdentity(t *testing.T) {
	f := newFixture(t)
	f.contract(t, "alpha")
	a := f.a
	a.openDraft("alpha")
	waitResult(t, a)
	d := a.modal
	if d.draft == nil {
		t.Fatal(d.err)
	}
	id := d.draft.Request.ID
	d.fields[0].value = "Reviewed planning notes"
	d.fields[1].value = "Write next section"
	if err := a.persistDraft(d); err != nil {
		t.Fatal(err)
	}
	a.modal = nil
	a.openDraft("alpha")
	waitResult(t, a)
	d = a.modal
	if d.draft.Request.ID != id || d.fields[0].value != "Reviewed planning notes" {
		t.Fatal("draft not recovered")
	}
	competitor := model.Clone(d.draft.Request)
	competitor.ID = model.NewID()
	competitor.Checkpoint.Summary = "Saved elsewhere"
	if _, err := f.service.Execute(f.ctx, competitor, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	a.submitForm(d)
	waitResult(t, a)
	if !strings.Contains(d.err, "409") || !d.draft.Locked {
		t.Fatal("conflict lost draft", d.err)
	}
	var saved continuity.Request
	if err := readBounded(d.draft.Path, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.ID != id || saved.Checkpoint.Previous != "none" {
		t.Fatal("preconditions rewritten")
	}
	// External editors cannot rewrite target, request ID or artifact pins.
	changed := model.Clone(saved)
	changed.Target = "beta"
	writePrivate(d.draft.Path, changed, false)
	if d.draft.reload() == nil {
		t.Fatal("foreign editor rewrite accepted")
	}
}
func TestReadOnlyViewsAndRetiredBrowserRoutes(t *testing.T) {
	f := newFixture(t)
	before, _ := f.e.Store.State(f.ctx)
	for _, open := range []func(string){f.a.openStepWithoutProposal, f.a.openFiles, f.a.openWorkspace} {
		open("alpha")
		waitResult(t, f.a)
		f.a.Draw()
		f.a.modal = nil
	}
	after, _ := f.e.Store.State(f.ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("inspection wrote events")
	}
	for _, path := range []string{"/ui/", "/ui/app.js", "/ui/session", "/ui-bootstrap"} {
		if _, err := f.call(f.ctx, "GET", path, nil); err == nil {
			t.Fatal("retired browser route still served", path)
		}
	}
	f.a.openHelp()
	old := f.a.modal.generation
	f.a.modal = nil
	f.a.apply(result{kind: "step", generation: old, value: snapshot{}})
	if f.a.modal != nil {
		t.Fatal("late read reopened dialog")
	}
}
func (a *App) openStepWithoutProposal(target string) { a.openStep(target, "") }
func TestDraftSuccessAndPasteNeverInvokesActions(t *testing.T) {
	f := newFixture(t)
	f.contract(t, "alpha")
	a := f.a
	a.openDraft("alpha")
	waitResult(t, a)
	d := a.modal
	d.fields[0].value = "Saved from terminal"
	d.fields[1].value = "Review result"
	a.submitForm(d)
	waitResult(t, a)
	st, _ := f.e.Store.State(f.ctx)
	if len(st.Checkpoints) != 1 || st.Checkpoints[st.CheckpointHeads["alpha"]].Summary != "Saved from terminal" {
		t.Fatal("checkpoint not saved")
	}
	if _, err := os.Stat(d.draft.Path); !os.IsNotExist(err) {
		t.Fatal("submitted draft not archived")
	}
	files, _ := filepath.Glob(filepath.Join(a.opts.DataDir, "tui-drafts", "*.saved.json"))
	if len(files) != 1 {
		t.Fatal("exact submitted file missing")
	}
	a.pasting = true
	for _, r := range "qaoc" {
		a.key(runeKey(r))
	}
	if a.quitting || a.modal != nil {
		t.Fatal("paste executed shortcut")
	}
}
