package continuity

import (
	"context"
	"encoding/json"
	"heimdall/internal/model"
	"reflect"
	"strings"
	"time"
)

// RecordedNextAction selects checkpoint direction only while its recorded
// lineage and accepted decisions still match. It does not observe resource bytes.
func RecordedNextAction(st model.State, target string) (string, string) {
	r, _, err := model.ResolveTarget(st, target)
	if err != nil {
		return "", "task_missing"
	}
	cp, ok := st.Checkpoints[st.CheckpointHeads[target]]
	if !ok {
		return r.Task.NextAction, "no_checkpoint"
	}
	for _, ref := range cp.Artifacts {
		if st.ArtifactHeads[ref.ArtifactID] != ref.VersionID {
			return r.Task.NextAction, "checkpoint_needs_review"
		}
	}
	targets, err := lineage(st, target)
	if err != nil || !reflect.DeepEqual(cp.Context, versions(st, targets)) || !reflect.DeepEqual(cp.Decisions, decisionIDs(st, targets)) {
		return r.Task.NextAction, "checkpoint_needs_review"
	}
	if len(cp.Blockers) > 0 {
		return cp.NextAction, "checkpoint_blocked"
	}
	return cp.NextAction, "checkpoint_recorded"
}

// ResumeView adds bounded review counts to core context without changing the
// existing context wire format or treating saved evidence as a fresh evaluation.
type ResumeView struct {
	Bundle
	Reviews ReviewSummary `json:"reviews"`
}

type ReviewSummary struct {
	PendingProposals int `json:"pending_proposals"`
	FailedEvidence   int `json:"failed_evidence"`
	StaleEvidence    int `json:"stale_evidence"`
	UnknownEvidence  int `json:"unknown_evidence"`
}

func (s Service) Resume(ctx context.Context, target string, budget int) (ResumeView, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	st, err := s.Store.State(ctx)
	if err != nil {
		return ResumeView{}, err
	}
	bundle, err := buildContext(ctx, st, target, budget)
	if err != nil {
		return ResumeView{}, err
	}
	if err := bundle.ObserveOwnedWindows(ctx, s.Desktop, budget, st); err != nil {
		return ResumeView{}, err
	}
	result := ResumeView{Bundle: bundle, Reviews: reviewSummary(st, target)}
	// Account for the entire response, including its review summary. As with
	// context, mandatory direction and drift warnings are never truncated.
	for i := 0; i < 8; i++ {
		raw, err := json.Marshal(result)
		if err != nil {
			return ResumeView{}, err
		}
		n := (len(raw) + 3) / 4
		if n == result.EstimatedTokens {
			break
		}
		result.EstimatedTokens = n
	}
	if result.EstimatedTokens > budget {
		return ResumeView{}, &BudgetError{Required: result.EstimatedTokens, Budget: budget}
	}
	return result, nil
}

func reviewSummary(st model.State, target string) ReviewSummary {
	return RecordedReviews(st, target)
}

// RecordedReviews summarizes existing records without executing evaluators or
// inspecting resources. Display integrations must not call this live evidence.
func RecordedReviews(st model.State, target string) ReviewSummary {
	includes := func(candidate string) bool {
		return candidate == target || (!strings.Contains(target, "#") && strings.HasPrefix(candidate, target+"#"))
	}
	result := ReviewSummary{}
	for _, p := range st.Proposals {
		if includes(p.Target) && p.Status == "pending" {
			result.PendingProposals++
		}
	}
	// Count only the latest attempt of each current evaluator, so old failures
	// do not remain review needs after a replacement definition or successful run.
	latest := map[string]model.Evidence{}
	for _, e := range st.Evidence {
		d, ok := st.Evaluators[e.EvaluatorID]
		if !ok || !includes(e.Target) || st.EvaluatorHeads[model.EvaluatorKey(d.Target, d.CheckID)] != d.ID {
			continue
		}
		if old, ok := latest[d.ID]; !ok || e.SourceEvent > old.SourceEvent {
			latest[d.ID] = e
		}
	}
	for _, e := range latest {
		if e.Status != "finished" {
			result.UnknownEvidence++
		} else if !model.EvidenceCurrent(st, e) {
			result.StaleEvidence++
		} else if e.Outcome == "unknown" {
			result.UnknownEvidence++
		} else if e.Outcome == "not_matched" {
			result.FailedEvidence++
		}
	}
	return result
}
