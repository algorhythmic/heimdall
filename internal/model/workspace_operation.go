package model

import (
	"fmt"
	"time"
)

const ResidentWorkspaceLimit = 3

// An operation coordinates shared action IDs. It does not contain a second
// execution journal or infer authority from a read-only recovery preview.
type WorkspaceOperationIntent struct {
	Version       int      `json:"version"`
	ID            string   `json:"id"`
	Kind          string   `json:"kind"`
	Target        string   `json:"target"`
	TaskRevision  int64    `json:"task_revision"`
	ManifestID    string   `json:"manifest_id"`
	SnapshotID    string   `json:"snapshot_id"`
	ContextDigest string   `json:"context_digest"`
	InputDigest   string   `json:"input_digest"`
	PreviewDigest string   `json:"preview_digest"`
	SourceID      string   `json:"source_id"`
	SourceEpoch   string   `json:"source_epoch"`
	SurfaceIDs    []string `json:"surface_ids"`
	// A swap includes independent current review pins for the named resident.
	// Its close actions belong to this same operation and precede incoming input.
	Swap      *WorkspaceSwap `json:"swap,omitempty"`
	Actor     string         `json:"actor"`
	At        time.Time      `json:"at"`
	ExpiresAt time.Time      `json:"expires_at"`
}

func (v WorkspaceOperationIntent) Validate() error {
	if v.Version != 1 || !OpaqueID.MatchString(v.ID) || !Contains([]string{"open", "focus", "close"}, v.Kind) || !ValidID(v.Target) || v.TaskRevision < 1 || !OpaqueID.MatchString(v.ManifestID) || !OpaqueID.MatchString(v.SnapshotID) || !OpaqueID.MatchString(v.SourceID) || !TokenHashPattern.MatchString(v.SourceEpoch) || v.Actor != "cli" || v.At.IsZero() || v.ExpiresAt.Sub(v.At) != time.Minute {
		return fmt.Errorf("invalid reviewed workspace operation")
	}
	for _, d := range []string{v.ContextDigest, v.InputDigest, v.PreviewDigest} {
		if !TokenHashPattern.MatchString(d) {
			return fmt.Errorf("workspace operation input pins required")
		}
	}
	if v.SurfaceIDs == nil || len(v.SurfaceIDs) > 32 {
		return fmt.Errorf("explicit bounded surface selection required")
	}
	seen := map[string]bool{}
	for _, id := range v.SurfaceIDs {
		if !OpaqueID.MatchString(id) || seen[id] {
			return fmt.Errorf("invalid or duplicate selected surface")
		}
		seen[id] = true
	}
	if v.Swap != nil {
		s := v.Swap
		if v.Kind != "open" || !ValidID(s.Target) || s.Target == v.Target || s.TaskRevision < 1 || !OpaqueID.MatchString(s.ManifestID) || !OpaqueID.MatchString(s.SnapshotID) {
			return fmt.Errorf("swap requires a named different resident and reviewed close pins")
		}
		for _, d := range []string{s.ContextDigest, s.InputDigest, s.PreviewDigest} {
			if !TokenHashPattern.MatchString(d) {
				return fmt.Errorf("swap review digests required")
			}
		}
	}
	return nil
}

type WorkspaceSwap struct {
	Target        string `json:"target"`
	TaskRevision  int64  `json:"task_revision"`
	ManifestID    string `json:"manifest_id"`
	SnapshotID    string `json:"snapshot_id"`
	ContextDigest string `json:"context_digest"`
	InputDigest   string `json:"input_digest"`
	PreviewDigest string `json:"preview_digest"`
}

type WorkspaceOperation struct {
	Intent          WorkspaceOperationIntent `json:"intent"`
	Revision        int64                    `json:"revision"`
	Status          string                   `json:"status"`
	Outcome         string                   `json:"outcome"`
	ActionIDs       []string                 `json:"action_ids"`
	Slot            int                      `json:"slot"`
	DiffID          string                   `json:"diff_id,omitempty"`
	CloseSnapshotID string                   `json:"close_snapshot_id,omitempty"`
	Unsupported     []WorkspaceIssue         `json:"unsupported"`
	CancelRequested bool                     `json:"cancel_requested"`
	LastReason      string                   `json:"last_reason"`
	LastEventID     int64                    `json:"last_event_id"`
	UpdatedAt       time.Time                `json:"updated_at"`
}

type WorkspaceIssue struct {
	Target    string `json:"target"`
	SurfaceID string `json:"surface_id"`
	Reason    string `json:"reason"`
}

// A census retains only explicitly bound identities. Its complete coverage is
// checked against every active binding, including residents predating W05.
type WorkspaceResidency struct {
	Version      int               `json:"version"`
	ID           string            `json:"id"`
	SourceID     string            `json:"source_id"`
	SourceEpoch  string            `json:"source_epoch"`
	SnapshotID   string            `json:"snapshot_id"`
	AfterEventID int64             `json:"after_event_id"`
	StartedAt    time.Time         `json:"started_at"`
	CapturedAt   time.Time         `json:"captured_at"`
	ReceivedAt   time.Time         `json:"received_at"`
	Bindings     []ResidentBinding `json:"bindings"`
}
type ResidentBinding struct {
	BindingID string `json:"binding_id"`
	Known     bool   `json:"known"`
	Present   bool   `json:"present"`
}

type WorkspaceOperationChange struct {
	Version          int       `json:"version"`
	ID               string    `json:"id"`
	OperationID      string    `json:"operation_id"`
	PreviousRevision int64     `json:"previous_revision"`
	Reason           string    `json:"reason"`
	At               time.Time `json:"at"`
}

type WorkspaceOperationQueued struct {
	Intent      WorkspaceOperationIntent `json:"intent"`
	ActionIDs   []string                 `json:"action_ids"`
	Unsupported []WorkspaceIssue         `json:"unsupported"`
}
type WorkspaceDiff struct {
	Version           int              `json:"version"`
	ID                string           `json:"id"`
	OperationID       string           `json:"operation_id"`
	PreviewDigest     string           `json:"preview_digest"`
	SwapPreviewDigest string           `json:"swap_preview_digest,omitempty"`
	Changes           []WorkspaceIssue `json:"changes"`
	At                time.Time        `json:"at"`
}

// Matching a swap's outgoing close is required before any incoming input.
func WorkspaceSwapReady(st State, op WorkspaceOperation) bool {
	if op.Intent.Swap == nil {
		return true
	}
	slot := st.WorkspaceSlots[op.Slot]
	if slot.Target != op.Intent.Target || slot.PendingTarget != "" {
		return false
	}
	for _, id := range op.ActionIDs {
		a := st.Actions[id]
		if a.Intent.Target == op.Intent.Swap.Target && a.Verification != "matched" {
			return false
		}
	}
	for _, issue := range op.Unsupported {
		if issue.Target == op.Intent.Swap.Target {
			return false
		}
	}
	return true
}

func WorkspaceActionScope(op WorkspaceOperation, v ActionIntent) bool {
	i := op.Intent
	if v.Target == i.Target {
		return v.TaskRevision == i.TaskRevision && v.ManifestID == i.ManifestID && v.ContextDigest == i.ContextDigest && v.SnapshotID == i.SnapshotID && Contains(i.SurfaceIDs, v.SurfaceID)
	}
	s := i.Swap
	return s != nil && v.Target == s.Target && v.TaskRevision == s.TaskRevision && v.ManifestID == s.ManifestID && v.ContextDigest == s.ContextDigest && v.SnapshotID == s.SnapshotID && v.Native != nil && v.Native.Kind == "close"
}

// A pending transfer occupies the outgoing resident's same slot. It cannot
// grant incoming dispatch until exact absence of the outgoing owned views is
// independently observed. Cancellation never implies that a slot is empty.
type WorkspaceSlot struct {
	Number        int       `json:"number"`
	Target        string    `json:"target"`
	PendingTarget string    `json:"pending_target,omitempty"`
	OperationID   string    `json:"operation_id,omitempty"`
	Status        string    `json:"status"`
	ObservationID string    `json:"observation_id,omitempty"`
	UpdatedAt     time.Time `json:"updated_at"`
}

func WorkspaceOperationHolds(o WorkspaceOperation) bool {
	return Contains([]string{"queued", "running", "uncertain"}, o.Status)
}

// Native actions share ActionRecord's intent/dispatch/report/verification
// history. The coordinator must additionally pin the current viewport binding.
type NativeIntent struct {
	OperationID       string          `json:"operation_id"`
	Kind              string          `json:"kind"`
	ViewportBindingID string          `json:"viewport_binding_id"`
	SourceID          string          `json:"source_id"`
	SourceEpoch       string          `json:"source_epoch"`
	Window            *WindowIdentity `json:"window,omitempty"`
	Workspace         string          `json:"workspace,omitempty"`
}

func (n NativeIntent) Validate() error {
	if !OpaqueID.MatchString(n.OperationID) || !OpaqueID.MatchString(n.SourceID) || !TokenHashPattern.MatchString(n.SourceEpoch) || !Contains([]string{"open", "focus", "move", "close"}, n.Kind) {
		return fmt.Errorf("invalid native action authority")
	}
	if n.ViewportBindingID != "" && !OpaqueID.MatchString(n.ViewportBindingID) {
		return fmt.Errorf("invalid viewport binding pin")
	}
	if n.Kind != "open" && (n.Window == nil || n.ViewportBindingID == "") {
		return fmt.Errorf("existing owned native window required")
	}
	if n.Window != nil && (n.Window.Validate() != nil || n.Window.SourceEpoch != n.SourceEpoch) {
		return fmt.Errorf("native action epoch changed")
	}
	if n.Kind == "move" {
		if !workspaceText(n.Workspace, 256) {
			return fmt.Errorf("named destination required")
		}
	} else if n.Workspace != "" {
		return fmt.Errorf("destination on a non-move action")
	}
	return nil
}

func NativePostcondition(n NativeIntent) ActionPostcondition {
	kind := map[string]string{"open": "native_surface_present", "focus": "native_window_focused", "move": "native_workspace_membership", "close": "native_window_absent"}[n.Kind]
	return ActionPostcondition{Kind: kind, RedirectPolicy: "none", LoadCondition: "none"}
}
