package continuity

import (
	"encoding/json"
	"errors"
	"heimdall/internal/core"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func proposeProgress(t *testing.T, f *fixture, input ProgressInput) model.ProgressProposal {
	t.Helper()
	r := ProgressRequest{Version: 1, ID: model.NewID(), Target: f.target, ExpectedTaskRevision: f.rev, Proposal: &input}
	raw, err := f.s.Progress(f.ctx, r, "cli", f.now)
	if err != nil {
		t.Fatal(err)
	}
	var p model.ProgressProposal
	if err := json.Unmarshal(raw, &p); err != nil {
		t.Fatal(err)
	}
	return p
}
func progressReview(f *fixture, p model.ProgressProposal, status, previous string) ProgressRequest {
	return ProgressRequest{Version: 1, ID: model.NewID(), Target: f.target, ExpectedTaskRevision: f.rev, Review: &ProgressReviewInput{ProposalID: p.ID, Digest: p.Digest, Previous: previous, Status: status, Note: "Explicit review of this exact proposal"}}
}
func sendProgress(t *testing.T, f *fixture, r ProgressRequest) json.RawMessage {
	t.Helper()
	b, err := f.s.Progress(f.ctx, r, "cli", f.now)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestProgressDecisionReviewContextSupersessionAndManualCompletion(t *testing.T) {
	f := setup(t)
	contract := f.contract()
	f.send(f.checkpoint(contract, "none"))
	p := proposeProgress(t, f, ProgressInput{Kind: "decision", Text: "Use explicit task ownership", ContractID: contract})
	b, err := f.s.Context(f.ctx, f.target, 16000)
	if err != nil || len(b.Decisions) != 0 || len(b.Progress) != 1 || b.Progress[0].Status != "draft" || !hasIssue(b, "unresolved_progress") {
		t.Fatal(b, err)
	}
	reviewed := progressReview(f, p, "reviewed", "none")
	sendProgress(t, f, reviewed)
	st, _ := f.e.Store.State(f.ctx)
	if len(st.Decisions) != 0 || st.Tasks[f.target].Task.Status != "active" || len(st.Proposals) != 0 {
		t.Fatal("review alone accepted direction or completed work")
	}
	accept := progressReview(f, p, "accepted", reviewed.ID)
	first := sendProgress(t, f, accept)
	b, err = f.s.Context(f.ctx, f.target, 16000)
	if err != nil || len(b.Decisions) != 1 || b.Decisions[0].Text != p.Text || b.Progress[0].Freshness != "recorded_current" || hasIssue(b, "unresolved_progress") || !hasIssue(b, "decisions_changed") {
		t.Fatal(b, err)
	}
	if _, err = f.s.Context(f.ctx, f.target, b.EstimatedTokens-1); err == nil {
		t.Fatal("mandatory decision/progress dropped to fit budget")
	}
	if retry := sendProgress(t, f, accept); string(retry) != string(first) {
		t.Fatal("review receipt changed")
	}
	if _, err = f.s.Progress(f.ctx, progressReview(f, p, "rejected", accept.ID), "cli", f.now); !errors.Is(err, store.ErrConflict) {
		t.Fatal("terminal review overwritten", err)
	}
	newP := proposeProgress(t, f, ProgressInput{Kind: "decision", Text: "Use task and environment ownership", ContractID: contract, Supersedes: accept.ID})
	newAccept := progressReview(f, newP, "accepted", "none")
	sendProgress(t, f, newAccept)
	b, err = f.s.Context(f.ctx, f.target, 16000)
	if err != nil || len(b.Decisions) != 1 || b.Decisions[0].ID != newAccept.ID {
		t.Fatal("supersession missing", b, err)
	}
	v, err := f.s.ProgressShow(f.ctx, f.target, p.ID)
	if err != nil || v.Status != "superseded" {
		t.Fatal(v, err)
	}
	st, _ = f.e.Store.State(f.ctx)
	replay, err := f.e.Replay(f.ctx)
	if err != nil || !reflect.DeepEqual(st, replay) {
		t.Fatal("progress replay differs", err)
	}
	// Existing CLI manual attestation remains authoritative; review did not
	// modify its acceptance definition, task revision, evidence or proposals.
	if st.Tasks[f.target].Revision != f.rev || st.Tasks[f.target].Task.Status != "active" || len(st.Evidence) != 0 || len(st.Proposals) != 0 {
		t.Fatal("progress changed completion authority")
	}
	if _, err := f.e.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "complete", Target: f.target}, "cli", f.now); err != nil {
		t.Fatal("manual completion unavailable", err)
	}
}

func TestProgressStaleDecisionRefusalAndRejection(t *testing.T) {
	for _, change := range []string{"contract", "decision", "revision", "scope", "digest", "target", "previous"} {
		t.Run(change, func(t *testing.T) {
			f := setup(t)
			contract := f.contract()
			p := proposeProgress(t, f, ProgressInput{Kind: "decision", Text: "Reviewed design choice", ContractID: contract})
			r := progressReview(f, p, "accepted", "none")
			switch change {
			case "contract":
				f.contract()
			case "decision":
				d := f.request("decision.accept")
				d.Decision = &DecisionInput{Text: "Concurrent accepted decision"}
				f.send(d)
			case "revision":
				st, _ := f.e.Store.State(f.ctx)
				task := st.Tasks[f.target].Task
				task.Title = "Changed task"
				if _, err := f.e.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "update", Task: &task}, "cli", f.now); err != nil {
					t.Fatal(err)
				}
				st, _ = f.e.Store.State(f.ctx)
				f.rev = st.Tasks[f.target].Revision
				r.ExpectedTaskRevision = f.rev
			case "scope":
				d := f.request("resource.bind")
				d.Resource = &ResourceInput{Kind: "tree", Root: t.TempDir(), Path: "."}
				f.send(d)
			case "digest":
				r.Review.Digest = strings.Repeat("a", 64)
			case "target":
				r.Target = "other-task"
			case "previous":
				r.Review.Previous = model.NewID()
			}
			before, _ := f.e.Store.State(f.ctx)
			if _, err := f.s.Progress(f.ctx, r, "cli", f.now); err == nil {
				t.Fatal("stale/foreign acceptance succeeded")
			}
			after, _ := f.e.Store.State(f.ctx)
			if !reflect.DeepEqual(before, after) {
				t.Fatal("refused review mutated state")
			}
			sendProgress(t, f, progressReview(f, p, "rejected", "none"))
			view, err := f.s.ProgressShow(f.ctx, f.target, p.ID)
			if err != nil || view.Status != "rejected" {
				t.Fatal(view, err)
			}
		})
	}
}

func TestProgressReviewConcurrencyAuthorityAndReadScope(t *testing.T) {
	f := setup(t)
	p := proposeProgress(t, f, ProgressInput{Kind: "decision", Text: "Keep proposals distinct", ContractID: f.contract()})
	a, b := progressReview(f, p, "accepted", "none"), progressReview(f, p, "rejected", "none")
	for _, actor := range []string{"", "browser", "client:" + model.NewID()} {
		if _, err := f.s.Progress(f.ctx, a, actor, f.now); err == nil {
			t.Fatal("non-CLI reviewed progress")
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, r := range []ProgressRequest{a, b} {
		wg.Add(1)
		go func(r ProgressRequest) { defer wg.Done(); _, err := f.s.Progress(f.ctx, r, "cli", f.now); errs <- err }(r)
	}
	wg.Wait()
	close(errs)
	wins, conflicts := 0, 0
	for err := range errs {
		if err == nil {
			wins++
		} else if errors.Is(err, store.ErrConflict) {
			conflicts++
		} else {
			t.Fatal(err)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatal(wins, conflicts)
	}
	st, _ := f.e.Store.State(f.ctx)
	g := model.Grant{ID: model.NewID(), Target: f.target, ResourceIDs: []string{}}
	bundle, err := ScopedContext(f.ctx, st, g, f.target, 16000)
	if err != nil {
		t.Fatal(err)
	}
	if st.ProgressReviews[st.ProgressReviewHeads[p.ID]].Status == "accepted" && len(bundle.Decisions) != 1 {
		t.Fatal("accepted decision absent from scoped mandatory context")
	}
	g.Target = "other-task"
	if _, err = ScopedContext(f.ctx, st, g, f.target, 16000); err == nil {
		t.Fatal("cross-scope progress leaked")
	}
	if _, err = f.s.ProgressShow(f.ctx, "other-task", p.ID); err == nil {
		t.Fatal("cross-target show")
	}
	items, err := f.s.ProgressList(f.ctx, f.target, "", 1)
	if err != nil || len(items) != 1 {
		t.Fatal(items, err)
	}
	items, err = f.s.ProgressList(f.ctx, f.target, items[0].Proposal.ID, 1)
	if err != nil || len(items) != 0 {
		t.Fatal(items, err)
	}
	after, _ := f.e.Store.State(f.ctx)
	if !reflect.DeepEqual(st, after) {
		t.Fatal("progress reads mutated state")
	}
}

func TestProgressInheritedDirectionAndScope(t *testing.T) {
	f := setup(t)
	parent, parentContract := f.target, f.contract()
	child := model.Task{ID: "child-task", Parent: parent, Title: "Child planning", Type: "project", Status: "active"}
	if _, err := f.e.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "add", Task: &child}, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	st, _ := f.e.Store.State(f.ctx)
	f.target, f.rev = child.ID, st.Tasks[child.ID].Revision
	p := proposeProgress(t, f, ProgressInput{Kind: "decision", Text: "Use the inherited direction", ContractID: f.contract()})
	st, _ = f.e.Store.State(f.ctx)
	if _, err := ScopedContext(f.ctx, st, model.Grant{Target: child.ID}, child.ID, 16000); err == nil {
		t.Fatal("missing inherited permission ignored")
	}
	b, err := ScopedContext(f.ctx, st, model.Grant{Target: parent, Subtree: true}, child.ID, 16000)
	if err != nil || len(b.Progress) != 1 {
		t.Fatal(b, err)
	}
	rev := st.Tasks[parent].Revision
	f.send(Request{Version: 1, ID: model.NewID(), Target: parent, ExpectedTaskRevision: &rev, Op: "contract.accept", Contract: &ContractInput{Previous: parentContract, Objective: "Changed parent direction"}})
	if _, err := f.s.Progress(f.ctx, progressReview(f, p, "accepted", "none"), "cli", f.now); !errors.Is(err, store.ErrConflict) {
		t.Fatal("changed ancestor accepted", err)
	}
}
