package model

import (
	"fmt"
	"path"
	"regexp"
	"strings"
	"time"
)

// Artifact identity is explicit and permanently owned by one target/environment
// on one observed host. Locations belong to immutable versions, not identity.
type Artifact struct {
	Version     int       `json:"version"`
	ID          string    `json:"id"`
	Target      string    `json:"target"`
	Name        string    `json:"name"`
	Environment string    `json:"environment"`
	Host        string    `json:"host"`
	Platform    string    `json:"platform"`
	Actor       string    `json:"actor"`
	At          time.Time `json:"at"`
}
type ArtifactGit struct {
	Repository  string `json:"repository"`
	Worktree    string `json:"worktree"`
	Commit      string `json:"commit"`
	Ref         string `json:"ref"`
	IndexOID    string `json:"index_oid"`
	IndexMode   string `json:"index_mode"`
	HeadOID     string `json:"head_oid"`
	HeadMode    string `json:"head_mode"`
	Dirty       bool   `json:"dirty"`
	InputDigest string `json:"input_digest"`
}
type ArtifactObservation struct {
	Digest string       `json:"digest"`
	Bytes  int64        `json:"bytes"`
	Mode   uint32       `json:"mode"`
	Git    *ArtifactGit `json:"git,omitempty"`
}
type ArtifactVersion struct {
	Version      int                 `json:"version"`
	ID           string              `json:"id"`
	ArtifactID   string              `json:"artifact_id"`
	Target       string              `json:"target"`
	TaskRevision int64               `json:"task_revision"`
	Previous     string              `json:"previous"`
	ResourceID   string              `json:"resource_id"`
	Path         string              `json:"path"`
	Observation  ArtifactObservation `json:"observation"`
	Actor        string              `json:"actor"`
	At           time.Time           `json:"at"`
}
type ArtifactRef struct {
	ArtifactID string `json:"artifact_id"`
	VersionID  string `json:"version_id"`
}

var artifactEnvironment = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
var artifactDigest = regexp.MustCompile(`^[a-f0-9]{64}$`)
var artifactOID = regexp.MustCompile(`^([a-f0-9]{40}|[a-f0-9]{64})$`)

func ValidArtifactEnvironment(s string) bool { return artifactEnvironment.MatchString(s) }
func ValidArtifactPath(s string) bool {
	return len(s) > 0 && len(s) <= 4096 && !strings.ContainsAny(s, "\\\x00\r\n") && !strings.HasPrefix(s, "/") && path.Clean(s) == s && s != ".." && !strings.HasPrefix(s, "../")
}
func ValidArtifact(a Artifact) error {
	if !workspaceText(a.Name, 256) || !ValidArtifactEnvironment(a.Environment) || !artifactDigest.MatchString(a.Host) || a.Platform != "linux" {
		return fmt.Errorf("invalid artifact identity")
	}
	return ValidRecord(a.Version, a.ID, a.Target, a.Actor, a.At)
}
func ValidArtifactVersion(v ArtifactVersion) error {
	if !OpaqueID.MatchString(v.ArtifactID) || !OpaqueID.MatchString(v.ResourceID) || (v.Previous != "" && !OpaqueID.MatchString(v.Previous)) || !ValidArtifactPath(v.Path) || v.TaskRevision < 1 || !artifactDigest.MatchString(v.Observation.Digest) || v.Observation.Bytes < 0 || v.Observation.Bytes > 64<<20 || v.Observation.Mode > 0777 {
		return fmt.Errorf("invalid artifact version")
	}
	if g := v.Observation.Git; g != nil {
		if !declaredPath("linux", g.Repository) || !declaredPath("linux", g.Worktree) || (g.Commit != "" && !artifactOID.MatchString(g.Commit)) || (g.Ref != "" && (!strings.HasPrefix(g.Ref, "refs/") || !workspaceText(g.Ref, 1024))) || !artifactDigest.MatchString(g.InputDigest) {
			return fmt.Errorf("invalid artifact Git identity")
		}
		for _, p := range [][2]string{{g.IndexOID, g.IndexMode}, {g.HeadOID, g.HeadMode}} {
			if (p[0] == "") != (p[1] == "") || (p[0] != "" && (!artifactOID.MatchString(p[0]) || !Contains([]string{"100644", "100755", "120000"}, p[1]))) {
				return fmt.Errorf("invalid artifact Git entry")
			}
		}
	}
	return ValidRecord(v.Version, v.ID, v.Target, v.Actor, v.At)
}
func ValidArtifactRefs(refs []ArtifactRef) error {
	if len(refs) < 1 || len(refs) > 16 {
		return fmt.Errorf("artifact checkpoint requires 1..16 explicit references")
	}
	for i, r := range refs {
		if !OpaqueID.MatchString(r.ArtifactID) || !OpaqueID.MatchString(r.VersionID) || (i > 0 && refs[i-1].ArtifactID >= r.ArtifactID) {
			return fmt.Errorf("artifact references must be unique and sorted by artifact ID")
		}
	}
	return nil
}

// ArtifactResource checks the declared selector without accessing the filesystem.
func ArtifactResource(r Resource, relative string) (Resource, error) {
	if !r.Active || !declaredPath("linux", r.Root) || !ValidArtifactPath(relative) {
		return r, fmt.Errorf("active resource and local artifact path required")
	}
	if r.Kind == "file" {
		if relative != "." {
			return r, fmt.Errorf("file-bound artifacts use path .")
		}
	} else if r.Kind == "tree" {
		if relative == "." {
			return r, fmt.Errorf("artifact must select a file inside the bound tree")
		}
		for _, part := range strings.Split(relative, "/") {
			if part == ".git" || Contains(r.Exclude, part) {
				return r, fmt.Errorf("artifact path is excluded from resource")
			}
		}
		r.Path = path.Join(strings.ReplaceAll(r.Path, "\\", "/"), relative)
	} else {
		return r, fmt.Errorf("unsupported artifact resource")
	}
	r.Kind = "file"
	r.Exclude = []string{}
	return r, nil
}

// ArtifactOrigin is the first exact-digest transfer observed in a conversation:
// equal content proves equality, never direction; fuzzy matching stays Braid's.
type ArtifactOrigin struct {
	Version        int       `json:"version"`
	VersionID      string    `json:"version_id"`
	ConversationID string    `json:"conversation_id"`
	Evidence       string    `json:"evidence"` // prompt | write_tool | first_message
	RecordKey      string    `json:"record_key"`
	SourceRevision string    `json:"source_revision"`
	At             time.Time `json:"at"`
}

func (o ArtifactOrigin) Validate() error {
	if o.Version != 1 || !OpaqueID.MatchString(o.VersionID) || !OpaqueID.MatchString(o.ConversationID) || !Contains([]string{"prompt", "write_tool", "first_message"}, o.Evidence) || o.RecordKey == "" || len(o.RecordKey) > 1024 || o.At.IsZero() {
		return fmt.Errorf("invalid artifact origin")
	}
	return nil
}
