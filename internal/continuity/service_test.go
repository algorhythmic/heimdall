package continuity

import (
	"context"
	"encoding/json"
	"errors"
	"heimdall/internal/core"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type fixture struct {
	t      *testing.T
	e      *core.Engine
	s      Service
	ctx    context.Context
	now    time.Time
	target string
	rev    int64
}

func setup(t *testing.T) *fixture {
	t.Helper()
	e, err := core.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	ctx := context.Background()
	now := time.Date(2026, 9, 4, 20, 0, 0, 0, time.UTC)
	task := model.Task{ID: "continuity-task", Title: "Continue implementation", Type: "project", Status: "active", Done: model.Done{Text: "Changes tested and reviewed"}}
	if _, err = e.Execute(ctx, core.Command{ID: model.NewID(), Op: "add", Task: &task}, "cli", now); err != nil {
		t.Fatal(err)
	}
	st, _ := e.Store.State(ctx)
	return &fixture{t, e, Service{e.Store}, ctx, now, task.ID, st.Tasks[task.ID].Revision}
}
func (f *fixture) request(op string) Request {
	return Request{Version: 1, ID: model.NewID(), Op: op, Target: f.target, ExpectedTaskRevision: &f.rev}
}
func (f *fixture) send(r Request) json.RawMessage {
	f.t.Helper()
	b, err := f.s.Execute(f.ctx, r, "cli", f.now)
	if err != nil {
		f.t.Fatal(err)
	}
	return b
}
func (f *fixture) contract() string {
	st, _ := f.e.Store.State(f.ctx)
	previous := st.ContractHeads[f.target]
	if previous == "" {
		previous = "none"
	}
	targets, _ := lineage(st, f.target)
	r := f.request("contract.accept")
	r.Contract = &ContractInput{Previous: previous, Objective: "Continue safely", Constraints: []string{"Preserve user edits"}, ResourceIDs: resourceIDs(st, targets)}
	f.send(r)
	return r.ID
}
func (f *fixture) checkpoint(contract, prev string) Request {
	r := f.request("checkpoint.record")
	r.Checkpoint = &CheckpointInput{Previous: prev, ContractID: contract, Summary: "Implementation started", NextAction: "Run meaningful checks", Blockers: []string{}}
	return r
}

func TestCheckpointResumeDriftReplayAndRetry(t *testing.T) {
	f := setup(t)
	contract := f.contract()
	root := t.TempDir()
	path := filepath.Join(root, "work.txt")
	if err := os.WriteFile(path, []byte("before"), 0600); err != nil {
		t.Fatal(err)
	}
	resource := f.request("resource.bind")
	resource.Resource = &ResourceInput{Kind: "tree", Root: root, Path: "."}
	f.send(resource)
	if b, err := f.s.Context(f.ctx, f.target, 20000); err != nil || !hasIssue(b, "contract_scope_changed") {
		t.Fatal("scope drift was not reported", b, err)
	}
	if _, err := f.s.Execute(f.ctx, f.checkpoint(contract, "none"), "cli", f.now); err == nil {
		t.Fatal("checkpoint accepted changed resource scope")
	}
	contract = f.contract()
	cp := f.checkpoint(contract, "none")
	original := f.send(cp)
	st, _ := f.e.Store.State(f.ctx)
	event := st.LastEventID
	// Once committed, a retry must return the recorded observation, even if the file is gone.
	os.Remove(path)
	if retry := f.send(cp); !reflect.DeepEqual(original, retry) {
		t.Fatal("retry changed checkpoint")
	}
	st, _ = f.e.Store.State(f.ctx)
	if st.LastEventID != event {
		t.Fatal("retry duplicated events")
	}
	os.WriteFile(path, []byte("before"), 0600)
	b, err := f.s.Context(f.ctx, f.target, 20000)
	if err != nil || b.ResumeStatus != "ready" || len(b.Resources) != 1 || b.Resources[0].Status != "matched" {
		t.Fatal(b, err)
	}
	budget := b.EstimatedTokens
	if _, err = f.s.Context(f.ctx, f.target, budget-1); err == nil {
		t.Fatal("small budget omitted context")
	} else {
		var e *BudgetError
		if !errors.As(err, &e) {
			t.Fatal(err)
		}
	}
	before, _ := json.Marshal(st)
	after, err := f.e.Replay(f.ctx)
	serialized, _ := json.Marshal(after)
	if err != nil || string(before) != string(serialized) {
		t.Fatal("replay mismatch", err)
	}
	os.WriteFile(path, []byte("after"), 0600)
	b, err = f.s.Context(f.ctx, f.target, 20000)
	if err != nil || b.ResumeStatus != "needs_review" || !hasIssue(b, "resource_changed") {
		t.Fatal(b, err)
	}
	// A new checkpoint intentionally observes the new working version.
	next := f.checkpoint(contract, cp.ID)
	f.send(next)
	b, err = f.s.Context(f.ctx, f.target, 20000)
	if err != nil || b.ResumeStatus != "ready" {
		t.Fatal(b, err)
	}
	decision := f.request("decision.accept")
	decision.Decision = &DecisionInput{Text: "Use the established adapter"}
	f.send(decision)
	b, _ = f.s.Context(f.ctx, f.target, 20000)
	if !hasIssue(b, "decisions_changed") {
		t.Fatal("accepted decision drift missing")
	}
	task := after.Tasks[f.target].Task
	task.Title = "Changed objective"
	if _, err = f.e.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "update", Task: &task}, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	b, _ = f.s.Context(f.ctx, f.target, 20000)
	if !hasIssue(b, "stale_contract") || !hasIssue(b, "context_changed") {
		t.Fatal("revision drift missing", b)
	}
	bad := f.checkpoint(contract, next.ID)
	if _, err = f.s.Execute(f.ctx, bad, "cli", f.now); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale task write accepted", err)
	}
}
func hasIssue(b Bundle, code string) bool {
	for _, i := range b.Issues {
		if i.Code == code {
			return true
		}
	}
	return false
}
func TestConcurrentHeadAdvancement(t *testing.T) {
	f := setup(t)
	contract := f.contract()
	var successes atomic.Int32
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		r := f.checkpoint(contract, "none")
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := f.s.Execute(f.ctx, r, "cli", f.now); err == nil {
				successes.Add(1)
			} else if !errors.Is(err, store.ErrConflict) {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if successes.Load() != 1 {
		t.Fatal("competing checkpoint heads", successes.Load())
	}
}
func TestAncestorContractAndResourceChanges(t *testing.T) {
	f := setup(t)
	parentContract := f.contract()
	parent := f.target
	child := model.Task{ID: "child-task", Parent: parent, Title: "Child", Type: "project", Status: "active"}
	if _, err := f.e.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "add", Task: &child}, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	st, _ := f.e.Store.State(f.ctx)
	f.target = child.ID
	f.rev = st.Tasks[child.ID].Revision
	cc := f.contract()
	cp := f.checkpoint(cc, "none")
	f.send(cp)
	b, err := f.s.Context(f.ctx, f.target, 20000)
	if err != nil || len(b.Ancestors) != 1 || len(b.Contracts) != 2 {
		t.Fatal(b, err)
	}
	revision := st.Tasks[parent].Revision
	r := Request{Version: 1, ID: model.NewID(), Target: parent, ExpectedTaskRevision: &revision, Op: "contract.accept", Contract: &ContractInput{Previous: parentContract, Objective: "Parent scope changed", Constraints: []string{"New constraint"}}}
	f.send(r)
	b, _ = f.s.Context(f.ctx, f.target, 20000)
	if !hasIssue(b, "context_changed") {
		t.Fatal("ancestor change not surfaced")
	}
}
func TestInputAndAuthorityBoundaries(t *testing.T) {
	f := setup(t)
	r := f.request("contract.accept")
	r.Contract = &ContractInput{Previous: "none", Objective: "Objective"}
	if _, err := f.s.Execute(f.ctx, r, "observer:browser", f.now); err == nil {
		t.Fatal("observer authorized contract")
	}
	body, _ := json.Marshal(r)
	var v map[string]any
	json.Unmarshal(body, &v)
	v["actor"] = "cli"
	bad, _ := json.Marshal(v)
	if _, err := Decode(bad); err == nil {
		t.Fatal("forged actor accepted")
	}
	r.Contract.Previous = ""
	if _, err := f.s.Execute(f.ctx, r, "cli", f.now); err == nil {
		t.Fatal("implicit head accepted")
	}
}
func TestResourceBoundaryAndUnbind(t *testing.T) {
	f := setup(t)
	contract := f.contract()
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "file"), []byte("source"), 0600)
	r := f.request("resource.bind")
	r.Resource = &ResourceInput{Kind: "file", Root: root, Path: "../escape"}
	if _, err := f.s.Execute(f.ctx, r, "cli", f.now); err == nil {
		t.Fatal("escape accepted")
	}
	r.Resource.Path = "file"
	f.send(r)
	contract = f.contract()
	cp := f.checkpoint(contract, "none")
	f.send(cp)
	u := f.request("resource.unbind")
	u.ResourceID = r.ID
	f.send(u)
	b, err := f.s.Context(f.ctx, f.target, 20000)
	if err != nil || !hasIssue(b, "resource_removed") {
		t.Fatal(b, err)
	}
	if _, err = f.s.Execute(f.ctx, u, "cli", f.now); err != nil {
		t.Fatal("unbind retry", err)
	}
}
func TestSymlinkRefusal(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "secret")
	os.WriteFile(outside, []byte("do not read"), 0600)
	if err := os.Symlink(outside, filepath.Join(root, "link")); err != nil {
		t.Skip("symlink creation unavailable:", err)
	}
	if _, err := Observe(context.Background(), model.Resource{Kind: "file", Root: root, Path: "link"}); err == nil {
		t.Fatal("symlink read accepted")
	}
}
