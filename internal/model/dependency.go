package model

import (
	"fmt"
	"strings"
	"time"
)

// Task dependencies describe planning prerequisites, not completion evidence or
// execution authority. Existing hierarchy and step After edges keep their roles.
type TaskDependency struct {
	Version              int       `json:"version"`
	ID                   string    `json:"id"`
	Target               string    `json:"target"`
	DependsOn            string    `json:"depends_on"`
	TaskRevision         int64     `json:"task_revision"`
	PrerequisiteRevision int64     `json:"prerequisite_revision"`
	Previous             string    `json:"previous"`
	Active               bool      `json:"active"`
	Reason               string    `json:"reason"`
	Actor                string    `json:"actor"`
	At                   time.Time `json:"at"`
}

func DependencyKey(target, on string) string { return target + "/" + on }
func (d TaskDependency) Validate() error {
	if !ValidID(d.Target) || !ValidID(d.DependsOn) || d.Target == d.DependsOn || d.TaskRevision < 1 || d.PrerequisiteRevision < 1 || (d.Previous != "" && !OpaqueID.MatchString(d.Previous)) || strings.TrimSpace(d.Reason) == "" || len(d.Reason) > 4096 || d.Actor != "cli" {
		return fmt.Errorf("invalid task dependency")
	}
	return ValidRecord(d.Version, d.ID, d.Target, d.Actor, d.At)
}
func ValidateDependencyGraph(st State) error {
	if len(st.DependencyHeads) == 0 {
		return nil
	}
	edges := map[string][]string{}
	// A parent aggregates descendants. Child -> ancestor prerequisites would
	// therefore create a cycle; reparenting cannot bypass this boundary.
	for id, t := range st.Tasks {
		if t.Task.Parent != "" {
			edges[t.Task.Parent] = append(edges[t.Task.Parent], id)
		}
	}
	for key, id := range st.DependencyHeads {
		d, ok := st.Dependencies[id]
		if !ok || key != DependencyKey(d.Target, d.DependsOn) {
			return fmt.Errorf("invalid dependency head")
		}
		if !d.Active {
			continue
		}
		if _, ok := st.Tasks[d.Target]; !ok {
			return fmt.Errorf("dependency target missing")
		}
		if _, ok := st.Tasks[d.DependsOn]; !ok {
			return fmt.Errorf("dependency prerequisite missing")
		}
		edges[d.Target] = append(edges[d.Target], d.DependsOn)
	}
	colors := map[string]int{}
	var visit func(string) error
	visit = func(id string) error {
		if colors[id] == 1 {
			return fmt.Errorf("cycle in task dependencies and hierarchy")
		}
		if colors[id] == 2 {
			return nil
		}
		colors[id] = 1
		for _, on := range edges[id] {
			if err := visit(on); err != nil {
				return err
			}
		}
		colors[id] = 2
		return nil
	}
	for id := range st.Tasks {
		if err := visit(id); err != nil {
			return err
		}
	}
	return nil
}
func TaskPrerequisiteStatus(st State, target string) string {
	t, ok := st.Tasks[target]
	if !ok {
		return "unavailable"
	}
	if Contains(t.Workflow.Success, t.Task.Status) {
		return "satisfied"
	}
	if Contains(t.Workflow.Dropped, t.Task.Status) {
		return "dropped"
	}
	return "pending"
}
