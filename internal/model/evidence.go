package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"reflect"
	"slices"
	"sort"
	"time"
)

// Evaluator definitions are accepted through the local CLI, never from progress
// text or a machine-supplied result. A replacement preserves its predecessor.
type EvaluatorSpec struct {
	Kind           string   `json:"kind"`
	ResourceID     string   `json:"resource_id"`
	ExpectedDigest string   `json:"expected_digest,omitempty"`
	ExpectedCommit string   `json:"expected_commit,omitempty"`
	RequireClean   bool     `json:"require_clean,omitempty"`
	Argv           []string `json:"argv,omitempty"`
	TimeoutSeconds int      `json:"timeout_seconds,omitempty"`
}
type Evaluator struct {
	Version    int           `json:"version"`
	ID         string        `json:"id"`
	Target     string        `json:"target"`
	CheckID    string        `json:"check_id"`
	ContractID string        `json:"contract_id"`
	Previous   string        `json:"previous"`
	Spec       EvaluatorSpec `json:"spec"`
	Digest     string        `json:"digest"`
	Actor      string        `json:"actor"`
	At         time.Time     `json:"at"`
}
type Evidence struct {
	Version           int               `json:"version"`
	ID                string            `json:"id"`
	EvaluatorID       string            `json:"evaluator_id"`
	Target            string            `json:"target"`
	TaskRevision      int64             `json:"task_revision"`
	SourceEvent       int64             `json:"source_event"`
	StartedAt         time.Time         `json:"started_at"`
	FinishedAt        *time.Time        `json:"finished_at,omitempty"`
	Status            string            `json:"status"`
	Outcome           string            `json:"outcome"`
	Reason            string            `json:"reason,omitempty"`
	Observer          string            `json:"observer"`
	EvaluatorVersion  string            `json:"evaluator_version"`
	Inputs            []ResourceVersion `json:"inputs"`
	Context           []ContextVersion  `json:"context"`
	DecisionDigest    string            `json:"decision_digest"`
	OutputDigest      string            `json:"output_digest,omitempty"`
	ExecutableDigest  string            `json:"executable_digest,omitempty"`
	EnvironmentDigest string            `json:"environment_digest,omitempty"`
	OutputBytes       int64             `json:"output_bytes,omitempty"`
	ExitCode          *int              `json:"exit_code,omitempty"`
	Repo              *RepoIdentity     `json:"repo,omitempty"`
}
type RepoIdentity struct {
	Root         string `json:"root"`
	Commit       string `json:"commit"`
	StatusDigest string `json:"status_digest"`
	Clean        bool   `json:"clean"`
}
type EvidenceInvalidation struct {
	ID     string    `json:"id"`
	Reason string    `json:"reason"`
	At     time.Time `json:"at"`
}

func ContentDigest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func EvaluatorKey(target, check string) string { return target + "/" + check }
func (d Evaluator) Current(st State) bool {
	c, ok := st.Contracts[d.ContractID]
	if !ok || c.Version != 2 || st.ContractHeads[d.Target] != c.ID {
		return false
	}
	r, step, err := ResolveTarget(st, d.Target)
	if err != nil || r.Revision != c.TaskRevision {
		return false
	}
	done := r.Task.Done
	if step != nil {
		done = step.Done
	}
	if !reflect.DeepEqual(done, c.Acceptance) {
		return false
	}
	scope, err := ResourceScope(st, d.Target)
	if err != nil || !slices.Equal(scope, c.ResourceIDs) || !Contains(scope, d.Spec.ResourceID) {
		return false
	}
	for _, check := range done.Checks {
		if check.ID == d.CheckID && check.Kind == d.Spec.Kind {
			return true
		}
	}
	return false
}
func (d Evaluator) Validate() error {
	if err := ValidRecord(d.Version, d.ID, d.Target, d.Actor, d.At); err != nil {
		return err
	}
	if !OpaqueID.MatchString(d.ContractID) || !OpaqueID.MatchString(d.Spec.ResourceID) || !localID.MatchString(d.CheckID) || d.Digest != ContentDigest(d.Spec) {
		return fmt.Errorf("invalid evaluator identity or digest")
	}
	s := d.Spec
	switch s.Kind {
	case "artifact.exists", "artifact.digest":
		if len(s.Argv) > 0 || s.TimeoutSeconds != 0 || s.ExpectedCommit != "" || s.RequireClean {
			return fmt.Errorf("artifact evaluator has foreign parameters")
		}
		if s.Kind == "artifact.digest" {
			b, err := hex.DecodeString(s.ExpectedDigest)
			if err != nil || len(b) != 32 {
				return fmt.Errorf("expected snapshot digest required")
			}
		} else if s.ExpectedDigest != "" {
			return fmt.Errorf("existence check cannot declare digest")
		}
	case "repo.state":
		if len(s.Argv) > 0 || s.TimeoutSeconds != 0 || s.ExpectedDigest != "" || (!s.RequireClean && s.ExpectedCommit == "") {
			return fmt.Errorf("repo predicate required")
		}
		if s.ExpectedCommit != "" {
			b, err := hex.DecodeString(s.ExpectedCommit)
			if err != nil || (len(b) != 20 && len(b) != 32) {
				return fmt.Errorf("full commit hash required")
			}
		}
	case "test.exit":
		if len(s.Argv) == 0 || len(s.Argv) > 64 || !filepath.IsAbs(s.Argv[0]) || s.TimeoutSeconds < 1 || s.TimeoutSeconds > 300 || s.ExpectedDigest != "" || s.ExpectedCommit != "" || s.RequireClean {
			return fmt.Errorf("test requires absolute executable, argv and 1..300 second timeout")
		}
		for _, arg := range s.Argv {
			if len(arg) > 4096 {
				return fmt.Errorf("argument too large")
			}
		}
	default:
		return fmt.Errorf("unsupported evaluator")
	}
	return nil
}

// EvidenceCurrent uses recorded versions only. Filesystem freshness is checked
// separately before ratification; replay never observes or executes anything.
func EvidenceCurrent(st State, e Evidence) bool {
	lineage, err := EvidenceContext(st, e.Target)
	if err != nil || !reflect.DeepEqual(lineage, e.Context) || EvidenceDecisionDigest(st, e.Target) != e.DecisionDigest {
		return false
	}
	d, ok := st.Evaluators[e.EvaluatorID]
	if !ok || !d.Current(st) || st.EvaluatorHeads[EvaluatorKey(d.Target, d.CheckID)] != d.ID {
		return false
	}
	if _, invalid := st.EvidenceInvalidations[e.ID]; invalid {
		return false
	}
	r, _, err := ResolveTarget(st, e.Target)
	return err == nil && r.Revision == e.TaskRevision && e.Status == "finished"
}

func EvidenceDecisionDigest(st State, target string) string {
	lineage, err := TargetLineage(st, target)
	if err != nil {
		return ""
	}
	ids := []string{}
	for id, d := range st.Decisions {
		if Contains(lineage, d.Target) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	return ContentDigest(ids)
}

func EvidenceContext(st State, target string) ([]ContextVersion, error) {
	targets, err := TargetLineage(st, target)
	if err != nil {
		return nil, err
	}
	versions := []ContextVersion{}
	for _, target := range targets {
		r, _, err := ResolveTarget(st, target)
		if err != nil {
			return nil, err
		}
		versions = append(versions, ContextVersion{Target: target, TaskRevision: r.Revision, ContractID: st.ContractHeads[target]})
	}
	return versions, nil
}
func LatestEvidence(st State, target, check string) (Evidence, bool) {
	id := st.EvaluatorHeads[EvaluatorKey(target, check)]
	var latest Evidence
	for _, e := range st.Evidence {
		if e.EvaluatorID == id && e.Target == target && (latest.ID == "" || e.SourceEvent > latest.SourceEvent) {
			latest = e
		}
	}
	return latest, latest.ID != "" && EvidenceCurrent(st, latest)
}
