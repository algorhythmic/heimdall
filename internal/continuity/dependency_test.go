package continuity

import (
	"encoding/json"
	"errors"
	"heimdall/internal/core"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"reflect"
	"strings"
	"testing"
	"time"
)

func addDependencyTask(t *testing.T, f *fixture, id, parent string) {
	t.Helper()
	task := model.Task{ID: id, Title: "Task " + id, Type: "project", Status: "active", Parent: parent}
	if _, err := f.e.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "add", Task: &task}, "cli", f.now); err != nil {
		t.Fatal(err)
	}
}
func dependencyRequest(f *fixture, target, on string) DependencyRequest {
	st, _ := f.e.Store.State(f.ctx)
	previous := st.DependencyHeads[model.DependencyKey(target, on)]
	if previous == "" {
		previous = "none"
	}
	return DependencyRequest{Version: 1, ID: model.NewID(), Target: target, DependsOn: on, ExpectedTaskRevision: st.Tasks[target].Revision, ExpectedPrerequisiteRevision: st.Tasks[on].Revision, Previous: previous, Op: "add", Reason: "Explicit planning prerequisite"}
}
func sendDependency(t *testing.T, f *fixture, r DependencyRequest) model.TaskDependency {
	t.Helper()
	raw, err := f.s.Dependency(f.ctx, r, "cli", f.now)
	if err != nil {
		t.Fatal(err)
	}
	var d model.TaskDependency
	json.Unmarshal(raw, &d)
	return d
}
func TestDependencyCyclesRevisionsSatisfactionAndReplay(t *testing.T) {
	f := setup(t)
	addDependencyTask(t, f, "beta", "")
	addDependencyTask(t, f, "gamma", "")
	cp := f.checkpoint(f.contract(), "none")
	cp.Checkpoint.Blockers = []string{"Waiting for a reviewed specification"}
	f.send(cp)
	p := proposeProgress(t, f, ProgressInput{Kind: "decision", Text: "Review a planning decision", ContractID: cp.Checkpoint.ContractID})
	r := dependencyRequest(f, f.target, "beta")
	first := sendDependency(t, f, r)
	sendDependency(t, f, dependencyRequest(f, "beta", "gamma"))
	cycle := dependencyRequest(f, "gamma", f.target)
	before, _ := f.e.Store.State(f.ctx)
	if _, err := f.s.Dependency(f.ctx, cycle, "cli", f.now); err == nil {
		t.Fatal("cycle accepted")
	}
	if after, _ := f.e.Store.State(f.ctx); !reflect.DeepEqual(before, after) {
		t.Fatal("cycle refusal mutated state")
	}
	summary := func() ProgressOverviewItem {
		t.Helper()
		v, err := f.s.ProgressOverview(f.ctx, SummaryOptions{Target: f.target, Sort: "checkpoint", Limit: 25})
		if err != nil || len(v.Items) != 1 {
			t.Fatal(v, err)
		}
		return v.Items[0]
	}
	i := summary()
	if i.Dependencies[0].Status != "pending" || i.Checkpoint.ID != cp.ID || i.NextAction != cp.Checkpoint.NextAction || i.PendingDecisions[0].ID != p.ID {
		t.Fatal(i)
	}
	stale := dependencyRequest(f, "gamma", "beta")
	if _, err := f.e.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "complete", Target: "beta"}, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	if summary().Dependencies[0].Status != "satisfied" {
		t.Fatal("completion did not update derived dependency view")
	}
	if _, err := f.s.Dependency(f.ctx, stale, "cli", f.now); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale prerequisite revision accepted", err)
	}
	if _, err := f.e.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "reopen", Target: "beta"}, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	if summary().Dependencies[0].Status != "pending" {
		t.Fatal("reopened prerequisite remained satisfied")
	}
	remove := dependencyRequest(f, f.target, "beta")
	remove.Op = "remove"
	removed := sendDependency(t, f, remove)
	if removed.Active || removed.Previous != first.ID || len(summary().Dependencies) != 0 {
		t.Fatal(removed)
	}
	sendDependency(t, f, cycle)
	retry := sendDependency(t, f, r)
	if !reflect.DeepEqual(retry, first) {
		t.Fatal("retry replaced original relation receipt")
	}
	bad := r
	bad.Reason = "different"
	if _, err := f.s.Dependency(f.ctx, bad, "cli", f.now); !errors.Is(err, store.ErrConflict) {
		t.Fatal("reused request ID accepted", err)
	}
	if _, err := f.s.Dependency(f.ctx, r, "browser", f.now); err == nil {
		t.Fatal("browser mutation accepted")
	}
	state, _ := f.e.Store.State(f.ctx)
	if state.CheckpointHeads[f.target] != cp.ID || state.Tasks[f.target].Task.Status != "active" {
		t.Fatal("dependency changed local progress/completion")
	}
	replayed, err := f.e.Store.Replay(f.ctx)
	if err != nil || !reflect.DeepEqual(state, replayed) {
		t.Fatal("dependency replay mismatch", err)
	}
}
func TestDependencyReparentValidationIsAtomic(t *testing.T) {
	f := setup(t)
	addDependencyTask(t, f, "beta", "")
	sendDependency(t, f, dependencyRequest(f, f.target, "beta"))
	before, _ := f.e.Store.State(f.ctx)
	task := before.Tasks[f.target].Task
	task.Parent = "beta"
	if _, err := f.e.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "update", Task: &task}, "cli", f.now); err == nil {
		t.Fatal("reparent introduced a dependency cycle")
	}
	if after, _ := f.e.Store.State(f.ctx); !reflect.DeepEqual(before, after) {
		t.Fatal("failed reparent mutated state")
	}
	addDependencyTask(t, f, "a-parent", "")
	addDependencyTask(t, f, "b-required", "")
	addDependencyTask(t, f, "z-child", "a-parent")
	sendDependency(t, f, dependencyRequest(f, "z-child", "b-required"))
	st, _ := f.e.Store.State(f.ctx)
	doc := st.Document()
	for n := range doc.Tasks {
		switch doc.Tasks[n].ID {
		case "a-parent":
			doc.Tasks[n].Parent = "b-required"
		case "z-child":
			doc.Tasks[n].Parent = ""
		}
	}
	if _, err := f.e.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "replace", Document: &doc}, "cli", f.now); err != nil {
		t.Fatal("valid atomic reparent rejected", err)
	}
	after, _ := f.e.Store.State(f.ctx)
	if err := model.ValidateDependencyGraph(after); err != nil {
		t.Fatal(err)
	}
	if replayed, err := f.e.Store.Replay(f.ctx); err != nil || !reflect.DeepEqual(replayed, after) {
		t.Fatal("atomic reparent replay differs", err)
	}
}
func TestScopedProgressSummaryRedactionOrderingAndCursors(t *testing.T) {
	f := setup(t)
	addDependencyTask(t, f, "visible-child", f.target)
	addDependencyTask(t, f, "foreign-private", "")
	sendDependency(t, f, dependencyRequest(f, "visible-child", "foreign-private"))
	st, _ := f.e.Store.State(f.ctx)
	g := model.Grant{ID: model.NewID(), Target: f.target, Subtree: true}
	options := SummaryOptions{Target: f.target, Subtree: true, Sort: "id", Limit: 1}
	first, err := ScopedProgressOverview(st, g, options)
	if err != nil || len(first.Items) != 1 || first.NextCursor == "" {
		t.Fatal(first, err)
	}
	options.Cursor = first.NextCursor
	second, err := ScopedProgressOverview(st, g, options)
	if err != nil || len(second.Items) != 1 || second.Items[0].Target != "visible-child" {
		t.Fatal(second, err)
	}
	b, _ := json.Marshal(second)
	for _, secret := range []string{"foreign-private", "Task foreign", "Explicit planning prerequisite", "prerequisite_revision"} {
		if strings.Contains(string(b), secret) {
			t.Fatal("foreign dependency leaked", secret)
		}
	}
	if len(second.Items[0].Dependencies) != 1 || second.Items[0].Dependencies[0].Status != "restricted" {
		t.Fatal(second)
	}
	wrong := options
	wrong.Sort = "due"
	if _, err := ScopedProgressOverview(st, g, wrong); err == nil {
		t.Fatal("cursor used with another ordering")
	}
	wrongGrant := g
	wrongGrant.ID = model.NewID()
	if _, err := ScopedProgressOverview(st, wrongGrant, options); err == nil {
		t.Fatal("cursor used with another grant")
	}
	changed := model.Clone(st)
	changed.LastEventID++
	if _, err := ScopedProgressOverview(changed, g, options); !errors.Is(err, store.ErrConflict) {
		t.Fatal("stale cursor accepted", err)
	}
	if _, err := ScopedProgressOverview(st, g, SummaryOptions{Target: "*", Sort: "id", Limit: 50}); err == nil {
		t.Fatal("all bypassed scope")
	}
	if _, err := ScopedProgressOverview(st, g, SummaryOptions{Target: "foreign-private", Sort: "id", Limit: 50}); err == nil {
		t.Fatal("foreign root exposed")
	}
	// Explicit ordering uses saved checkpoint time, then task ID. No ranking.
	st.CheckpointHeads[f.target] = "old"
	st.Checkpoints["old"] = model.Checkpoint{ID: "old", At: f.now, Summary: "Older"}
	st.CheckpointHeads["visible-child"] = "new"
	st.Checkpoints["new"] = model.Checkpoint{ID: "new", At: f.now.Add(time.Second), Summary: "Latest"}
	v, err := ScopedProgressOverview(st, g, SummaryOptions{Target: f.target, Subtree: true, Sort: "checkpoint", Limit: 25})
	if err != nil || v.Items[0].Target != "visible-child" {
		t.Fatal(v, err)
	}
}

func TestProgressSummaryPreservesStepAfterAndAllTaskID(t *testing.T) {
	f := setup(t)
	addDependencyTask(t, f, "all", "")
	v, err := f.s.ProgressOverview(f.ctx, SummaryOptions{Target: "all", Sort: "id", Limit: 25})
	if err != nil || len(v.Items) != 1 || v.Items[0].Target != "all" {
		t.Fatal("task named all was treated as global scope", v, err)
	}
	st, _ := f.e.Store.State(f.ctx)
	task := st.Tasks[f.target].Task
	task.Subtasks = []model.Step{{ID: "first", Title: "First", Status: "open"}, {ID: "second", Title: "Second", Status: "open", After: []string{"first"}}}
	if _, err := f.e.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "update", Task: &task}, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	read := func() SummaryStep {
		t.Helper()
		v, err := f.s.ProgressOverview(f.ctx, SummaryOptions{Target: f.target, Sort: "id", Limit: 25})
		if err != nil || len(v.Items[0].StepPrerequisites) != 1 {
			t.Fatal(v, err)
		}
		return v.Items[0].StepPrerequisites[0]
	}
	if read().Status != "pending" {
		t.Fatal(read())
	}
	if _, err := f.e.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "complete", Target: f.target + "#first"}, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	if read().Status != "satisfied" {
		t.Fatal(read())
	}
	if _, err := f.e.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "drop", Target: f.target + "#first"}, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	if read().Status != "dropped" {
		t.Fatal(read())
	}
}
