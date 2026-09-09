package core

import (
	"fmt"
	"heimdall/internal/model"
	"heimdall/internal/resourceobs"
	"path/filepath"
	"reflect"
	"sort"
)

// materialize runs within the task transaction. It never executes a check.
func (b *builder) materialize() error {
	ids := make([]string, 0, len(b.state.Tasks))
	for id := range b.state.Tasks {
		ids = append(ids, id)
	}
	// Ancestor bindings must exist before a descendant freezes its scope.
	depth := func(id string) int {
		n := 0
		for id != "" {
			n++
			id = b.state.Tasks[id].Task.Parent
		}
		return n
	}
	sort.Slice(ids, func(i, j int) bool {
		a, c := depth(ids[i]), depth(ids[j])
		if a != c {
			return a < c
		}
		return ids[i] < ids[j]
	})
	for _, id := range ids {
		r := b.state.Tasks[id]
		if err := b.materializeTarget(id, r.Task.Title+"\n"+r.Task.NextAction, r.Task.Done); err != nil {
			return err
		}
		for _, step := range r.Task.Subtasks {
			if err := b.materializeTarget(id+"#"+step.ID, step.Title+"\n"+r.Task.NextAction, step.Done); err != nil {
				return err
			}
		}
	}
	return nil
}
func (b *builder) materializeTarget(target, objective string, done model.Done) error {
	old := b.state.Contracts[b.state.ContractHeads[target]]
	if old.ID != "" && old.MaterializedFrom == "" {
		return nil
	}
	provenance := fmt.Sprintf("tasks.yaml@%d", b.state.Revision)
	specs := map[string]model.EvaluatorSpec{}
	for _, c := range done.Checks {
		if c.Path == "" {
			continue
		}
		previous := b.state.Evaluators[b.state.EvaluatorHeads[model.EvaluatorKey(target, c.ID)]]
		if previous.ID != "" && previous.MaterializedFrom == "" {
			continue
		}
		if !filepath.IsAbs(c.Path) {
			return fmt.Errorf("check %s path must be absolute", c.ID)
		}
		resource := model.Resource{Version: 1, ID: model.NewID(), Target: target, Kind: "tree", Root: c.Path, Path: ".", Exclude: append([]string{}, c.Exclude...), Active: true, Actor: "cli", At: b.now, MaterializedFrom: provenance}
		if c.Kind == "artifact.exists" || c.Kind == "artifact.digest" {
			resource.Kind = "file"
			resource.Root = filepath.Dir(c.Path)
			resource.Path = filepath.Base(c.Path)
		}
		if err := resourceobs.PrepareResource(b.ctx, &resource); err != nil {
			return fmt.Errorf("check %s: %w", c.ID, err)
		}
		scope, err := model.ResourceScope(b.state, target)
		if err != nil {
			return err
		}
		found := false
		for _, id := range scope {
			v := b.state.Resources[id]
			if v.Kind == resource.Kind && v.Root == resource.Root && v.Path == resource.Path && reflect.DeepEqual(v.Exclude, resource.Exclude) {
				resource = v
				found = true
				break
			}
		}
		if !found {
			if err := b.emit("resource", "bound", resource.ID, resource); err != nil {
				return err
			}
		}
		specs[c.ID] = model.EvaluatorSpec{Kind: c.Kind, ResourceID: resource.ID, Argv: c.Argv, TimeoutSeconds: c.TimeoutSeconds, ExpectedDigest: c.ExpectedDigest, ExpectedCommit: c.ExpectedCommit, RequireClean: c.RequireClean, Env: c.Env}
	}
	if len(specs) == 0 {
		return nil
	}
	scope, err := model.ResourceScope(b.state, target)
	if err != nil {
		return err
	}
	r, _, err := model.ResolveTarget(b.state, target)
	if err != nil {
		return err
	}
	contract := old
	if old.ID == "" || old.TaskRevision != r.Revision || old.Objective != objective || !reflect.DeepEqual(old.Acceptance, done) || !reflect.DeepEqual(old.ResourceIDs, scope) {
		contract = model.Contract{Version: 2, ID: model.NewID(), Target: target, TaskRevision: r.Revision, Previous: old.ID, Objective: objective, Acceptance: done, ResourceIDs: scope, Constraints: []string{}, Actor: "cli", At: b.now, MaterializedFrom: provenance}
		if err := model.ValidContract(contract); err != nil {
			return err
		}
		if err := b.emit("contract", "accepted", contract.ID, contract); err != nil {
			return err
		}
	}
	for _, c := range done.Checks {
		spec, ok := specs[c.ID]
		if !ok {
			continue
		}
		old := b.state.Evaluators[b.state.EvaluatorHeads[model.EvaluatorKey(target, c.ID)]]
		if old.ContractID == contract.ID && reflect.DeepEqual(old.Spec, spec) {
			continue
		}
		d := model.Evaluator{Version: 1, ID: model.NewID(), Target: target, CheckID: c.ID, ContractID: contract.ID, Previous: old.ID, Spec: spec, Digest: model.ContentDigest(spec), Actor: "cli", At: b.now, MaterializedFrom: provenance}
		if err := d.Validate(); err != nil {
			return err
		}
		if !d.Current(b.state) {
			return fmt.Errorf("materialized evaluator not current")
		}
		if err := b.emit("evaluator", "accepted", d.ID, d); err != nil {
			return err
		}
	}
	return nil
}
