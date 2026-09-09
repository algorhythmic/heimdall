package store

import (
	"fmt"
	"heimdall/internal/model"
	"maps"
)

func applyDependency(st *model.State, e Event) error {
	var d model.TaskDependency
	if err := model.StrictJSON(e.Payload, &d); err != nil {
		return err
	}
	if err := d.Validate(); err != nil {
		return err
	}
	if e.Actor != "cli" || e.Verb != "recorded" || d.ID != e.EntityID || d.Actor != e.Actor || !d.At.Equal(e.TS) {
		return fmt.Errorf("dependency envelope mismatch")
	}
	if _, ok := st.Dependencies[d.ID]; ok {
		return fmt.Errorf("duplicate dependency record")
	}
	t, ok := st.Tasks[d.Target]
	p, exists := st.Tasks[d.DependsOn]
	if !ok || !exists || t.Revision != d.TaskRevision || p.Revision != d.PrerequisiteRevision {
		return fmt.Errorf("dependency endpoint revision mismatch")
	}
	key := model.DependencyKey(d.Target, d.DependsOn)
	if st.DependencyHeads[key] != d.Previous {
		return fmt.Errorf("dependency head mismatch")
	}
	prior := st.Dependencies[d.Previous]
	if prior.Active == d.Active {
		return fmt.Errorf("dependency already in requested state")
	}
	if d.Active {
		count := 0
		for _, id := range st.DependencyHeads {
			v := st.Dependencies[id]
			if v.Target == d.Target && v.Active {
				count++
			}
		}
		if count >= 128 {
			return fmt.Errorf("target dependency limit (128) exceeded")
		}
	}
	candidate := *st
	candidate.Dependencies = maps.Clone(st.Dependencies)
	candidate.DependencyHeads = maps.Clone(st.DependencyHeads)
	candidate.Dependencies[d.ID] = d
	candidate.DependencyHeads[key] = d.ID
	if err := model.ValidateDependencyGraph(candidate); err != nil {
		return err
	}
	st.Dependencies = candidate.Dependencies
	st.DependencyHeads = candidate.DependencyHeads
	return nil
}
