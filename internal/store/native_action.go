package store

import (
	"fmt"
	"heimdall/internal/model"
	"time"
)

func queueNativeAction(st *model.State, e Event, v model.ActionIntent) error {
	op := st.WorkspaceOperations[v.Native.OperationID]
	if !model.Contains(op.ActionIDs, v.ID) || !model.WorkspaceActionScope(op, v) || !v.At.Equal(op.Intent.At) || v.Native.Kind == "open" || v.Native.SourceID != op.Intent.SourceID || v.Native.SourceEpoch != op.Intent.SourceEpoch {
		return fmt.Errorf("native action lacks exact operation authority")
	}
	owned := false
	for _, surface := range st.WorkspaceManifests[v.ManifestID].Surfaces {
		if surface.ID == v.SurfaceID {
			owned = true
			if surface.Kind == "browser" {
				return fmt.Errorf("browser surface requires its scoped application adapter")
			}
			if v.Native.Kind == "close" && (surface.Kind != "native" || st.SessionBindings[st.SessionHeads[surface.ID]].Active) {
				return fmt.Errorf("graceful native close cannot detach an application session")
			}
		}
	}
	if !owned || st.WorkspaceManifests[v.ManifestID].TaskRevision != v.TaskRevision || (v.Native.Kind == "close" && op.CloseSnapshotID == "") || !model.SnapshotProtected(*st, v.SnapshotID) {
		return fmt.Errorf("native action requires current owned surface and retained close membership")
	}
	if v.Native.Kind != "close" && op.Intent.Kind == "close" {
		return fmt.Errorf("close operation cannot grant another input")
	}
	if v.Native.Kind == "close" && v.Target == op.Intent.Target && op.Intent.Kind != "close" {
		return fmt.Errorf("only explicit close authorizes primary window closure")
	}
	if conflict := model.ActionConflict(*st, v); conflict != "" {
		return fmt.Errorf("surface has unresolved action: %w", ErrConflict)
	}
	count := 0
	for _, a := range st.Actions {
		if model.ActionHolds(a) {
			count++
		}
	}
	if count >= 128 {
		return fmt.Errorf("unresolved action limit reached")
	}
	st.Actions[v.ID] = model.ActionRecord{Intent: v, IntentDigest: model.ContentDigest(v), Revision: 1, Execution: "queued", Verification: "pending", UpdatedAt: v.At, LastEventID: e.ID}
	return nil
}

func nativeReadback(st model.State, a model.ActionRecord, o *model.ActionObservation, e Event) error {
	if o == nil || o.Native == nil || !model.OpaqueID.MatchString(o.ID) {
		return fmt.Errorf("native readback required")
	}
	r, n := o.Native, a.Intent.Native
	source := st.DesktopSources[n.SourceID]
	if r.Version != 1 || r.ActionID != a.Intent.ID || r.AttemptID != a.Intent.AttemptID || r.SourceID != n.SourceID || r.SourceEpoch != n.SourceEpoch || !source.Active || st.DesktopSourceHead != n.SourceID || source.Epoch != n.SourceEpoch || !model.TokenHashPattern.MatchString(r.SnapshotID) || r.AfterEventID != e.ID-2 || r.AfterEventID < a.LastEventID || r.StartedAt.Before(a.UpdatedAt) || r.CapturedAt.Before(r.StartedAt) || r.ReceivedAt.Before(r.CapturedAt) || r.ReceivedAt.Sub(r.StartedAt) > 5*time.Second || !r.ReceivedAt.Equal(e.TS) || !o.ObservedAt.Equal(r.CapturedAt) || o.SourceEpoch != n.SourceEpoch || o.Digest != model.ContentDigest(r) || len(o.Detail) > 512 {
		return fmt.Errorf("native readback scope, ordering or freshness changed")
	}
	if r.Window != nil && (n.Window == nil || r.Window.Identity != *n.Window || len(r.Window.Title) > 512 || len(r.Window.Class) > 512 || len(r.Window.Address) > 18 || r.Window.PID < 1 || r.Window.Size[0] < 0 || r.Window.Size[1] < 0 || r.Workspace == "" || len(r.Workspace) > 256) {
		return fmt.Errorf("native readback contains a foreign or invalid window")
	}
	if r.Window == nil && r.Workspace != "" {
		return fmt.Errorf("workspace without observed window")
	}
	// Persist only whether this task-owned window is focused; no foreign window identity.
	if r.FocusedWindow != nil && (!r.FocusKnown || n.Window == nil || *r.FocusedWindow != *n.Window || r.Window == nil) {
		return fmt.Errorf("foreign native focus metadata")
	}
	return nil
}

func transitionNativeAction(st model.State, a *model.ActionRecord, v model.ActionTransition, e Event) error {
	if v.Version != 4 {
		return fmt.Errorf("native transition requires version 4")
	}
	n, op := a.Intent.Native, st.WorkspaceOperations[a.Intent.Native.OperationID]
	switch v.Kind {
	case "dispatch":
		if e.Actor != "coordinator" || a.Execution != "queued" || a.CancelRequested || !model.OpaqueID.MatchString(v.DeliveryID) || !v.At.Before(a.Intent.ExpiresAt) || !v.At.Before(op.Intent.ExpiresAt) || !model.ActionInputsCurrent(st, a.Intent) || (a.Intent.Target == op.Intent.Target && !model.WorkspaceSwapReady(st, op)) {
			return fmt.Errorf("native action cannot dispatch")
		}
		if err := nativeReadback(st, *a, v.Observation, e); err != nil {
			return err
		}
		if !v.Observation.Native.Complete || v.Observation.Native.Window == nil || v.Observation.Status != "unknown" {
			return fmt.Errorf("dispatch requires fresh existing exact window")
		}
		a.Execution, a.DeliveryID, a.DispatchObservation = "dispatching", v.DeliveryID, v.Observation
	case "report":
		r := v.Report
		if e.Actor != "coordinator" || r == nil || r.Native == nil || a.Report != nil || !model.Contains([]string{"dispatching", "uncertain"}, a.Execution) || r.TabID != 0 || r.WindowID != 0 || r.URL != "" || len(r.Detail) > 512 || (r.Native.Acknowledged && !r.Native.Submitted) {
			return fmt.Errorf("invalid native dispatch receipt")
		}
		status := "uncertain"
		if r.Native.Acknowledged {
			status = "succeeded"
		}
		if r.Status != status {
			return fmt.Errorf("native receipt cannot assert verification")
		}
		a.Report = r
		a.Execution, a.Verification = "uncertain", "unknown"
		if r.Native.Acknowledged {
			a.Execution, a.Verification = "api_reported", "pending"
		} else if a.UncertainSince.IsZero() {
			a.UncertainSince = v.At
		}
		a.VerificationAttempts = 0
	case "verify":
		if e.Actor != "observer:hyprland" || !model.Contains([]string{"dispatching", "api_reported", "uncertain"}, a.Execution) || a.VerificationAttempts >= 8 {
			return fmt.Errorf("native verification unavailable or exhausted")
		}
		if err := nativeReadback(st, *a, v.Observation, e); err != nil {
			return err
		}
		status, detail := model.NativeOutcome(*a, *v.Observation.Native)
		if v.Observation.Status != status || v.Observation.Detail != detail {
			return fmt.Errorf("native verification differs from independent readback")
		}
		a.Observation, a.Verification = v.Observation, status
		a.VerificationAttempts++
	case "source_lost":
		source := st.DesktopSources[n.SourceID]
		if e.Actor != "coordinator" || !model.ActionHolds(*a) || model.Contains([]string{"queued", "refused", "cancelled"}, a.Execution) || (source.Active && st.DesktopSourceHead == n.SourceID && source.Epoch == n.SourceEpoch) {
			return fmt.Errorf("native source is still current")
		}
		a.Verification = "unknown"
	case "observe":
		o := v.Observation
		if e.Actor != "observer:hyprland" || o == nil || o.Native != nil || o.Status != "unknown" || o.SourceEpoch != n.SourceEpoch || o.Digest != model.ContentDigest(n) || !model.OpaqueID.MatchString(o.ID) || !o.ObservedAt.Equal(v.At) || o.Detail != "Native readback unavailable; no input repeated" || !model.Contains([]string{"dispatching", "api_reported", "uncertain"}, a.Execution) || a.VerificationAttempts >= 8 {
			return fmt.Errorf("invalid native unavailable observation")
		}
		a.Verification, a.Observation = "unknown", o
		a.VerificationAttempts++
	default:
		return fmt.Errorf("unknown native transition")
	}
	return nil
}
