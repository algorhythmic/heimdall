package model

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Preservation is a manual handoff and observations, never a dispatch journal.
// No receipt is task evidence or permission to copy, commit, push or retry.
type PreservationSelection struct {
	ArtifactID string `json:"artifact_id"`
	VersionID  string `json:"version_id"`
	MirrorPath string `json:"mirror_path"`
}
type PreservationInput struct {
	CheckpointID string                  `json:"checkpoint_id"`
	PrivateRoot  string                  `json:"private_root"`
	Remote       string                  `json:"remote"`
	Ref          string                  `json:"ref"`
	Selections   []PreservationSelection `json:"selections"`
}
type PreservedFile struct {
	Status string `json:"status"`
	Digest string `json:"digest,omitempty"`
	Bytes  int64  `json:"bytes"`
}
type PreservationItem struct {
	ArtifactID     string        `json:"artifact_id"`
	VersionID      string        `json:"version_id"`
	ExpectedDigest string        `json:"expected_digest"`
	Source         PreservedFile `json:"source"`
	Mirror         PreservedFile `json:"mirror"`
	Committed      PreservedFile `json:"committed"`
}
type PreservationSnapshot struct {
	Items             []PreservationItem `json:"items"`
	RepositoryStatus  string             `json:"repository_status"`
	Head              string             `json:"head,omitempty"`
	Ref               string             `json:"ref,omitempty"`
	DirtyCount        int                `json:"dirty_count"`
	UnrelatedCount    int                `json:"unrelated_count"`
	WorkingDigest     string             `json:"working_digest,omitempty"`
	RemoteFingerprint string             `json:"remote_fingerprint,omitempty"`
	RemoteStatus      string             `json:"remote_status"`
	RemoteCommit      string             `json:"remote_commit,omitempty"`
	Stage             string             `json:"stage"`
}
type PreservationPlan struct {
	Version       int                  `json:"version"`
	ID            string               `json:"id"`
	Target        string               `json:"target"`
	TaskRevision  int64                `json:"task_revision"`
	Host          string               `json:"host"`
	Input         PreservationInput    `json:"input"`
	PreviewDigest string               `json:"preview_digest"`
	Snapshot      PreservationSnapshot `json:"snapshot"`
	Actor         string               `json:"actor"`
	At            time.Time            `json:"at"`
}
type PreservationReceipt struct {
	Version         int                  `json:"version"`
	ID              string               `json:"id"`
	Target          string               `json:"target"`
	TaskRevision    int64                `json:"task_revision"`
	PlanID          string               `json:"plan_id"`
	Previous        string               `json:"previous"`
	ReportedOutcome string               `json:"reported_outcome"`
	Note            string               `json:"note"`
	Snapshot        PreservationSnapshot `json:"snapshot"`
	Actor           string               `json:"actor"`
	At              time.Time            `json:"at"`
}

func PreservationDigest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func ValidPreservationPath(s string) bool {
	if !ValidArtifactPath(s) || s == "." {
		return false
	}
	for _, p := range strings.Split(s, "/") {
		if p == ".git" {
			return false
		}
	}
	return !strings.ContainsAny(s, "\t\x1b")
}
func (p PreservationInput) Validate() error {
	if !OpaqueID.MatchString(p.CheckpointID) || !declaredPath("linux", p.PrivateRoot) || !ValidArtifactEnvironment(p.Remote) || !strings.HasPrefix(p.Ref, "refs/heads/") || len(p.Ref) > 256 || strings.ContainsAny(p.Ref, " ~^:?*[\\\x00\t\r\n\x1b") || strings.Contains(p.Ref, "..") || strings.Contains(p.Ref, "@{") || strings.HasSuffix(p.Ref, "/") || len(p.Selections) < 1 || len(p.Selections) > 16 {
		return fmt.Errorf("invalid preservation selection, private root, remote or branch ref")
	}
	paths := map[string]bool{}
	for i, s := range p.Selections {
		if !OpaqueID.MatchString(s.ArtifactID) || !OpaqueID.MatchString(s.VersionID) || !ValidPreservationPath(s.MirrorPath) || paths[s.MirrorPath] || (i > 0 && p.Selections[i-1].ArtifactID >= s.ArtifactID) {
			return fmt.Errorf("preservation selections must have unique mirror paths and sorted artifact IDs")
		}
		paths[s.MirrorPath] = true
	}
	return nil
}
func PreservationPins(st State, target string, in PreservationInput) error {
	c, ok := st.Checkpoints[in.CheckpointID]
	if !ok || c.Target != target {
		return fmt.Errorf("checkpoint not found for target")
	}
	for _, s := range in.Selections {
		found := false
		for _, a := range c.Artifacts {
			if a.ArtifactID == s.ArtifactID && a.VersionID == s.VersionID {
				found = true
			}
		}
		v := st.ArtifactVersions[s.VersionID]
		if !found || v.ArtifactID != s.ArtifactID || v.Target != target {
			return fmt.Errorf("selection must pin an exact artifact version in the saved checkpoint")
		}
	}
	return nil
}
func (s PreservationSnapshot) Validate(in PreservationInput, st State) error {
	if len(s.Items) != len(in.Selections) || !Contains([]string{"available", "unavailable", "rebase_conflict", "changed_during_observation"}, s.RepositoryStatus) || !Contains([]string{"not_checked", "unavailable", "missing", "matched", "different", "config_changed"}, s.RemoteStatus) || s.DirtyCount < 0 || s.UnrelatedCount < 0 || s.UnrelatedCount > s.DirtyCount {
		return fmt.Errorf("invalid preservation observation")
	}
	for _, oid := range []string{s.Head, s.RemoteCommit} {
		if oid != "" && !artifactOID.MatchString(oid) {
			return fmt.Errorf("invalid observed commit")
		}
	}
	for _, digest := range []string{s.WorkingDigest, s.RemoteFingerprint} {
		if digest != "" && !artifactDigest.MatchString(digest) {
			return fmt.Errorf("invalid observed digest")
		}
	}
	if s.RepositoryStatus == "unavailable" && (s.Head != "" || s.WorkingDigest != "" || s.RemoteCommit != "") {
		return fmt.Errorf("unavailable repository cannot confirm revisions")
	}
	if s.RepositoryStatus != "unavailable" && (s.Head == "" || s.WorkingDigest == "") {
		return fmt.Errorf("repository observation requires revisions")
	}
	if (s.RemoteStatus == "matched" || s.RemoteStatus == "different") != (s.RemoteCommit != "") {
		return fmt.Errorf("remote commit requires confirmed readback")
	}
	if (s.RemoteStatus == "matched" && (s.Head == "" || s.Head != s.RemoteCommit)) || (s.RemoteStatus == "different" && (s.RemoteCommit == "" || s.Head == s.RemoteCommit)) {
		return fmt.Errorf("invalid remote confirmation")
	}
	for i, item := range s.Items {
		sel := in.Selections[i]
		v := st.ArtifactVersions[sel.VersionID]
		if item.ArtifactID != sel.ArtifactID || item.VersionID != sel.VersionID || item.ExpectedDigest != v.Observation.Digest {
			return fmt.Errorf("observation selection mismatch")
		}
		for _, f := range []PreservedFile{item.Source, item.Mirror, item.Committed} {
			if !Contains([]string{"matched", "content_changed", "missing", "refused", "unavailable", "changed_during_observation", "scope_changed", "host_changed"}, f.Status) || f.Bytes < 0 || f.Bytes > 64<<20 {
				return fmt.Errorf("invalid file observation")
			}
			known := f.Status == "matched" || f.Status == "content_changed"
			if known != artifactDigest.MatchString(f.Digest) || (!known && (f.Digest != "" || f.Bytes != 0)) || (f.Status == "matched" && (f.Digest != item.ExpectedDigest || f.Bytes != v.Observation.Bytes)) || (f.Status == "content_changed" && f.Digest == item.ExpectedDigest && f.Bytes == v.Observation.Bytes) {
				return fmt.Errorf("invalid file digest observation")
			}
		}
	}
	if s.Stage != s.DeriveStage() {
		return fmt.Errorf("invalid preservation stage")
	}
	return nil
}
func (s PreservationSnapshot) DeriveStage() string {
	if s.RepositoryStatus == "changed_during_observation" {
		return "uncertain"
	}
	mirrored, committed := len(s.Items) > 0, len(s.Items) > 0
	for _, i := range s.Items {
		mirrored = mirrored && i.Mirror.Status == "matched"
		committed = committed && i.Committed.Status == "matched"
	}
	if committed && s.Head != "" {
		if s.RemoteStatus == "matched" {
			return "published"
		}
		return "committed"
	}
	if mirrored {
		return "mirrored"
	}
	return "not_preserved"
}
func ValidPreservationReport(outcome, note string) bool {
	return Contains([]string{"not_reported", "succeeded", "offline", "refused_file", "missing_original", "rebase_conflict", "source_changed_during_copy", "push_failed", "unknown"}, outcome) && len(note) <= 4096 && (outcome == "not_reported" || strings.TrimSpace(note) != "")
}

func ValidPreservationDigest(digest string) bool { return artifactDigest.MatchString(digest) }
