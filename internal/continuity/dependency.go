package continuity

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"sort"
	"strings"
	"time"
)

type DependencyRequest struct {
	Version                      int    `json:"version"`
	ID                           string `json:"id"`
	Target                       string `json:"target"`
	DependsOn                    string `json:"depends_on"`
	ExpectedTaskRevision         int64  `json:"expected_task_revision"`
	ExpectedPrerequisiteRevision int64  `json:"expected_prerequisite_revision"`
	Previous                     string `json:"previous"`
	Op                           string `json:"op"`
	Reason                       string `json:"reason"`
}

func (r DependencyRequest) Validate() error {
	if r.Version != 1 || !model.OpaqueID.MatchString(r.ID) || !model.ValidID(r.Target) || !model.ValidID(r.DependsOn) || r.Target == r.DependsOn || r.ExpectedTaskRevision < 1 || r.ExpectedPrerequisiteRevision < 1 || !model.Contains([]string{"add", "remove"}, r.Op) || strings.TrimSpace(r.Reason) == "" || len(r.Reason) > 4096 {
		return fmt.Errorf("invalid dependency request; explicit tasks, revisions, operation and reason required")
	}
	_, err := head(r.Previous)
	return err
}
func DecodeDependency(raw []byte) (DependencyRequest, error) {
	var r DependencyRequest
	if len(raw) > MaxRequest {
		return r, fmt.Errorf("dependency request exceeds 64 KiB")
	}
	if err := model.StrictJSON(raw, &r); err != nil {
		return r, err
	}
	return r, r.Validate()
}
func (s Service) Dependency(ctx context.Context, r DependencyRequest, actor string, now time.Time) (json.RawMessage, error) {
	if actor != "cli" {
		return nil, fmt.Errorf("dependency mutations require CLI authority")
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(r)
	return s.Store.Transact(ctx, "dependency-"+r.ID, actor, raw, now, func(st model.State) (store.Change, error) {
		c := store.Change{Revision: st.Revision}
		t, ok := st.Tasks[r.Target]
		p, exists := st.Tasks[r.DependsOn]
		if !ok || !exists {
			return c, fmt.Errorf("dependency task not found")
		}
		previous, _ := head(r.Previous)
		if t.Revision != r.ExpectedTaskRevision || p.Revision != r.ExpectedPrerequisiteRevision || st.DependencyHeads[model.DependencyKey(r.Target, r.DependsOn)] != previous {
			return c, fmt.Errorf("dependency endpoint revision or relation head changed: %w", store.ErrConflict)
		}
		d := model.TaskDependency{Version: 1, ID: r.ID, Target: r.Target, DependsOn: r.DependsOn, TaskRevision: t.Revision, PrerequisiteRevision: p.Revision, Previous: previous, Active: r.Op == "add", Reason: r.Reason, Actor: actor, At: now.UTC()}
		c.Events = []store.Pending{{Subject: "dependency", Verb: "recorded", EntityID: d.ID, Payload: d}}
		c.Result = d
		return c, nil
	})
}

type DependencyView struct {
	Kind      string                `json:"kind"`
	Status    string                `json:"status"`
	Target    string                `json:"target,omitempty"`
	DependsOn string                `json:"depends_on,omitempty"`
	Title     string                `json:"title,omitempty"`
	Record    *model.TaskDependency `json:"record,omitempty"`
}

// DependencyViews never includes a foreign ID, title, reason, record or status
// when that prerequisite is outside current read scope.
func DependencyViews(st model.State, target string, visible func(string) bool) []DependencyView {
	out := []DependencyView{}
	ids := []string{}
	for _, id := range st.DependencyHeads {
		d := st.Dependencies[id]
		if d.Target == target && d.Active {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool { return st.Dependencies[ids[i]].DependsOn < st.Dependencies[ids[j]].DependsOn })
	restricted := false
	for _, id := range ids {
		d := st.Dependencies[id]
		if !visible(d.DependsOn) {
			restricted = true
			continue
		}
		out = append(out, DependencyView{Kind: "task", Status: model.TaskPrerequisiteStatus(st, d.DependsOn), Target: target, DependsOn: d.DependsOn, Title: st.Tasks[d.DependsOn].Task.Title, Record: &d})
	}
	// Collapse unknown prerequisites to one placeholder; even the count and sort
	// order of inaccessible projects are not exposed.
	if restricted {
		out = append(out, DependencyView{Kind: "task", Status: "restricted"})
	}
	return out
}
func (s Service) DependencyList(ctx context.Context, target string) ([]DependencyView, error) {
	st, err := s.Store.State(ctx)
	if err != nil {
		return nil, err
	}
	if _, ok := st.Tasks[target]; !ok {
		return nil, fmt.Errorf("task not found")
	}
	return DependencyViews(st, target, func(string) bool { return true }), nil
}
func (s Service) DependencyShow(ctx context.Context, target, id string) (model.TaskDependency, error) {
	st, err := s.Store.State(ctx)
	if err != nil {
		return model.TaskDependency{}, err
	}
	d, ok := st.Dependencies[id]
	if !ok || d.Target != target {
		return model.TaskDependency{}, fmt.Errorf("dependency record not found for target")
	}
	return d, nil
}
