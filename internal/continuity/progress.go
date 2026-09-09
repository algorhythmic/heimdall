package continuity

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"sort"
	"time"
)

type ProgressInput struct {
	Kind       string              `json:"kind"`
	Text       string              `json:"text"`
	ContractID string              `json:"contract_id"`
	Artifacts  []model.ArtifactRef `json:"artifacts,omitempty"`
	Supersedes string              `json:"supersedes,omitempty"`
	Previous   string              `json:"previous,omitempty"`
}
type ProgressReviewInput struct {
	ProposalID string `json:"proposal_id"`
	Digest     string `json:"digest"`
	Previous   string `json:"previous"`
	Status     string `json:"status"`
	Note       string `json:"note"`
}
type ProgressRequest struct {
	Version              int                  `json:"version"`
	ID                   string               `json:"id"`
	Target               string               `json:"target"`
	ExpectedTaskRevision int64                `json:"expected_task_revision"`
	Proposal             *ProgressInput       `json:"proposal,omitempty"`
	Review               *ProgressReviewInput `json:"review,omitempty"`
}

func (r ProgressRequest) Validate() error {
	if r.Version != 1 || !model.OpaqueID.MatchString(r.ID) || r.ExpectedTaskRevision < 1 || (r.Proposal == nil) == (r.Review == nil) {
		return fmt.Errorf("progress requires v1, ID, task revision and exactly one payload")
	}
	if p := r.Proposal; p != nil {
		if !textValid(p.Text, 8192) || !model.OpaqueID.MatchString(p.ContractID) || (p.Supersedes != "" && !model.OpaqueID.MatchString(p.Supersedes)) {
			return fmt.Errorf("invalid progress proposal")
		}
		switch p.Kind {
		case "artifact":
			if len(p.Artifacts) != 1 || p.Supersedes != "" {
				return fmt.Errorf("artifact proposal requires exactly one version")
			}
			if _, err := head(p.Previous); err != nil {
				return err
			}
		case "decision":
			if p.Previous != "" {
				return fmt.Errorf("decision proposal cannot replace artifact progress")
			}
		default:
			return fmt.Errorf("progress kind must be decision or artifact")
		}
		if len(p.Artifacts) > 0 {
			return model.ValidArtifactRefs(p.Artifacts)
		}
		return nil
	}
	i := r.Review
	previous, err := head(i.Previous)
	if err != nil {
		return err
	}
	return (model.ProgressReview{Version: 1, ID: r.ID, Target: r.Target, TaskRevision: r.ExpectedTaskRevision, ProposalID: i.ProposalID, Digest: i.Digest, Previous: previous, Status: i.Status, Note: i.Note, Actor: "cli", At: time.Unix(1, 0)}).Validate()
}
func DecodeProgress(raw []byte) (ProgressRequest, error) {
	var r ProgressRequest
	if len(raw) > MaxRequest {
		return r, fmt.Errorf("progress request exceeds 64 KiB")
	}
	if err := model.StrictJSON(raw, &r); err != nil {
		return r, err
	}
	return r, r.Validate()
}

func (s Service) Progress(ctx context.Context, r ProgressRequest, actor string, now time.Time) (json.RawMessage, error) {
	if actor != "cli" {
		return nil, fmt.Errorf("progress requires CLI authority")
	}
	return s.progress(ctx, r, actor, now)
}

func (s Service) progress(ctx context.Context, r ProgressRequest, actor string, now time.Time) (json.RawMessage, error) {
	if err := r.Validate(); err != nil {
		return nil, err
	}
	raw, _ := json.Marshal(r)
	if len(raw) > MaxRequest {
		return nil, fmt.Errorf("progress request exceeds 64 KiB")
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	commandID := "progress-" + r.ID
	return s.Store.Transact(ctx, commandID, actor, raw, now, func(st model.State) (store.Change, error) {
		c := store.Change{Revision: st.Revision}
		task, _, err := model.ResolveTarget(st, r.Target)
		if err != nil {
			return c, err
		}
		if task.Revision != r.ExpectedTaskRevision {
			return c, fmt.Errorf("progress task revision: %w", store.ErrConflict)
		}
		var record any
		verb := "proposed"
		if in := r.Proposal; in != nil {
			context, err := model.EvidenceContext(st, r.Target)
			if err != nil {
				return c, err
			}
			previous := ""
			if in.Kind == "artifact" {
				previous, _ = head(in.Previous)
			}
			p := model.ProgressProposal{Version: 1, ID: r.ID, Target: r.Target, TaskRevision: task.Revision, Kind: in.Kind, Text: in.Text, ContractID: in.ContractID, Context: context, DecisionDigest: model.EvidenceDecisionDigest(st, r.Target), Artifacts: []model.ProgressArtifact{}, Supersedes: in.Supersedes, Previous: previous, Actor: actor, At: now.UTC()}
			for _, ref := range in.Artifacts {
				v, err := artifactSelection(st, r.Target, ref.ArtifactID, ref.VersionID)
				if err != nil {
					return c, err
				}
				p.Artifacts = append(p.Artifacts, model.ProgressArtifact{ArtifactID: ref.ArtifactID, VersionID: ref.VersionID, Observation: v.Record.Observation})
			}
			p.Digest = p.ContentDigest()
			if err := model.ProgressCurrent(st, p); err != nil {
				return c, fmt.Errorf("%s: %w", err, store.ErrConflict)
			}
			if p.Kind == "artifact" && st.ArtifactProgressHeads[p.Artifacts[0].ArtifactID] != previous {
				return c, fmt.Errorf("artifact progress head: %w", store.ErrConflict)
			}
			if err := validateLiveArtifactRefs(ctx, st, r.Target, in.Artifacts, st.Contracts[in.ContractID].ResourceIDs); err != nil {
				return c, err
			}
			record = p
		} else {
			verb = "reviewed"
			in := r.Review
			previous, _ := head(in.Previous)
			p, ok := st.ProgressProposals[in.ProposalID]
			if !ok || p.Target != r.Target {
				return c, fmt.Errorf("proposal not found for target")
			}
			if p.Digest != in.Digest || st.ProgressReviewHeads[p.ID] != previous {
				return c, fmt.Errorf("review digest or head: %w", store.ErrConflict)
			}
			prior := st.ProgressReviews[previous]
			if prior.Status == "accepted" || prior.Status == "rejected" || (prior.Status == "reviewed" && in.Status == "reviewed") {
				return c, fmt.Errorf("proposal already resolved: %w", store.ErrConflict)
			}
			if in.Status != "rejected" {
				if model.ProgressStatus(st, p) == "superseded" {
					return c, fmt.Errorf("proposal superseded: %w", store.ErrConflict)
				}
				if err := model.ProgressCurrent(st, p); err != nil {
					return c, fmt.Errorf("%s: %w", err, store.ErrConflict)
				}
				if err := validateLiveArtifactRefs(ctx, st, r.Target, p.ArtifactRefs(), st.Contracts[p.ContractID].ResourceIDs); err != nil {
					return c, err
				}
			}
			record = model.ProgressReview{Version: 1, ID: r.ID, Target: r.Target, TaskRevision: task.Revision, ProposalID: p.ID, Digest: p.Digest, Previous: previous, Status: in.Status, Note: in.Note, Actor: actor, At: now.UTC()}
		}
		encoded, _ := json.Marshal(record)
		if len(encoded) > MaxRequest {
			return c, fmt.Errorf("progress record exceeds 64 KiB")
		}
		c.Events = []store.Pending{{Subject: "progress", Verb: verb, EntityID: r.ID, Payload: record}}
		c.Result = record
		return c, nil
	})
}

type ProgressView struct {
	Proposal   model.ProgressProposal `json:"proposal"`
	ReviewHead string                 `json:"review_head"`
	Review     *model.ProgressReview  `json:"review,omitempty"`
	Status     string                 `json:"status"`
	Freshness  string                 `json:"freshness"`
	Artifacts  []ArtifactCheck        `json:"artifacts,omitempty"`
}

// Context exposes proposal text and opaque identity only. Existing scoped read
// grants gain no artifact metadata or filesystem/Git observation authority.
type ProgressSummary struct {
	ID        string `json:"id"`
	Target    string `json:"target"`
	Kind      string `json:"kind"`
	Text      string `json:"text"`
	Status    string `json:"status"`
	Freshness string `json:"freshness"`
}

func progressContext(st model.State, targets []string) []ProgressSummary {
	out := []ProgressSummary{}
	for _, p := range st.ProgressProposals {
		if !model.Contains(targets, p.Target) {
			continue
		}
		v := progressView(context.Background(), st, p, false)
		if v.Status == "rejected" || v.Status == "superseded" {
			continue
		}
		// Accepted decisions are already mandatory, but keep their review
		// provenance/freshness visible alongside unresolved proposals.
		out = append(out, ProgressSummary{p.ID, p.Target, p.Kind, p.Text, v.Status, v.Freshness})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func progressView(ctx context.Context, st model.State, p model.ProgressProposal, live bool) ProgressView {
	v := ProgressView{Proposal: p, ReviewHead: st.ProgressReviewHeads[p.ID], Status: model.ProgressStatus(st, p), Freshness: "recorded_current"}
	if r, ok := st.ProgressReviews[v.ReviewHead]; ok {
		v.Review = &r
		// An accepted decision itself is the expected result, not input drift.
		if r.Status == "accepted" && p.Kind == "decision" {
			decisions := make(map[string]model.Decision, len(st.Decisions))
			for id, d := range st.Decisions {
				if id != r.ID {
					decisions[id] = d
				}
			}
			st.Decisions = decisions
		}
	}
	if model.ProgressCurrent(st, p) != nil || v.Status == "superseded" {
		v.Freshness = "stale"
	}
	if len(p.Artifacts) > 0 && v.Freshness != "stale" {
		v.Freshness = "requires_cli_check"
		if live {
			v.Freshness = "matched"
			for _, ref := range p.Artifacts {
				a, err := artifactSelection(st, p.Target, ref.ArtifactID, ref.VersionID)
				if err != nil {
					v.Freshness = "stale"
					continue
				}
				check := checkArtifact(ctx, st, a, time.Now())
				v.Artifacts = append(v.Artifacts, check)
				if check.Status != "matched" {
					v.Freshness = "stale"
				}
			}
		}
	}
	return v
}

func (s Service) ProgressShow(ctx context.Context, target, id string) (ProgressView, error) {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	st, err := s.Store.State(ctx)
	if err != nil {
		return ProgressView{}, err
	}
	p, ok := st.ProgressProposals[id]
	if !ok || p.Target != target {
		return ProgressView{}, fmt.Errorf("proposal not found for target")
	}
	v := progressView(ctx, st, p, true)
	after, err := s.Store.State(ctx)
	if err != nil {
		return v, err
	}
	if after.LastEventID != st.LastEventID {
		v.Freshness = "state_changed"
	}
	return v, nil
}

// List is bounded and uses recorded state only; show performs fresh observation.
func (s Service) ProgressList(ctx context.Context, target, after string, limit int) ([]ProgressView, error) {
	if limit < 1 || limit > 50 || (after != "" && !model.OpaqueID.MatchString(after)) {
		return nil, fmt.Errorf("progress list requires limit 1..50 and a valid after ID")
	}
	st, err := s.Store.State(ctx)
	if err != nil {
		return nil, err
	}
	if _, _, err := model.ResolveTarget(st, target); err != nil {
		return nil, err
	}
	ids := []string{}
	for id, p := range st.ProgressProposals {
		if p.Target == target && id > after {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	out, size := []ProgressView{}, 2
	for _, id := range ids {
		v := progressView(ctx, st, st.ProgressProposals[id], false)
		raw, _ := json.Marshal(v)
		if len(out) == limit || size+len(raw)+1 > MaxReadBytes {
			break
		}
		out, size = append(out, v), size+len(raw)+1
	}
	return out, nil
}
