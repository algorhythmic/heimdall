package workspace

import (
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"time"
)

func DecodePreview(raw []byte) (PreviewRequest, error) {
	var r PreviewRequest
	if len(raw) > MaxRequest {
		return r, fmt.Errorf("preview request exceeds 64 KiB")
	}
	if err := model.StrictJSON(raw, &r); err != nil {
		return r, err
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(raw, &fields)
	if string(fields["max_snapshot_age_seconds"]) == "null" {
		return r, fmt.Errorf("snapshot age limit cannot be null")
	}
	return r, r.Validate()
}

// PreviewRequest selects immutable inputs. It grants no dispatch authority.
type PreviewRequest struct {
	Version               int    `json:"version"`
	Target                string `json:"target"`
	ManifestID            string `json:"manifest_id"`
	SnapshotID            string `json:"snapshot_id"`
	MaxSnapshotAgeSeconds int    `json:"max_snapshot_age_seconds"`
}

func (r PreviewRequest) Validate() error {
	if r.Version != 1 || !model.ValidID(r.Target) || !model.OpaqueID.MatchString(r.ManifestID) || !model.OpaqueID.MatchString(r.SnapshotID) || r.MaxSnapshotAgeSeconds < 0 || r.MaxSnapshotAgeSeconds > 365*86400 {
		return fmt.Errorf("preview requires task, manifest ID, snapshot ID and optional age limit 0..31536000 seconds")
	}
	return nil
}

// Placement uses workspace/monitor names, never numeric IDs across epochs.
// Coordinates are logical global coordinates; tiled split trees are not saved.
type Placement struct {
	Workspace  string `json:"workspace"`
	Monitor    string `json:"monitor"`
	At         [2]int `json:"at"`
	Size       [2]int `json:"size"`
	Floating   bool   `json:"floating"`
	Pinned     bool   `json:"pinned"`
	Hidden     bool   `json:"hidden"`
	Fullscreen int    `json:"fullscreen"`
}

type PreviewSurface struct {
	BrowserReview     *BrowserApplicationReview `json:"browser_review,omitempty"`
	ApplicationRecipe *model.ApplicationRecipe  `json:"application_recipe,omitempty"`
	SurfaceID         string                    `json:"surface_id"`
	Kind              string                    `json:"kind"`
	Label             string                    `json:"label"`
	Required          bool                      `json:"required"`
	Membership        string                    `json:"membership"`
	ViewportBindingID string                    `json:"viewport_binding_id"`
	SessionBindingID  string                    `json:"session_binding_id"`
	Window            *model.WindowIdentity     `json:"window,omitempty"`
	Observed          *Placement                `json:"observed,omitempty"`
	Desired           *Placement                `json:"desired,omitempty"`
	SessionStatus     string                    `json:"session_status"`
	ObservationStatus string                    `json:"observation_status"`
	Disposition       string                    `json:"disposition"`
	Changes           []string                  `json:"changes"`
	Issues            []string                  `json:"issues"`
	Requirements      []string                  `json:"requirements"`
}

type BrowserApplicationReview struct {
	Profile string             `json:"profile"`
	Epoch   string             `json:"epoch"`
	Ready   bool               `json:"ready"`
	Tabs    []model.BrowserTab `json:"tabs"`
}

type Preview struct {
	Version               int                    `json:"version"`
	Request               PreviewRequest         `json:"request"`
	TaskRevision          int64                  `json:"task_revision"`
	InputDigest           string                 `json:"input_digest"`
	SnapshotPayloadDigest string                 `json:"snapshot_payload_digest"`
	SnapshotObservedAt    time.Time              `json:"snapshot_observed_at"`
	SnapshotAgeSeconds    int64                  `json:"snapshot_age_seconds"`
	SnapshotProtected     bool                   `json:"snapshot_protected"`
	SourceID              string                 `json:"source_id"`
	SourceEpoch           string                 `json:"source_epoch"`
	Fresh                 bool                   `json:"fresh"`
	Boundary              model.SnapshotBoundary `json:"boundary"`
	Surfaces              []PreviewSurface       `json:"surfaces"`
	Displays              []model.DesktopMonitor `json:"displays"`
	UnownedLeftOpen       int                    `json:"unowned_left_open"`
	Issues                []string               `json:"issues"`
	ReviewRequired        bool                   `json:"review_required"`
	AsOf                  time.Time              `json:"as_of"`
	ExpiresAt             time.Time              `json:"expires_at"`
	Digest                string                 `json:"digest"`
	// Daemon-lifetime seal authenticates the complete review artifact, including
	// its deadline. Restart requires a new preview; no token authorizes actions.
	Token string `json:"token,omitempty"`
}

type PreviewValidation struct {
	Version int      `json:"version"`
	Current bool     `json:"current"`
	Issue   string   `json:"issue"`
	Preview *Preview `json:"preview,omitempty"`
}

type WorkspaceList struct {
	Target       string                 `json:"target"`
	TaskRevision int64                  `json:"task_revision"`
	ManifestID   string                 `json:"manifest_id"`
	Fresh        bool                   `json:"fresh"`
	Issues       []string               `json:"issues"`
	Boundary     model.SnapshotBoundary `json:"boundary"`
	Surfaces     []ListedSurface        `json:"surfaces"`
}
type ListedSurface struct {
	SurfaceID     string                `json:"surface_id"`
	Kind          string                `json:"kind"`
	Label         string                `json:"label"`
	Status        string                `json:"status"`
	Window        *model.WindowIdentity `json:"window,omitempty"`
	Placement     *Placement            `json:"placement,omitempty"`
	SessionStatus string                `json:"session_status"`
	Issues        []string              `json:"issues"`
}
