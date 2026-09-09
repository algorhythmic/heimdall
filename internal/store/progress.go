package store

import (
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
)

func applyProgress(st *model.State, e Event) error {
	switch e.Verb {
	case "proposed":
		if e.Actor != "cli" {
			return fmt.Errorf("progress proposals require CLI authority")
		}
		var p model.ProgressProposal
		if err := model.StrictJSON(e.Payload, &p); err != nil {
			return err
		}
		if err := p.Validate(); err != nil {
			return err
		}
		if p.ID != e.EntityID || p.Actor != e.Actor || !p.At.Equal(e.TS) {
			return fmt.Errorf("progress proposal envelope mismatch")
		}
		if _, ok := st.ProgressProposals[p.ID]; ok {
			return fmt.Errorf("duplicate progress proposal")
		}
		if err := model.ProgressCurrent(*st, p); err != nil {
			return err
		}
		// Bound unresolved proposals per target; accepted/rejected history remains.
		count := 0
		for _, old := range st.ProgressProposals {
			status := model.ProgressStatus(*st, old)
			if old.Target == p.Target && (status == "draft" || status == "reviewed") && old.ID != p.Previous {
				count++
			}
		}
		if count >= 128 {
			return fmt.Errorf("unresolved progress proposal limit (128) exceeded")
		}
		if p.Kind == "artifact" {
			if st.ArtifactProgressHeads[p.Artifacts[0].ArtifactID] != p.Previous {
				return fmt.Errorf("artifact progress head changed")
			}
			st.ArtifactProgressHeads[p.Artifacts[0].ArtifactID] = p.ID
		}
		st.ProgressProposals[p.ID] = p
	case "reviewed":
		var r model.ProgressReview
		if err := model.StrictJSON(e.Payload, &r); err != nil {
			return err
		}
		if r.Version == 1 {
			var fields map[string]json.RawMessage
			_ = json.Unmarshal(e.Payload, &fields)
			if _, ok := fields["authority"]; ok {
				return fmt.Errorf("legacy review cannot carry authority")
			}
		}
		if err := r.Validate(); err != nil {
			return err
		}
		if r.ID != e.EntityID || r.Actor != e.Actor || !r.At.Equal(e.TS) {
			return fmt.Errorf("progress review envelope mismatch")
		}
		if _, ok := st.ProgressReviews[r.ID]; ok {
			return fmt.Errorf("duplicate progress review")
		}
		p, ok := st.ProgressProposals[r.ProposalID]
		if !ok || p.Target != r.Target || p.Digest != r.Digest || st.ProgressReviewHeads[p.ID] != r.Previous {
			return fmt.Errorf("review proposal, digest or head mismatch")
		}
		if r.Version == 2 && !r.Authority.Permits(*st, p) {
			return fmt.Errorf("review outside recorded UI authority")
		}
		task, _, err := model.ResolveTarget(*st, r.Target)
		if err != nil || task.Revision != r.TaskRevision {
			return fmt.Errorf("review task revision mismatch")
		}
		prior := st.ProgressReviews[r.Previous]
		if prior.Status == "accepted" || prior.Status == "rejected" || (prior.Status == "reviewed" && r.Status == "reviewed") {
			return fmt.Errorf("progress review already resolved")
		}
		if r.Status != "rejected" {
			if model.ProgressStatus(*st, p) == "superseded" {
				return fmt.Errorf("proposal superseded")
			}
			if err := model.ProgressCurrent(*st, p); err != nil {
				return err
			}
		}
		if r.Status == "accepted" && p.Kind == "decision" {
			if _, ok := st.Decisions[r.ID]; ok {
				return fmt.Errorf("duplicate accepted decision")
			}
			st.Decisions[r.ID] = model.Decision{Version: r.Version, ID: r.ID, Target: p.Target, TaskRevision: r.TaskRevision, Text: p.Text, Supersedes: p.Supersedes, Actor: r.Actor, At: r.At}
		}
		st.ProgressReviews[r.ID] = r
		st.ProgressReviewHeads[p.ID] = r.ID
	default:
		return fmt.Errorf("unknown progress event")
	}
	return nil
}
