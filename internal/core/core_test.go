package core

import (
	"context"
	"encoding/json"
	"gopkg.in/yaml.v3"
	"heimdall/internal/model"
	"os"
	"path/filepath"
	"testing"
	"time"
)

var ctx = context.Background()
var fixed = time.Date(2026, 9, 4, 18, 0, 0, 0, time.UTC)

func openTest(t *testing.T) *Engine {
	t.Helper()
	e, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	return e
}
func command(t *testing.T, e *Engine, c Command, now time.Time) json.RawMessage {
	t.Helper()
	if c.ID == "" {
		c.ID = model.NewID()
	}
	r, err := e.Execute(ctx, c, "cli", now)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func importFixture(t *testing.T, e *Engine) {
	t.Helper()
	b, err := os.ReadFile("../../testdata/tasks.yaml")
	if err != nil {
		t.Fatal(err)
	}
	d, err := model.ParseDocument(b)
	if err != nil {
		t.Fatal(err)
	}
	command(t, e, Command{Op: "replace", Document: &d}, fixed)
}
func pending(st model.State, target string) string {
	for id, p := range st.Proposals {
		if p.Target == target && p.Status == "pending" {
			return id
		}
	}
	return ""
}
func TestCoreFlowReplayAndRestart(t *testing.T) {
	dir := t.TempDir()
	e, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	importFixture(t, e)
	command(t, e, Command{Op: "capture", Line: "heimdall-core/reference: useful design", Pointer: "https://example.test/design", Client: "cli"}, fixed)
	command(t, e, Command{Op: "complete", Target: "heimdall-core#store"}, fixed.Add(time.Minute))
	command(t, e, Command{Op: "complete", Target: "heimdall-core#tasks"}, fixed.Add(2*time.Minute))
	st, _ := e.Store.State(ctx)
	id := pending(st, "heimdall-core")
	if id == "" {
		t.Fatal("no aggregate proposal")
	}
	command(t, e, Command{Op: "ratify", Target: id, Action: "accept"}, fixed.Add(3*time.Minute))
	st, _ = e.Store.State(ctx)
	if st.Tasks["heimdall-core"].Task.Status != "done" {
		t.Fatal("accept did not complete")
	}
	before, _ := json.Marshal(st)
	after, err := e.Replay(ctx)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(after)
	if string(a) != string(before) {
		t.Fatal("replay mismatch")
	}
	events, _ := e.Store.Events(ctx)
	count := len(events)
	e.Close()
	e, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if err = e.ReconcileFile(ctx, fixed); err != nil {
		t.Fatal(err)
	}
	after, _ = e.Store.State(ctx)
	a, _ = json.Marshal(after)
	if string(a) != string(before) {
		t.Fatal("restart mismatch")
	}
	events, _ = e.Store.Events(ctx)
	if len(events) != count {
		t.Fatal("restart emitted file echo")
	}
}
func TestFileConflictAndNoop(t *testing.T) {
	e := openTest(t)
	importFixture(t, e)
	st, _ := e.Store.State(ctx)
	d := st.Document()
	b, _ := yaml.Marshal(d)
	p := filepath.Join(e.Dir, "tasks.yaml")
	if err := os.WriteFile(p, append([]byte("# user comment\n"), b...), 0600); err != nil {
		t.Fatal(err)
	}
	if err := e.ReconcileFile(ctx, fixed); err != nil {
		t.Fatal(err)
	}
	events, _ := e.Store.Events(ctx)
	taskEvents := 0
	for _, v := range events {
		if v.Subject == "task" {
			taskEvents++
		}
	}
	if taskEvents != 3 {
		t.Fatal("noop save emitted task mutations")
	}
	// Save a newer editor draft, then accept an independent CLI change before watcher ingestion.
	d.Tasks[0].Title = "User's unsynced edit"
	b, _ = yaml.Marshal(d)
	_ = os.WriteFile(p, b, 0600)
	command(t, e, Command{Op: "complete", Target: "heimdall-core#store"}, fixed.Add(time.Minute))
	disk, _ := os.ReadFile(p)
	if string(disk) != string(b) {
		t.Fatal("overwrote editor draft")
	}
	if e.ViewError() == "" {
		t.Fatal("no visible conflict")
	}
	if _, err := os.Stat(filepath.Join(e.Dir, "tasks.pending.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := e.ReconcileFile(ctx, fixed); err == nil {
		t.Fatal("accepted stale file revision")
	}
	st, _ = e.Store.State(ctx)
	if st.Tasks["heimdall-core"].Task.Subtasks[0].Status != "done" {
		t.Fatal("accepted command lost")
	}
}
func TestEvidenceAndSilence(t *testing.T) {
	e := openTest(t)
	importFixture(t, e)
	command(t, e, Command{Op: "complete", Target: "jobapp-sensemesh#send"}, fixed)
	command(t, e, Command{Op: "tick"}, fixed.AddDate(0, 0, 15))
	st, _ := e.Store.State(ctx)
	if pending(st, "jobapp-sensemesh") != "" || st.Tasks["jobapp-sensemesh"].Task.Status != "drafting" {
		t.Fatal("silence falsely completed task")
	}
	due := false
	for _, v := range st.Timers {
		if v.Kind == "silence_review" && v.Status == "due" && v.Outcome == "review_required_without_mail_coverage" {
			due = true
		}
	}
	if !due {
		t.Fatal("missing reminder")
	}
	command(t, e, Command{Op: "reopen", Target: "jobapp-sensemesh#send"}, fixed.AddDate(0, 0, 16))
	st, _ = e.Store.State(ctx)
	for _, v := range st.Timers {
		if v.Kind == "silence_review" && v.Status != "cancelled" {
			t.Fatal("reopen did not cancel timer")
		}
	}
	// Manual completion of the child produces a parent proposal; rejection deduplicates.
	command(t, e, Command{Op: "complete", Target: "jobapp-sensemesh"}, fixed)
	st, _ = e.Store.State(ctx)
	id := pending(st, "job-search")
	if id == "" {
		t.Fatal("missing parent proposal")
	}
	command(t, e, Command{Op: "ratify", Target: id, Action: "reject"}, fixed)
	command(t, e, Command{Op: "tick"}, fixed)
	st, _ = e.Store.State(ctx)
	if pending(st, "job-search") != "" {
		t.Fatal("rejected evidence re-proposed")
	}
}
func TestCaptureScopeAndExpiry(t *testing.T) {
	e := openTest(t)
	importFixture(t, e)
	command(t, e, Command{Op: "capture", Line: "unassigned/reference: keep", Pointer: "https://example.test", Client: "one"}, fixed)
	if _, err := e.Execute(ctx, Command{ID: model.NewID(), Op: "capture", Line: "^unassigned/reference: next", Pointer: "https://example.test/next", Client: "two"}, "cli", fixed.Add(time.Minute)); err == nil {
		t.Fatal("origin crossed client scope")
	}
	st, _ := e.Store.State(ctx)
	var id string
	for k := range st.Captures {
		id = k
	}
	command(t, e, Command{Op: "assign", Target: id, Targets: []string{"heimdall-core", "job-search"}}, fixed.Add(time.Minute))
	command(t, e, Command{Op: "tick"}, fixed.AddDate(0, 0, 4))
	st, _ = e.Store.State(ctx)
	if st.Captures[id].Expired || len(st.Captures) != 1 {
		t.Fatal("assignment failed to cancel expiry or fan-out duplicated capture")
	}
	command(t, e, Command{Op: "capture", Line: "unassigned/reference: expire", Pointer: "https://example.test/expire", Client: "one"}, fixed)
	command(t, e, Command{Op: "tick"}, fixed.AddDate(0, 0, 4))
	st, _ = e.Store.State(ctx)
	expired := 0
	for _, c := range st.Captures {
		if c.Expired {
			expired++
		}
	}
	if expired != 1 {
		t.Fatal("wrong expiry count")
	}
}
func TestStaleProposalAndNonVacuous(t *testing.T) {
	e := openTest(t)
	importFixture(t, e)
	st, _ := e.Store.State(ctx)
	if pending(st, "job-search") != "" {
		t.Fatal("incomplete child matched")
	}
	command(t, e, Command{Op: "complete", Target: "heimdall-core#store"}, fixed)
	command(t, e, Command{Op: "complete", Target: "heimdall-core#tasks"}, fixed)
	st, _ = e.Store.State(ctx)
	id := pending(st, "heimdall-core")
	command(t, e, Command{Op: "reopen", Target: "heimdall-core#store"}, fixed)
	if _, err := e.Execute(ctx, Command{ID: model.NewID(), Op: "ratify", Target: id, Action: "accept"}, "cli", fixed); err == nil {
		t.Fatal("stale proposal accepted")
	}
	st, _ = e.Store.State(ctx)
	r := st.Tasks["heimdall-core"]
	r.Task.Subtasks = nil
	st.Tasks["heimdall-core"] = r
	if _, ok := proposalFor(st, "heimdall-core", fixed); ok {
		t.Fatal("empty aggregate matched")
	}
}
func TestViewPublicationDoesNotClobberRace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "tasks.yaml")
	old := []byte("original")
	_ = os.WriteFile(path, old, 0600)
	newEditor := []byte("concurrent editor save")
	err := replaceView(path, []byte("daemon output"), digest(old), func() {
		if err := os.WriteFile(path, newEditor, 0600); err != nil {
			t.Fatal(err)
		}
	})
	if err == nil {
		t.Fatal("concurrent save was not detected")
	}
	b, _ := os.ReadFile(path)
	if string(b) != string(newEditor) {
		t.Fatal("clobbered concurrent file")
	}
	history, _ := filepath.Glob(filepath.Join(dir, "task-file-history", "*.yaml"))
	if len(history) != 1 {
		t.Fatal("missing recoverable original")
	}
}
func TestLogicalRetryAfterRevisionAdvance(t *testing.T) {
	e := openTest(t)
	rev := int64(0)
	c := Command{ID: "retry-test", Op: "add", ExpectedRevision: &rev, Task: &model.Task{Title: "Retry task", Type: "project"}}
	first := command(t, e, c, fixed)
	rev = 1
	again := command(t, e, c, fixed.Add(time.Hour))
	if string(first) != string(again) {
		t.Fatal("retry did not return saved result")
	}
	st, _ := e.Store.State(ctx)
	if len(st.Tasks) != 1 || st.Revision != 1 {
		t.Fatal("retry mutated state")
	}
	c.Task.Title = "Different intent"
	if _, err := e.Execute(ctx, c, "cli", fixed); err == nil {
		t.Fatal("conflicting retry accepted")
	}
}
