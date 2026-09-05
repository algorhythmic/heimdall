package model

import (
	"fmt"
	"sort"
)

func TargetLineage(st State, target string) ([]string, error) {
	r, step, err := ResolveTarget(st, target)
	if err != nil {
		return nil, err
	}
	result := []string{target}
	if step != nil {
		result = append(result, r.Task.ID)
	}
	seen := map[string]bool{r.Task.ID: true}
	for r.Task.Parent != "" {
		id := r.Task.Parent
		if seen[id] {
			return nil, fmt.Errorf("parent cycle")
		}
		seen[id] = true
		var ok bool
		r, ok = st.Tasks[id]
		if !ok {
			return nil, fmt.Errorf("missing parent")
		}
		result = append(result, id)
	}
	return result, nil
}

func ResourceScope(st State, target string) ([]string, error) {
	targets, err := TargetLineage(st, target)
	if err != nil {
		return nil, err
	}
	ids := []string{}
	for id, r := range st.Resources {
		if r.Active && Contains(targets, r.Target) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ids, nil
}
