package model

import (
	"fmt"
	"time"
)

// WorkspacePoint is small event/index metadata. Only current heads are retained
// in State; historical records and immutable payloads use dedicated SQL tables.
type WorkspacePoint struct {
	Version       int       `json:"version"`
	ID            string    `json:"id"`
	Target        string    `json:"target"`
	TaskRevision  int64     `json:"task_revision"`
	ManifestID    string    `json:"manifest_id"`
	SourceID      string    `json:"source_id"`
	SourceEpoch   string    `json:"source_epoch"`
	PreviousHead  string    `json:"previous_head"`
	InputDigest   string    `json:"input_digest"`
	ContentDigest string    `json:"content_digest"`
	PayloadDigest string    `json:"payload_digest"`
	PayloadBytes  int       `json:"payload_bytes"`
	Kind          string    `json:"kind"`
	PolicyID      string    `json:"policy_id,omitempty"`
	Coverage      string    `json:"coverage"`
	Published     bool      `json:"published"`
	ObservedAt    time.Time `json:"observed_at"`
	Actor         string    `json:"actor"`
	At            time.Time `json:"at"`
}

type SnapshotBoundary struct {
	Method         string    `json:"method"`
	StartedAt      time.Time `json:"started_at"`
	FinishedAt     time.Time `json:"finished_at"`
	KnownEventGaps uint64    `json:"known_event_gaps"`
}

type WorkspacePointPayload struct {
	Boundary     SnapshotBoundary   `json:"boundary"`
	Version      int                `json:"version"`
	Target       string             `json:"target"`
	ManifestID   string             `json:"manifest_id"`
	TaskRevision int64              `json:"task_revision"`
	SourceEpoch  string             `json:"source_epoch"`
	ObservedAt   time.Time          `json:"observed_at"`
	Coverage     string             `json:"coverage"`
	Monitors     []DesktopMonitor   `json:"monitors"`
	Workspaces   []DesktopWorkspace `json:"workspaces"`
	Surfaces     []SnapshotSurface  `json:"surfaces"`
}
type SnapshotSurface struct {
	SurfaceID         string         `json:"surface_id"`
	ViewportBindingID string         `json:"viewport_binding_id"`
	SessionBindingID  string         `json:"session_binding_id,omitempty"`
	Status            string         `json:"status"`
	Window            *DesktopWindow `json:"window,omitempty"`
}

type SnapshotPolicy struct {
	Version         int       `json:"version"`
	ID              string    `json:"id"`
	Target          string    `json:"target"`
	Previous        string    `json:"previous"`
	Enabled         bool      `json:"enabled"`
	TaskRevision    int64     `json:"task_revision"`
	ManifestID      string    `json:"manifest_id"`
	SourceID        string    `json:"source_id"`
	DebounceSeconds int       `json:"debounce_seconds"`
	MaxDirtySeconds int       `json:"max_dirty_seconds"`
	RetainCount     int       `json:"retain_count"`
	Actor           string    `json:"actor"`
	At              time.Time `json:"at"`
}

// Only active explicit references belong in the projection. C12 operations must
// acquire a durable pin before referencing a point and release it only after
// completion; retention has one shared protection predicate for these references.
type SnapshotPin struct {
	Version    int       `json:"version"`
	ID         string    `json:"id"`
	Target     string    `json:"target"`
	SnapshotID string    `json:"snapshot_id"`
	Previous   string    `json:"previous"`
	Active     bool      `json:"active"`
	Reason     string    `json:"reason"`
	Actor      string    `json:"actor"`
	At         time.Time `json:"at"`
}
type SnapshotPrune struct {
	Version     int       `json:"version"`
	ID          string    `json:"id"`
	Target      string    `json:"target"`
	SnapshotIDs []string  `json:"snapshot_ids"`
	Reason      string    `json:"reason"`
	Actor       string    `json:"actor"`
	At          time.Time `json:"at"`
}

func (v WorkspacePoint) Validate() error {
	if v.Version != 1 || !OpaqueID.MatchString(v.ID) || !ValidID(v.Target) || v.TaskRevision < 1 || !OpaqueID.MatchString(v.ManifestID) || !OpaqueID.MatchString(v.SourceID) || (v.PreviousHead != "" && !OpaqueID.MatchString(v.PreviousHead)) || v.At.IsZero() || v.ObservedAt.IsZero() {
		return fmt.Errorf("invalid workspace snapshot envelope")
	}
	for _, digest := range []string{v.SourceEpoch, v.InputDigest, v.ContentDigest, v.PayloadDigest} {
		if !TokenHashPattern.MatchString(digest) {
			return fmt.Errorf("invalid workspace snapshot digest")
		}
	}
	if v.PayloadBytes < 1 || v.PayloadBytes > 384<<10 || !Contains([]string{"complete", "partial"}, v.Coverage) || v.Published != (v.Coverage == "complete") {
		return fmt.Errorf("invalid snapshot coverage or payload bound")
	}
	if v.Kind == "manual" {
		if v.Actor != "cli" || v.PolicyID != "" {
			return fmt.Errorf("manual snapshot requires CLI authority")
		}
	} else if v.Kind == "automatic" {
		if v.Actor != "snapshotter" || !OpaqueID.MatchString(v.PolicyID) || !v.Published {
			return fmt.Errorf("automatic snapshot requires complete coverage and a policy")
		}
	} else {
		return fmt.Errorf("unknown snapshot kind")
	}
	return nil
}
func (v SnapshotPolicy) Validate() error {
	if err := ValidWorkspaceRecord(v.Version, v.ID, v.Target, v.Previous, v.Actor, v.At, v.TaskRevision); err != nil {
		return err
	}
	if !OpaqueID.MatchString(v.ManifestID) || !OpaqueID.MatchString(v.SourceID) || v.DebounceSeconds < 1 || v.DebounceSeconds > 30 || v.MaxDirtySeconds < v.DebounceSeconds || v.MaxDirtySeconds < 5 || v.MaxDirtySeconds > 300 || v.RetainCount < 2 || v.RetainCount > 256 {
		return fmt.Errorf("invalid snapshot policy; debounce 1..30s, max dirty 5..300s, retain 2..256")
	}
	return nil
}
func (v SnapshotPin) Validate() error {
	if v.Version != 1 || !OpaqueID.MatchString(v.ID) || !ValidID(v.Target) || !OpaqueID.MatchString(v.SnapshotID) || (v.Previous != "" && !OpaqueID.MatchString(v.Previous)) || v.Actor != "cli" || v.At.IsZero() || !workspaceText(v.Reason, 512) {
		return fmt.Errorf("invalid snapshot pin")
	}
	return nil
}
func (v SnapshotPrune) Validate() error {
	if v.Version != 1 || !OpaqueID.MatchString(v.ID) || !ValidID(v.Target) || !Contains([]string{"cli", "snapshotter"}, v.Actor) || v.At.IsZero() || !workspaceText(v.Reason, 512) || len(v.SnapshotIDs) < 1 || len(v.SnapshotIDs) > 256 {
		return fmt.Errorf("invalid snapshot retention record")
	}
	seen := map[string]bool{}
	for _, id := range v.SnapshotIDs {
		if !OpaqueID.MatchString(id) || seen[id] {
			return fmt.Errorf("invalid retained snapshot ID")
		}
		seen[id] = true
	}
	return nil
}
func SnapshotProtected(st State, id string) bool {
	for _, action := range st.Actions {
		if action.Intent.SnapshotID == id && ActionHolds(action) {
			return true
		}
	}
	if _, ok := st.SnapshotPins[id]; ok {
		return true
	}
	for _, head := range st.SnapshotHeads {
		if head.ID == id {
			return true
		}
	}
	return false
}
