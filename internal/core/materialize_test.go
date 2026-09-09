package core

import (
	"encoding/json"
	"gopkg.in/yaml.v3"
	"heimdall/internal/checks"
	"heimdall/internal/continuity"
	"heimdall/internal/model"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestYAMLCheckMaterializationReplayAndOverride(t *testing.T) {
	e := openTest(t)
	root := t.TempDir()
	file := filepath.Join(root, "proof.txt")
	if err := os.WriteFile(file, []byte("proof"), 0600); err != nil {
		t.Fatal(err)
	}
	d := model.Document{Version: 1, Tasks: []model.Task{{ID: "materialized", Title: "Proof", Type: "project", Status: "active", Done: model.Done{Text: "File exists", Checks: []model.Check{{ID: "proof", Kind: "artifact.exists", Path: file}}}}}}
	save := func() {
		t.Helper()
		data, err := yaml.Marshal(d)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(e.Dir, "tasks.yaml"), data, 0600); err != nil {
			t.Fatal(err)
		}
		if err = e.ReconcileFile(ctx, fixed); err != nil {
			t.Fatal(err)
		}
	}
	save()
	st, _ := e.Store.State(ctx)
	if len(st.Resources) != 1 || len(st.Contracts) != 1 || len(st.Evaluators) != 1 || len(st.Evidence) != 0 {
		t.Fatalf("materialization counts: %+v", st)
	}
	head := st.Evaluators[st.EvaluatorHeads["materialized/proof"]]
	if !head.Current(st) || head.MaterializedFrom != "tasks.yaml@1" {
		t.Fatal(head)
	}
	events, _ := e.Store.Events(ctx)
	verbs := []string{}
	for _, ev := range events {
		if ev.Subject == "resource" || ev.Subject == "contract" || ev.Subject == "evaluator" {
			verbs = append(verbs, ev.Subject+"."+ev.Verb)
			if ev.Actor != "cli" {
				t.Fatal(ev)
			}
		}
	}
	if !reflect.DeepEqual(verbs, []string{"resource.bound", "contract.accepted", "evaluator.accepted"}) {
		t.Fatal(verbs)
	}
	// Normalize generated identities and machine paths while retaining the derived
	// record shapes and links as a reviewed golden event fixture.
	canonical := map[string]string{}
	for id := range st.Resources {
		canonical[id] = "resource"
	}
	for id := range st.Contracts {
		canonical[id] = "contract"
	}
	for id := range st.Evaluators {
		canonical[id] = "evaluator"
	}
	var normalize func(any) any
	normalize = func(v any) any {
		switch x := v.(type) {
		case string:
			if name, ok := canonical[x]; ok {
				return name
			}
			if x == root {
				return "<root>"
			}
			if x == file {
				return "<root>/proof.txt"
			}
			return x
		case map[string]any:
			for key, value := range x {
				if key == "digest" {
					x[key] = "<digest>"
				} else {
					x[key] = normalize(value)
				}
			}
			return x
		case []any:
			for i, value := range x {
				x[i] = normalize(value)
			}
			return x
		}
		return v
	}
	golden := []any{}
	for _, ev := range events {
		if ev.Subject == "resource" || ev.Subject == "contract" || ev.Subject == "evaluator" {
			var payload any
			if err := json.Unmarshal(ev.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			golden = append(golden, map[string]any{"subject": ev.Subject, "verb": ev.Verb, "actor": ev.Actor, "payload": normalize(payload)})
		}
	}
	actual, _ := json.MarshalIndent(golden, "", "  ")
	actual = append(actual, '\n')
	goldenPath := "../../testdata/materialization/events.golden.json"
	if os.Getenv("HEIMDALL_UPDATE_MATERIALIZATION_GOLDEN") == "1" {
		if err := os.WriteFile(goldenPath, actual, 0600); err != nil {
			t.Fatal(err)
		}
	}
	expectedGolden, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != string(expectedGolden) {
		t.Fatalf("materialization golden drift:\n%s", actual)
	}
	before, _ := json.Marshal(st)
	after, err := e.Replay(ctx)
	if err != nil {
		t.Fatal(err)
	}
	replayed, _ := json.Marshal(after)
	if string(before) != string(replayed) {
		t.Fatal("replay drift")
	}
	d = st.Document()
	save()
	same, _ := e.Store.State(ctx)
	if same.LastEventID != st.LastEventID {
		t.Fatal("no-op save emitted records")
	}
	d.Tasks[0].Title = "Revised proof"
	save()
	st, _ = e.Store.State(ctx)
	next := st.Evaluators[st.EvaluatorHeads["materialized/proof"]]
	if next.Previous != head.ID || len(st.Resources) != 1 || !next.Current(st) {
		t.Fatal(next)
	}

	// Exercise actual CLI service acceptance, then edit the YAML again.
	evaluatorID := model.NewID()
	service := checks.Service{Store: e.Store}
	_, err = service.Accept(ctx, checks.Request{Version: 1, ID: evaluatorID, Target: next.Target, ExpectedTaskRevision: st.Tasks["materialized"].Revision, CheckID: next.CheckID, ContractID: next.ContractID, Previous: next.ID, Spec: &next.Spec}, fixed)
	if err != nil {
		t.Fatal(err)
	}
	d = st.Document()
	d.Tasks[0].Title = "After explicit evaluator"
	save()
	st, _ = e.Store.State(ctx)
	if st.EvaluatorHeads["materialized/proof"] != evaluatorID {
		t.Fatal("explicit evaluator overwritten")
	}
	contract := st.Contracts[st.ContractHeads["materialized"]]
	contractID := model.NewID()
	rev := st.Tasks["materialized"].Revision
	_, err = (continuity.Service{Store: e.Store}).Execute(ctx, continuity.Request{Version: 1, ID: contractID, Op: "contract.accept", Target: "materialized", ExpectedTaskRevision: &rev, Contract: &continuity.ContractInput{Previous: contract.ID, Objective: "Explicit review", ResourceIDs: contract.ResourceIDs}}, "cli", fixed)
	if err != nil {
		t.Fatal(err)
	}
	st, _ = e.Store.State(ctx)
	d = st.Document()
	d.Tasks[0].Title = "After explicit contract"
	save()
	st, _ = e.Store.State(ctx)
	if st.ContractHeads["materialized"] != contractID || st.EvaluatorHeads["materialized/proof"] != evaluatorID {
		t.Fatal("explicit definitions overwritten")
	}

}

func TestMaterializationFailureRollsBackTaskAndBindings(t *testing.T) {
	e := openTest(t)
	root := t.TempDir()
	file := filepath.Join(root, "proof")
	if err := os.WriteFile(file, []byte("proof"), 0600); err != nil {
		t.Fatal(err)
	}
	task := model.Task{ID: "invalid-proof", Title: "Invalid", Type: "project", Done: model.Done{Text: "Proof", Mode: "all", Checks: []model.Check{{ID: "proof", Kind: "artifact.exists", Path: file}, {ID: "missing", Kind: "artifact.exists", Path: filepath.Join(root, "missing")}}}}
	_, err := e.Execute(ctx, Command{ID: model.NewID(), Op: "add", Task: &task}, "cli", fixed)
	if err == nil {
		t.Fatal("missing input accepted")
	}
	st, _ := e.Store.State(ctx)
	if len(st.Tasks)+len(st.Resources)+len(st.Contracts)+len(st.Evaluators) != 0 {
		t.Fatal("partial transaction")
	}
}

func TestMaterializationPreservesDeclaredExclusions(t *testing.T) {
	e := openTest(t)
	task := model.Task{ID: "tree-proof", Title: "Tree", Type: "project", Done: model.Done{Text: "Clean", Checks: []model.Check{{ID: "proof", Kind: "repo.state", Path: t.TempDir(), RequireClean: true, Exclude: []string{"z", "a", "n", "b", "c"}}}}}
	expected := model.Clone(task.Done)
	command(t, e, Command{Op: "add", Task: &task}, fixed)
	st, _ := e.Store.State(ctx)
	if !reflect.DeepEqual(st.Tasks[task.ID].Task.Done, expected) {
		t.Fatal("resource normalization mutated task declaration")
	}
}

func TestMaterializationAncestorScopeAndSteps(t *testing.T) {
	e := openTest(t)
	root := t.TempDir()
	file := filepath.Join(root, "proof")
	if err := os.WriteFile(file, []byte("proof"), 0600); err != nil {
		t.Fatal(err)
	}
	done := model.Done{Text: "Exists", Checks: []model.Check{{ID: "proof", Kind: "artifact.exists", Path: file}}}
	d := model.Document{Version: 1, Tasks: []model.Task{{ID: "a-child", Parent: "z-parent", Title: "Child", Type: "project", Done: done, Subtasks: []model.Step{{ID: "step", Title: "Step", Status: "open", Done: done}}}, {ID: "z-parent", Title: "Parent", Type: "project", Done: done}}}
	command(t, e, Command{Op: "replace", Document: &d}, fixed)
	st, _ := e.Store.State(ctx)
	if len(st.Resources) != 1 || len(st.Evaluators) != 3 {
		t.Fatal("ancestor binding was not reused")
	}
	for _, d := range st.Evaluators {
		if !d.Current(st) {
			t.Fatal("stale descendant scope", d)
		}
	}
}
