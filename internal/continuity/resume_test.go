package continuity

import (
	"encoding/json"
	"errors"
	"heimdall/internal/model"
	"reflect"
	"testing"
)

func TestResumePreservesContextAndDoesNotWrite(t *testing.T) {
	f := setup(t)
	contract := f.contract()
	f.send(f.checkpoint(contract, "none"))
	before, _ := f.e.Store.State(f.ctx)
	v, err := f.s.Resume(f.ctx, f.target, 16000)
	if err != nil || v.Checkpoint == nil || v.ResumeStatus != "ready" {
		t.Fatal(v, err)
	}
	after, _ := f.e.Store.State(f.ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("resume mutated state")
	}
	context, err := f.s.Context(f.ctx, f.target, 16000)
	if err != nil {
		t.Fatal(err)
	}
	if context.EstimatedTokens >= v.EstimatedTokens {
		t.Fatal("review fields were not budgeted")
	}
	v.Bundle.EstimatedTokens = context.EstimatedTokens
	if !reflect.DeepEqual(v.Bundle, context) {
		t.Fatal("resume changed mandatory context")
	}
	raw, _ := json.Marshal(context)
	var fields map[string]any
	json.Unmarshal(raw, &fields)
	if _, ok := fields["reviews"]; ok {
		t.Fatal("legacy context format changed")
	}
	full, _ := f.s.Resume(f.ctx, f.target, 16000)
	_, err = f.s.Resume(f.ctx, f.target, full.EstimatedTokens-1)
	var budget *BudgetError
	if !errors.As(err, &budget) || budget.Required != full.EstimatedTokens {
		t.Fatal("full response budget not enforced", err)
	}
}

func TestRecordedNextActionRejectsStaleCheckpointDirection(t *testing.T) {
	f := setup(t)
	contract := f.contract()
	st, _ := f.e.Store.State(f.ctx)
	if _, status := RecordedNextAction(st, f.target); status != "no_checkpoint" {
		t.Fatal(status)
	}
	cpRequest := f.checkpoint(contract, "none")
	f.send(cpRequest)
	for _, change := range []string{"none", "blocker", "revision", "contract", "decision"} {
		t.Run(change, func(t *testing.T) {
			st, _ := f.e.Store.State(f.ctx)
			task := st.Tasks[f.target]
			task.Task.NextAction = "Review current task direction"
			st.Tasks[f.target] = task
			wantAction, wantStatus := cpRequest.Checkpoint.NextAction, "checkpoint_recorded"
			switch change {
			case "blocker":
				cp := st.Checkpoints[cpRequest.ID]
				cp.Blockers = []string{"Pending review"}
				st.Checkpoints[cp.ID] = cp
				wantStatus = "checkpoint_blocked"
			case "revision":
				task.Revision++
				st.Tasks[f.target] = task
			case "contract":
				st.ContractHeads[f.target] = "replacement-contract"
			case "decision":
				st.Decisions["new-decision"] = model.Decision{ID: "new-decision", Target: f.target}
			}
			if change == "revision" || change == "contract" || change == "decision" {
				wantAction, wantStatus = task.Task.NextAction, "checkpoint_needs_review"
			}
			action, status := RecordedNextAction(st, f.target)
			if action != wantAction || status != wantStatus {
				t.Fatalf("got %q/%s, want %q/%s", action, status, wantAction, wantStatus)
			}
		})
	}
}

func TestReviewCountsStayOnTargetAndUseLatestAttempt(t *testing.T) {
	f := setup(t)
	contract := f.contract()
	st, _ := f.e.Store.State(f.ctx)
	done := model.Done{Checks: []model.Check{{ID: "artifact", Kind: "artifact.exists"}}}
	task := st.Tasks[f.target]
	task.Task.Done = done
	st.Tasks[f.target] = task
	c := st.Contracts[contract]
	c.Acceptance = done
	c.ResourceIDs = []string{"resource"}
	st.Contracts[contract] = c
	st.Resources["resource"] = model.Resource{ID: "resource", Target: f.target, Active: true}
	d := model.Evaluator{ID: "definition", Target: f.target, CheckID: "artifact", ContractID: contract, Spec: model.EvaluatorSpec{Kind: "artifact.exists", ResourceID: "resource"}}
	st.Evaluators[d.ID] = d
	st.EvaluatorHeads[model.EvaluatorKey(f.target, "artifact")] = d.ID
	versions, _ := model.EvidenceContext(st, f.target)
	e := model.Evidence{ID: "latest", EvaluatorID: d.ID, Target: f.target, TaskRevision: f.rev, Context: versions, DecisionDigest: model.EvidenceDecisionDigest(st, f.target), SourceEvent: 2, Status: "finished", Outcome: "matched"}
	st.Evidence[e.ID] = e
	old := e
	old.ID, old.SourceEvent, old.Outcome = "old", 1, "unknown"
	st.Evidence[old.ID] = old
	st.Proposals["own"] = model.Proposal{Target: f.target, Status: "pending"}
	st.Proposals["step"] = model.Proposal{Target: f.target + "#review", Status: "pending"}
	st.Proposals["other"] = model.Proposal{Target: "another-task", Status: "pending"}
	st.Proposals["accepted"] = model.Proposal{Target: f.target, Status: "accepted"}
	if got := reviewSummary(st, f.target); got != (ReviewSummary{PendingProposals: 2}) {
		t.Fatal("old failures or unrelated proposals leaked", got)
	}
	if got := reviewSummary(st, f.target+"#review"); got.PendingProposals != 1 {
		t.Fatal("step includes parent or sibling", got)
	}
	e.Status = "started"
	st.Evidence[e.ID] = e
	if got := reviewSummary(st, f.target); got.UnknownEvidence != 1 || got.StaleEvidence != 0 {
		t.Fatal("unfinished execution should be unknown", got)
	}
	e.Status, e.Outcome = "finished", "unknown"
	st.Evidence[e.ID] = e
	if got := reviewSummary(st, f.target); got.UnknownEvidence != 1 {
		t.Fatal("unknown outcome hidden", got)
	}
	e.Outcome = "not_matched"
	st.Evidence[e.ID] = e
	if got := reviewSummary(st, f.target); got.FailedEvidence != 1 {
		t.Fatal("failed evidence hidden", got)
	}
	st.EvidenceInvalidations[e.ID] = model.EvidenceInvalidation{ID: e.ID, Reason: "changed input"}
	if got := reviewSummary(st, f.target); got.StaleEvidence != 1 || got.UnknownEvidence != 0 {
		t.Fatal("invalidated evidence not shown", got)
	}
}
