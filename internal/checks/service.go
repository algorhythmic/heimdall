// Package checks records independently observed, version-bound completion evidence.
package checks

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"sort"
	"time"
)

type Request struct {
	Version              int                  `json:"version"`
	ID                   string               `json:"id"`
	Target               string               `json:"target"`
	ExpectedTaskRevision int64                `json:"expected_task_revision"`
	CheckID              string               `json:"check_id,omitempty"`
	ContractID           string               `json:"contract_id,omitempty"`
	Previous             string               `json:"previous,omitempty"`
	Spec                 *model.EvaluatorSpec `json:"spec,omitempty"`
	EvaluatorID          string               `json:"evaluator_id,omitempty"`
}
type Service struct{ Store *store.Store }

func (s Service) Accept(ctx context.Context, r Request, now time.Time) (json.RawMessage, error) {
	if r.Version != 1 || !model.OpaqueID.MatchString(r.ID) || r.Spec == nil || r.EvaluatorID != "" || r.ExpectedTaskRevision < 1 {
		return nil, fmt.Errorf("invalid evaluator request")
	}
	previous := r.Previous
	if previous == "none" {
		previous = ""
	} else if !model.OpaqueID.MatchString(previous) {
		return nil, fmt.Errorf("previous evaluator ID or none required")
	}
	body, _ := json.Marshal(r)
	return s.Store.Transact(ctx, "evaluator:"+r.ID, "cli", body, now, func(st model.State) (store.Change, error) {
		task, _, err := model.ResolveTarget(st, r.Target)
		if err != nil {
			return store.Change{}, err
		}
		if task.Revision != r.ExpectedTaskRevision {
			return store.Change{}, store.ErrConflict
		}
		d := model.Evaluator{Version: 1, ID: r.ID, Target: r.Target, CheckID: r.CheckID, ContractID: r.ContractID, Previous: previous, Spec: *r.Spec, Digest: model.ContentDigest(r.Spec), Actor: "cli", At: now}
		if err := d.Validate(); err != nil {
			return store.Change{}, err
		}
		if !d.Current(st) || st.EvaluatorHeads[model.EvaluatorKey(d.Target, d.CheckID)] != previous {
			return store.Change{}, store.ErrConflict
		}
		resource := st.Resources[d.Spec.ResourceID]
		if (d.Spec.Kind == "test.exit" || d.Spec.Kind == "repo.state") && (resource.Kind != "tree" || resource.Path != ".") {
			return store.Change{}, fmt.Errorf("test/repo evaluator requires a tree rooted at its working directory")
		}
		return store.Change{Revision: st.Revision, Events: []store.Pending{{Subject: "evaluator", Verb: "accepted", EntityID: d.ID, Payload: d}}, Result: d}, nil
	})
}

// Start commits intent before any executable work. The returned launch is true
// only for the caller that committed the new start. Retries never launch again.
func (s Service) Start(ctx context.Context, r Request, now time.Time) (model.Evidence, bool, error) {
	var result model.Evidence
	if r.Version != 1 || !model.OpaqueID.MatchString(r.ID) || !model.OpaqueID.MatchString(r.EvaluatorID) || r.ExpectedTaskRevision < 1 || r.Spec != nil || r.CheckID != "" || r.ContractID != "" || r.Previous != "" {
		return result, false, fmt.Errorf("invalid evaluation request")
	}
	body, _ := json.Marshal(r)
	launch := false
	raw, err := s.Store.Transact(ctx, "evidence:"+r.ID, "cli", body, now, func(st model.State) (store.Change, error) {
		d, ok := st.Evaluators[r.EvaluatorID]
		task, _, err := model.ResolveTarget(st, r.Target)
		if !ok || err != nil || r.Target != d.Target || task.Revision != r.ExpectedTaskRevision || !d.Current(st) || st.EvaluatorHeads[model.EvaluatorKey(d.Target, d.CheckID)] != d.ID {
			return store.Change{}, store.ErrConflict
		}
		running := 0
		for _, v := range st.Evidence {
			if v.Status == "started" {
				running++
			}
		}
		if running >= 4 {
			return store.Change{}, fmt.Errorf("evaluation concurrency limit reached")
		}
		result = model.Evidence{Version: 1, ID: r.ID, EvaluatorID: d.ID, Target: d.Target, TaskRevision: task.Revision, SourceEvent: st.LastEventID, StartedAt: now, Status: "started", Outcome: "unknown", Observer: "daemon", EvaluatorVersion: "1", Inputs: []model.ResourceVersion{}}
		result.Context, err = model.EvidenceContext(st, d.Target)
		result.DecisionDigest = model.EvidenceDecisionDigest(st, d.Target)
		if err != nil {
			return store.Change{}, err
		}
		launch = true
		return store.Change{Revision: st.Revision, Events: []store.Pending{{Subject: "evidence", Verb: "started", EntityID: result.ID, Payload: result}}, Result: result}, nil
	})
	if err != nil {
		return result, false, err
	}
	err = json.Unmarshal(raw, &result)
	return result, launch && err == nil, err
}
func (s Service) Execute(ctx context.Context, id string) error {
	st, err := s.Store.State(ctx)
	if err != nil {
		return err
	}
	e, ok := st.Evidence[id]
	if !ok || e.Status != "started" {
		return fmt.Errorf("evaluation not started")
	}
	d := st.Evaluators[e.EvaluatorID]
	limit := time.Duration(d.Spec.TimeoutSeconds)*time.Second + 10*time.Second
	if d.Spec.Kind != "test.exit" {
		limit = 10 * time.Second
	}
	evalCtx, cancel := context.WithTimeout(ctx, limit)
	defer cancel()
	result := evaluate(evalCtx, st, d, e)
	// Persist a cancelled/unknown result even when the evaluation deadline expires.
	finishCtx, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	return s.finish(finishCtx, result, time.Now().UTC())
}
func (s Service) finish(ctx context.Context, e model.Evidence, now time.Time) error {
	if now.Before(e.StartedAt) {
		now = e.StartedAt
	}
	_, err := s.Store.Transact(ctx, "evidence-finish:"+e.ID, "evaluator", []byte(e.ID), now, func(st model.State) (store.Change, error) {
		old, ok := st.Evidence[e.ID]
		if !ok || old.Status != "started" {
			return store.Change{}, store.ErrConflict
		}
		e.FinishedAt = &now
		e.Status = "finished"
		d := st.Evaluators[e.EvaluatorID]
		if !model.EvidenceCurrent(st, e) || !d.Current(st) || st.EvaluatorHeads[model.EvaluatorKey(d.Target, d.CheckID)] != d.ID {
			e.Outcome = "unknown"
			e.Reason = "definition_or_task_changed"
		}
		return store.Change{Revision: st.Revision, Events: []store.Pending{{Subject: "evidence", Verb: "finished", EntityID: e.ID, Payload: e}}, Result: e}, nil
	})
	return err
}

// Recovery closes abandoned intents without restarting their commands.
func (s Service) Recover(ctx context.Context, now time.Time) error {
	st, err := s.Store.State(ctx)
	if err != nil {
		return err
	}
	ids := []string{}
	for id, e := range st.Evidence {
		if e.Status == "started" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		e := st.Evidence[id]
		e.Reason = "daemon_interrupted_execution_uncertain"
		e.Outcome = "unknown"
		if err := s.finish(ctx, e, now); err != nil {
			return err
		}
	}
	return nil
}

type View struct {
	Definitions   []model.Evaluator            `json:"definitions"`
	Evidence      []model.Evidence             `json:"evidence"`
	Invalidations []model.EvidenceInvalidation `json:"invalidations"`
	Truncated     bool                         `json:"truncated"`
}

func (s Service) View(ctx context.Context, target string) (View, error) {
	st, err := s.Store.State(ctx)
	v := View{Definitions: []model.Evaluator{}, Evidence: []model.Evidence{}, Invalidations: []model.EvidenceInvalidation{}}
	if err != nil {
		return v, err
	}
	if _, _, err = model.ResolveTarget(st, target); err != nil {
		return v, err
	}
	for _, d := range st.Evaluators {
		if d.Target == target {
			v.Definitions = append(v.Definitions, d)
		}
	}
	sort.Slice(v.Definitions, func(i, j int) bool { return v.Definitions[i].At.After(v.Definitions[j].At) })
	for _, e := range st.Evidence {
		if e.Target == target {
			v.Evidence = append(v.Evidence, e)
		}
	}
	sort.Slice(v.Evidence, func(i, j int) bool { return v.Evidence[i].SourceEvent > v.Evidence[j].SourceEvent })
	if len(v.Definitions) > 50 {
		v.Definitions = v.Definitions[:50]
		v.Truncated = true
	}
	if len(v.Evidence) > 50 {
		v.Evidence = v.Evidence[:50]
		v.Truncated = true
	}
	for _, e := range v.Evidence {
		if invalid, ok := st.EvidenceInvalidations[e.ID]; ok {
			v.Invalidations = append(v.Invalidations, invalid)
		}
	}
	raw, _ := json.Marshal(v)
	if len(raw) > 512<<10 {
		return View{}, fmt.Errorf("evidence response exceeds limit")
	}
	return v, nil
}
func (s Service) Refresh(ctx context.Context, target string, now time.Time) error {
	_, err := s.Store.Transact(ctx, "evidence-refresh:"+model.NewID(), "evaluator", []byte(target), now, func(st model.State) (store.Change, error) {
		if _, _, err := model.ResolveTarget(st, target); err != nil {
			return store.Change{}, err
		}
		events := []store.Pending{}
		ids := []string{}
		for id, e := range st.Evidence {
			if e.Target == target && e.Status == "finished" && e.Outcome == "matched" {
				if _, ok := st.EvidenceInvalidations[id]; !ok {
					ids = append(ids, id)
				}
			}
		}
		sort.Strings(ids)
		for _, id := range ids {
			e := st.Evidence[id]
			if err := ValidateEvidence(ctx, st, e); err != nil {
				v := model.EvidenceInvalidation{ID: id, At: now, Reason: err.Error()}
				events = append(events, store.Pending{Subject: "evidence", Verb: "invalidated", EntityID: id, Payload: v})
			}
		}
		return store.Change{Revision: st.Revision, Events: events, Result: map[string]int{"invalidated": len(events)}}, nil
	})
	return err
}
