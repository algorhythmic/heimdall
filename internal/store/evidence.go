package store

import (
	"fmt"
	"heimdall/internal/model"
	"reflect"
	"slices"
)

func applyEvidence(st *model.State, e Event) error {
	switch e.Subject + "." + e.Verb {
	case "evaluator.accepted":
		var d model.Evaluator
		if err := model.StrictJSON(e.Payload, &d); err != nil {
			return err
		}
		if err := d.Validate(); err != nil {
			return err
		}
		if e.Actor != "cli" || d.Actor != e.Actor || !d.At.Equal(e.TS) || d.ID != e.EntityID || !d.Current(*st) {
			return fmt.Errorf("invalid evaluator authority or scope")
		}
		key := model.EvaluatorKey(d.Target, d.CheckID)
		if _, ok := st.Evaluators[d.ID]; ok || st.EvaluatorHeads[key] != d.Previous {
			return fmt.Errorf("invalid evaluator chain")
		}
		st.Evaluators[d.ID] = d
		st.EvaluatorHeads[key] = d.ID
	case "evidence.started":
		var v model.Evidence
		if err := model.StrictJSON(e.Payload, &v); err != nil {
			return err
		}
		d, ok := st.Evaluators[v.EvaluatorID]
		r, _, err := model.ResolveTarget(*st, v.Target)
		if !ok || err != nil || e.Actor != "cli" || v.Version != 1 || !model.OpaqueID.MatchString(v.ID) || v.ID != e.EntityID || v.Target != d.Target || v.TaskRevision != r.Revision || !d.Current(*st) || st.EvaluatorHeads[model.EvaluatorKey(d.Target, d.CheckID)] != d.ID || v.Status != "started" || v.Outcome != "unknown" || v.FinishedAt != nil || v.Observer != "daemon" || v.EvaluatorVersion != "1" || !v.StartedAt.Equal(e.TS) || v.SourceEvent >= e.ID || v.SourceEvent < 0 {
			return fmt.Errorf("invalid evidence start")
		}
		lineage, lineageErr := model.EvidenceContext(*st, v.Target)
		if lineageErr != nil || !reflect.DeepEqual(lineage, v.Context) || model.EvidenceDecisionDigest(*st, v.Target) != v.DecisionDigest {
			return fmt.Errorf("invalid evidence context boundary")
		}
		if len(v.Inputs) != 0 || v.OutputDigest != "" || v.ExecutableDigest != "" || v.EnvironmentDigest != "" || v.ExitCode != nil || v.Repo != nil || v.Reason != "" || v.OutputBytes != 0 {
			return fmt.Errorf("start cannot assert results")
		}
		if _, exists := st.Evidence[v.ID]; exists {
			return fmt.Errorf("duplicate evidence")
		}
		st.Evidence[v.ID] = v
	case "evidence.finished":
		var v model.Evidence
		if err := model.StrictJSON(e.Payload, &v); err != nil {
			return err
		}
		old, ok := st.Evidence[v.ID]
		if !ok || e.Actor != "evaluator" || v.ID != e.EntityID || old.Status != "started" || v.Status != "finished" || v.FinishedAt == nil || !v.FinishedAt.Equal(e.TS) || v.FinishedAt.Before(v.StartedAt) || !model.Contains([]string{"matched", "not_matched", "unknown"}, v.Outcome) {
			return fmt.Errorf("invalid evidence finish")
		}
		immutable := v
		immutable.Status = old.Status
		immutable.Outcome = old.Outcome
		immutable.FinishedAt = nil
		immutable.Inputs = old.Inputs
		immutable.OutputDigest = ""
		immutable.ExecutableDigest = ""
		immutable.EnvironmentDigest = ""
		immutable.OutputBytes = 0
		immutable.ExitCode = nil
		immutable.Repo = nil
		immutable.Reason = ""
		if !reflect.DeepEqual(immutable, old) {
			return fmt.Errorf("evidence finish changed input identity")
		}
		if v.Outcome == "matched" {
			d := st.Evaluators[v.EvaluatorID]
			if !model.EvidenceCurrent(*st, v) || !d.Current(*st) {
				return fmt.Errorf("matched evidence has stale definition")
			}
			ids := []string{}
			for _, ref := range v.Inputs {
				if len(ref.Snapshot.Digest) != 64 {
					return fmt.Errorf("invalid evidence snapshot")
				}
				ids = append(ids, ref.ID)
			}
			if !slices.Equal(ids, st.Contracts[d.ContractID].ResourceIDs) {
				return fmt.Errorf("matched evidence lacks complete coverage")
			}
			if d.Spec.Kind == "test.exit" && (v.ExitCode == nil || *v.ExitCode != 0 || v.OutputDigest == "" || len(v.ExecutableDigest) != 64 || len(v.EnvironmentDigest) != 64) {
				return fmt.Errorf("matched test lacks observed execution")
			}
			if d.Spec.Kind == "repo.state" && v.Repo == nil {
				return fmt.Errorf("matched repo evidence lacks identity")
			}
		}
		st.Evidence[v.ID] = v
	case "evidence.invalidated":
		var v model.EvidenceInvalidation
		if err := model.StrictJSON(e.Payload, &v); err != nil {
			return err
		}
		if e.Actor != "evaluator" || v.ID != e.EntityID || v.Reason == "" || !v.At.Equal(e.TS) {
			return fmt.Errorf("invalid evidence invalidation")
		}
		if _, ok := st.Evidence[v.ID]; !ok {
			return fmt.Errorf("unknown evidence")
		}
		if _, ok := st.EvidenceInvalidations[v.ID]; ok {
			return fmt.Errorf("duplicate invalidation")
		}
		st.EvidenceInvalidations[v.ID] = v
	}
	return nil
}
