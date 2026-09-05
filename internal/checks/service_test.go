package checks_test

import (
	"context"
	"encoding/json"
	"heimdall/internal/checks"
	"heimdall/internal/continuity"
	"heimdall/internal/core"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type fixture struct {
	t                          *testing.T
	ctx                        context.Context
	engine                     *core.Engine
	service                    checks.Service
	root                       string
	rev                        int64
	contract, resource, target string
}

func setup(t *testing.T, kind string, step bool) *fixture {
	t.Helper()
	e, err := core.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	e.ValidateEvidence = checks.ValidateTarget
	f := &fixture{t: t, ctx: context.Background(), engine: e, service: checks.Service{Store: e.Store}, root: t.TempDir(), target: "evidence-task"}
	if err = os.WriteFile(filepath.Join(f.root, "input.txt"), []byte("accepted input"), 0600); err != nil {
		t.Fatal(err)
	}
	task := model.Task{ID: f.target, Title: "Verify work", Type: "project", Status: "active", Done: model.Done{Text: "Verify the artifact", Checks: []model.Check{{ID: "proof", Kind: kind}}}}
	if step {
		task.Subtasks = []model.Step{{ID: "verify", Title: "Verify", Status: "open", Done: task.Done}}
		task.Done = model.Done{Text: "Finish steps", Checks: []model.Check{{ID: "steps", Kind: "subtasks_done"}}}
		f.target += "#verify"
	}
	if _, err = e.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "add", Task: &task}, "cli", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	st, _ := e.Store.State(f.ctx)
	f.rev = st.Tasks[task.ID].Revision
	cs := continuity.Service{Store: e.Store}
	resourceRaw, err := cs.Execute(f.ctx, continuity.Request{Version: 1, ID: model.NewID(), Op: "resource.bind", Target: f.target, ExpectedTaskRevision: &f.rev, Resource: &continuity.ResourceInput{Kind: "tree", Root: f.root, Path: "."}}, "cli", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	var resource model.Resource
	if err = json.Unmarshal(resourceRaw, &resource); err != nil {
		t.Fatal(err)
	}
	f.resource = resource.ID
	f.root = resource.Root
	raw, err := cs.Execute(f.ctx, continuity.Request{Version: 1, ID: model.NewID(), Op: "contract.accept", Target: f.target, ExpectedTaskRevision: &f.rev, Contract: &continuity.ContractInput{Previous: "none", Objective: "Verify current inputs", ResourceIDs: []string{resource.ID}}}, "cli", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	var contract model.Contract
	json.Unmarshal(raw, &contract)
	f.contract = contract.ID
	return f
}
func (f *fixture) define(spec model.EvaluatorSpec) model.Evaluator {
	f.t.Helper()
	spec.ResourceID = f.resource
	raw, err := f.service.Accept(f.ctx, checks.Request{Version: 1, ID: model.NewID(), Target: f.target, ExpectedTaskRevision: f.rev, CheckID: "proof", ContractID: f.contract, Previous: "none", Spec: &spec}, time.Now().UTC())
	if err != nil {
		f.t.Fatal(err)
	}
	var d model.Evaluator
	json.Unmarshal(raw, &d)
	return d
}
func (f *fixture) start(d model.Evaluator) (model.Evidence, checks.Request) {
	f.t.Helper()
	req := checks.Request{Version: 1, ID: model.NewID(), Target: f.target, ExpectedTaskRevision: f.rev, EvaluatorID: d.ID}
	e, launch, err := f.service.Start(f.ctx, req, time.Now().UTC())
	if err != nil || !launch {
		f.t.Fatal(e, launch, err)
	}
	return e, req
}
func (f *fixture) state() model.State {
	f.t.Helper()
	st, err := f.engine.Store.State(f.ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	return st
}
func (f *fixture) tick() {
	f.t.Helper()
	if _, err := f.engine.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "tick"}, "scheduler", time.Now().UTC()); err != nil {
		f.t.Fatal(err)
	}
}
func TestArtifactEvidenceRevalidationAndReplay(t *testing.T) {
	f := setup(t, "artifact.exists", false)
	d := f.define(model.EvaluatorSpec{Kind: "artifact.exists"})
	e, req := f.start(d)
	if err := f.service.Execute(f.ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	f.tick()
	st := f.state()
	if st.Evidence[e.ID].Outcome != "matched" || len(st.Proposals) != 1 {
		t.Fatal(st.Evidence, st.Proposals)
	}
	retry, launch, err := f.service.Start(f.ctx, req, time.Now().UTC())
	if err != nil || launch || retry.ID != e.ID {
		t.Fatal("retry launched", launch, err)
	}
	var proposal model.Proposal
	for _, p := range st.Proposals {
		proposal = p
	}
	if err = os.WriteFile(filepath.Join(f.root, "input.txt"), []byte("changed after passing"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = f.engine.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "ratify", Target: proposal.ID, Action: "accept"}, "cli", time.Now().UTC()); err == nil {
		t.Fatal("stale live inputs accepted")
	}
	if err = f.service.Refresh(f.ctx, f.target, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	f.tick()
	st = f.state()
	if st.Proposals[proposal.ID].Status != "superseded" || st.EvidenceInvalidations[e.ID].ID == "" {
		t.Fatal("missing invalidation", st)
	}
	replay, err := f.engine.Store.Replay(f.ctx)
	if err != nil || !reflect.DeepEqual(replay, st) {
		t.Fatal("replay changed evidence", err)
	}
}
func TestStepEvidenceRequiresRatification(t *testing.T) {
	f := setup(t, "artifact.exists", true)
	d := f.define(model.EvaluatorSpec{Kind: "artifact.exists"})
	e, _ := f.start(d)
	if err := f.service.Execute(f.ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	f.tick()
	st := f.state()
	if st.Tasks["evidence-task"].Task.Subtasks[0].Status == "done" {
		t.Fatal("evaluation completed step")
	}
	var proposal string
	for id, p := range st.Proposals {
		if p.Target == f.target {
			proposal = id
		}
	}
	if proposal == "" {
		t.Fatal("no step proposal")
	}
	if _, err := f.engine.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "ratify", Target: proposal, Action: "accept"}, "cli", time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	st = f.state()
	if st.Tasks["evidence-task"].Task.Subtasks[0].Status != "done" || st.Tasks["evidence-task"].Task.Status != "active" {
		t.Fatal("ratification target changed")
	}
}
func TestDefinitionScopeAndForgedEvidence(t *testing.T) {
	f := setup(t, "artifact.exists", false)
	d := f.define(model.EvaluatorSpec{Kind: "artifact.exists"})
	e, _ := f.start(d)
	payload, _ := json.Marshal(e)
	st := f.state()
	if err := store.Apply(&st, store.Event{ID: st.LastEventID + 1, Version: 1, Actor: "client:forged", Subject: "evidence", Verb: "finished", EntityID: e.ID, TS: time.Now().UTC(), Payload: payload}); err == nil {
		t.Fatal("forged evidence accepted")
	}
	wrong := d.Spec
	wrong.ResourceID = model.NewID()
	_, err := f.service.Accept(f.ctx, checks.Request{Version: 1, ID: model.NewID(), Target: f.target, ExpectedTaskRevision: f.rev, CheckID: "proof", ContractID: f.contract, Previous: d.ID, Spec: &wrong}, time.Now().UTC())
	if err == nil {
		t.Fatal("wrong resource allowed")
	}
}
func TestInterruptedEvaluationNeverRelaunches(t *testing.T) {
	f := setup(t, "test.exit", false)
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	d := f.define(model.EvaluatorSpec{Kind: "test.exit", Argv: []string{exe, "-test.run=TestEvaluatorProcess", "--", "heimdall-evaluator", "pass"}, TimeoutSeconds: 5})
	e, req := f.start(d)
	if err = f.service.Recover(f.ctx, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	_, launch, err := f.service.Start(f.ctx, req, time.Now().UTC())
	if err != nil || launch {
		t.Fatal("recovery retried executable", err)
	}
	if f.state().Evidence[e.ID].Outcome != "unknown" {
		t.Fatal("interrupted execution claimed success")
	}
}
func TestObservedTestOutcomes(t *testing.T) {
	for _, mode := range []string{"pass", "fail", "mutate", "flood", "sleep"} {
		t.Run(mode, func(t *testing.T) {
			f := setup(t, "test.exit", false)
			exe, _ := os.Executable()
			d := f.define(model.EvaluatorSpec{Kind: "test.exit", Argv: []string{exe, "-test.run=TestEvaluatorProcess", "--", "heimdall-evaluator", mode}, TimeoutSeconds: 1})
			e, _ := f.start(d)
			if err := f.service.Execute(f.ctx, e.ID); err != nil {
				t.Fatal(err)
			}
			result := f.state().Evidence[e.ID]
			want := "unknown"
			if mode == "pass" {
				want = "matched"
			}
			if mode == "fail" {
				want = "not_matched"
			}
			if result.Outcome != want {
				t.Fatal(result)
			}
			if mode == "pass" && (result.ExitCode == nil || result.OutputDigest == "" || result.ExecutableDigest == "") {
				t.Fatal("missing execution provenance")
			}
		})
	}
}
func TestEvaluatorProcess(t *testing.T) {
	for i, arg := range os.Args {
		if arg != "heimdall-evaluator" || i+1 >= len(os.Args) {
			continue
		}
		switch os.Args[i+1] {
		case "pass":
			os.Stdout.WriteString("observed success")
			os.Exit(0)
		case "fail":
			os.Exit(3)
		case "mutate":
			os.WriteFile("input.txt", []byte("mutated"), 0600)
			os.Exit(0)
		case "flood":
			os.Stdout.WriteString(strings.Repeat("x", 2<<20))
			os.Exit(0)
		case "sleep":
			time.Sleep(10 * time.Second)
			os.Exit(0)
		}
	}
}

func TestRepositoryIdentityChangesWithoutWorkingFileChanges(t *testing.T) {
	f := setup(t, "repo.state", false)
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-c", "safe.directory=" + filepath.ToSlash(f.root), "-c", "user.name=Heimdall fixture", "-c", "user.email=fixture@example.test", "-C", f.root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git fixture: %v %s", err, out)
		}
	}
	git("init")
	git("add", "input.txt")
	git("commit", "-m", "Synthetic input")
	d := f.define(model.EvaluatorSpec{Kind: "repo.state", RequireClean: true})
	e, _ := f.start(d)
	if err := f.service.Execute(f.ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	st := f.state()
	e = st.Evidence[e.ID]
	if e.Outcome != "matched" || e.Repo == nil {
		t.Fatal(e)
	}
	git("commit", "--allow-empty", "-m", "Changed repository identity")
	if err := checks.ValidateEvidence(f.ctx, st, e); err == nil {
		t.Fatal("changed commit reused old evidence")
	}
}

func TestChangedContractAndPartialEvidenceCannotComplete(t *testing.T) {
	f := setup(t, "artifact.exists", false)
	d := f.define(model.EvaluatorSpec{Kind: "artifact.exists"})
	e, _ := f.start(d)
	st := f.state()
	now := time.Now().UTC()
	partial := e
	partial.Status = "finished"
	partial.FinishedAt = &now
	partial.Outcome = "matched"
	payload, _ := json.Marshal(partial)
	if err := store.Apply(&st, store.Event{ID: st.LastEventID + 1, Version: 1, Actor: "evaluator", Subject: "evidence", Verb: "finished", EntityID: e.ID, TS: now, Payload: payload}); err == nil {
		t.Fatal("partial resource coverage accepted")
	}
	if err := f.service.Execute(f.ctx, e.ID); err != nil {
		t.Fatal(err)
	}
	f.tick()
	_, err := (continuity.Service{Store: f.engine.Store}).Execute(f.ctx, continuity.Request{Version: 1, ID: model.NewID(), Op: "contract.accept", Target: f.target, ExpectedTaskRevision: &f.rev, Contract: &continuity.ContractInput{Previous: f.contract, Objective: "New accepted direction", ResourceIDs: []string{f.resource}}}, "cli", time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	f.tick()
	for _, p := range f.state().Proposals {
		if p.Status == "pending" {
			t.Fatal("changed contract retained proposal")
		}
	}
}
