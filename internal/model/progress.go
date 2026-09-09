package model

import (
	"fmt"
	"reflect"
	"slices"
	"strings"
	"time"
)

// Progress records review of a frozen proposal. It never changes task status or
// constitutes evaluator evidence. Old decision.accepted events retain v1 rules.
type ProgressArtifact struct {
	ArtifactID  string              `json:"artifact_id"`
	VersionID   string              `json:"version_id"`
	Observation ArtifactObservation `json:"observation"`
}

type ProgressProposal struct {
	Version        int                `json:"version"`
	ID             string             `json:"id"`
	Target         string             `json:"target"`
	TaskRevision   int64              `json:"task_revision"`
	Kind           string             `json:"kind"`
	Text           string             `json:"text"`
	ContractID     string             `json:"contract_id"`
	Context        []ContextVersion   `json:"context"`
	DecisionDigest string             `json:"decision_digest"`
	Artifacts      []ProgressArtifact `json:"artifacts"`
	Supersedes     string             `json:"supersedes,omitempty"`
	Previous       string             `json:"previous,omitempty"`
	Digest         string             `json:"digest"`
	Actor          string             `json:"actor"`
	At             time.Time          `json:"at"`
}

func (p ProgressProposal) ContentDigest() string {
	p.Digest = ""
	return ContentDigest(p)
}

func (p ProgressProposal) Validate() error {
	if err := ValidRecord(p.Version, p.ID, p.Target, p.Actor, p.At); err != nil {
		return err
	}
	if (strings.TrimSpace(p.Text) == "" || len(p.Text) > 8192) || p.TaskRevision < 1 || !OpaqueID.MatchString(p.ContractID) || !artifactDigest.MatchString(p.DecisionDigest) || p.Digest != p.ContentDigest() || len(p.Context) == 0 {
		return fmt.Errorf("invalid progress proposal content or digest")
	}
	if (p.Previous != "" && !OpaqueID.MatchString(p.Previous)) || (p.Supersedes != "" && !OpaqueID.MatchString(p.Supersedes)) {
		return fmt.Errorf("invalid progress predecessor")
	}
	switch p.Kind {
	case "decision":
		if p.Previous != "" {
			return fmt.Errorf("decision proposal cannot replace artifact progress")
		}
	case "artifact":
		if len(p.Artifacts) != 1 || p.Supersedes != "" {
			return fmt.Errorf("artifact progress requires one version and no decision supersession")
		}
	default:
		return fmt.Errorf("unknown progress kind")
	}
	refs := p.ArtifactRefs()
	if len(refs) > 0 {
		return ValidArtifactRefs(refs)
	}
	return nil
}

func (p ProgressProposal) ArtifactRefs() []ArtifactRef {
	refs := []ArtifactRef{}
	for _, a := range p.Artifacts {
		refs = append(refs, ArtifactRef{a.ArtifactID, a.VersionID})
	}
	return refs
}

type ProgressReview struct {
	Authority    *ProgressAuthority `json:"authority,omitempty"`
	Version      int                `json:"version"`
	ID           string             `json:"id"`
	Target       string             `json:"target"`
	TaskRevision int64              `json:"task_revision"`
	ProposalID   string             `json:"proposal_id"`
	Digest       string             `json:"digest"`
	Previous     string             `json:"previous"`
	Status       string             `json:"status"`
	Note         string             `json:"note"`
	Actor        string             `json:"actor"`
	At           time.Time          `json:"at"`
}

func (r ProgressReview) Validate() error {
	actor := r.Actor
	switch r.Version {
	case 1:
		if r.Authority != nil {
			return fmt.Errorf("CLI review cannot declare UI authority")
		}
	case 2:
		if r.Authority == nil || r.Authority.Validate() != nil || r.Actor != "ui:"+r.Authority.SessionID || r.At.Before(r.Authority.At) || !r.At.Before(r.Authority.ExpiresAt) {
			return fmt.Errorf("invalid UI review authority")
		}
		actor = "cli"
	default:
		return fmt.Errorf("unsupported progress review version")
	}
	if err := ValidRecord(1, r.ID, r.Target, actor, r.At); err != nil {
		return err
	}
	if r.TaskRevision < 1 || !OpaqueID.MatchString(r.ProposalID) || !artifactDigest.MatchString(r.Digest) || (r.Previous != "" && !OpaqueID.MatchString(r.Previous)) || !Contains([]string{"reviewed", "accepted", "rejected"}, r.Status) || (strings.TrimSpace(r.Note) == "" || len(r.Note) > 4096) {
		return fmt.Errorf("invalid progress review")
	}
	return nil
}

// Historical UI review v2 records the non-secret scope enabled at sign-in.
// The browser interface is retired; this type remains solely for strict replay.
type ProgressAuthority struct {
	SessionID   string    `json:"session_id"`
	Target      string    `json:"target"`
	ResourceIDs []string  `json:"resource_ids"`
	At          time.Time `json:"at"`
	ExpiresAt   time.Time `json:"expires_at"`
}

func (a ProgressAuthority) Validate() error {
	if !OpaqueID.MatchString(a.SessionID) || !ValidID(a.Target) || a.At.IsZero() || !a.ExpiresAt.After(a.At) || a.ExpiresAt.Sub(a.At) > time.Hour || len(a.ResourceIDs) > 256 {
		return fmt.Errorf("invalid progress authority")
	}
	for i, id := range a.ResourceIDs {
		if !OpaqueID.MatchString(id) || (i > 0 && a.ResourceIDs[i-1] >= id) {
			return fmt.Errorf("invalid progress authority resources")
		}
	}
	return nil
}
func (a ProgressAuthority) Permits(st State, p ProgressProposal) bool {
	root, step, err := ResolveTarget(st, a.Target)
	if err != nil || step != nil || root.Task.Parent != "" {
		return false
	}
	g := Grant{Target: a.Target, Subtree: true, ResourceIDs: a.ResourceIDs}
	targets, err := TargetLineage(st, p.Target)
	if err != nil {
		return false
	}
	for _, target := range targets {
		if !g.Contains(st, target) {
			return false
		}
	}
	ids, err := ResourceScope(st, p.Target)
	if err != nil {
		return false
	}
	for _, id := range ids {
		if !Contains(a.ResourceIDs, id) {
			return false
		}
	}
	// Historical pins also require permission, including when rejecting a stale
	// proposal after unbinding a resource. No live observation is needed here.
	for _, pin := range p.Artifacts {
		v, ok := st.ArtifactVersions[pin.VersionID]
		if !ok || v.Target != p.Target || v.ArtifactID != pin.ArtifactID || !Contains(a.ResourceIDs, v.ResourceID) {
			return false
		}
	}
	return true
}

// ProgressCurrent checks only recorded state. Live artifact identity is checked
// by the service at proposal/review time, never by replay or scoped readers.
func ProgressCurrent(st State, p ProgressProposal) error {
	r, _, err := ResolveTarget(st, p.Target)
	if err != nil || r.Revision != p.TaskRevision {
		return fmt.Errorf("task revision changed")
	}
	context, err := EvidenceContext(st, p.Target)
	if err != nil || !reflect.DeepEqual(context, p.Context) || EvidenceDecisionDigest(st, p.Target) != p.DecisionDigest {
		return fmt.Errorf("accepted direction changed")
	}
	c, ok := st.Contracts[p.ContractID]
	scope, err := ResourceScope(st, p.Target)
	if !ok || err != nil || c.Target != p.Target || c.Version != 2 || st.ContractHeads[p.Target] != c.ID || c.TaskRevision != p.TaskRevision || !slices.Equal(scope, c.ResourceIDs) {
		return fmt.Errorf("accepted contract or resource scope changed")
	}
	// Inherited contracts, when present, must also still describe their scope.
	for _, v := range context {
		if v.ContractID == "" {
			continue
		}
		c := st.Contracts[v.ContractID]
		scope, err := ResourceScope(st, v.Target)
		if err != nil || c.Version != 2 || c.TaskRevision != v.TaskRevision || !slices.Equal(scope, c.ResourceIDs) {
			return fmt.Errorf("inherited contract is stale or unreviewed")
		}
	}
	if p.Supersedes != "" {
		old, ok := st.Decisions[p.Supersedes]
		if !ok || old.Target != p.Target {
			return fmt.Errorf("superseded decision outside target")
		}
		for _, d := range st.Decisions {
			if d.Supersedes == old.ID {
				return fmt.Errorf("decision already superseded")
			}
		}
	}
	for _, a := range p.Artifacts {
		identity, ok := st.Artifacts[a.ArtifactID]
		v, exists := st.ArtifactVersions[a.VersionID]
		if !ok || !exists || identity.Target != p.Target || v.Target != p.Target || v.ArtifactID != a.ArtifactID || st.ArtifactHeads[a.ArtifactID] != a.VersionID || !reflect.DeepEqual(v.Observation, a.Observation) || !Contains(scope, v.ResourceID) {
			return fmt.Errorf("artifact version, identity or scope changed")
		}
	}
	return nil
}

// ProgressStatus preserves accepted/rejected history even if current inputs
// drift. Superseded versions and replaced artifact proposals are explicit.
func ProgressStatus(st State, p ProgressProposal) string {
	if p.Kind == "artifact" {
		a := p.Artifacts[0]
		if st.ArtifactHeads[a.ArtifactID] != a.VersionID || st.ArtifactProgressHeads[a.ArtifactID] != p.ID {
			return "superseded"
		}
	}
	r := st.ProgressReviews[st.ProgressReviewHeads[p.ID]]
	if p.Kind == "decision" && r.Status == "accepted" {
		for _, d := range st.Decisions {
			if d.Supersedes == r.ID {
				return "superseded"
			}
		}
	}
	if r.ID != "" {
		return r.Status
	}
	return "draft"
}
