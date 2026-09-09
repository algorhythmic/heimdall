package store

import (
	"fmt"
	"heimdall/internal/model"
	"time"
)

func queueNativeAction(st *model.State, e Event, v model.ActionIntent) error {
	op := st.WorkspaceOperations[v.Native.OperationID]
	if !model.Contains(op.ActionIDs, v.ID) || !model.WorkspaceActionScope(op, v) || !v.At.Equal(op.Intent.At) || (v.Native.Kind == "open" && v.Native.Launch == nil) || v.Native.SourceID != op.Intent.SourceID || v.Native.SourceEpoch != op.Intent.SourceEpoch {
		return fmt.Errorf("native action lacks exact operation authority")
	}
	owned := false
	for _, surface := range st.WorkspaceManifests[v.ManifestID].Surfaces {
		if surface.ID == v.SurfaceID {
			owned = true
			if surface.Kind == "browser" {
				return fmt.Errorf("browser surface requires its scoped application adapter")
			}
			recipe := st.ApplicationRecipes[v.Native.RecipeID]
			terminalClose := surface.Kind == "terminal" && model.ApplicationRecipeCurrent(*st, recipe.ID, v.Target, v.SurfaceID) && model.Contains([]string{"graceful_session_end", "detach"}, recipe.Spec.ClosePolicy) && v.Native.SessionBindingID == recipe.Spec.SessionBindingID
			if v.Native.Launch != nil && ((!v.Native.Launch.Editor && surface.Kind != "terminal") || (v.Native.Launch.Editor && surface.Kind != "editor") || (st.ApplicationRecipes[v.Native.Launch.RecipeID].Spec.Editor != nil) != v.Native.Launch.Editor || st.ApplicationRecipes[v.Native.Launch.RecipeID].Spec.SessionBindingID != v.Native.Launch.SessionBindingID) {
				return fmt.Errorf("launch requires a generic terminal surface")
			}
			if v.Native.Kind == "close" && !terminalClose && (surface.Kind != "native" || st.SessionBindings[st.SessionHeads[surface.ID]].Active) {
				return fmt.Errorf("graceful native close cannot detach an application session")
			}
			if v.Native.SessionBindingID != "" {
				b := st.ViewportBindings[v.Native.ViewportBindingID]
				a := st.Actions[b.ApplicationActionID]
				if a.Intent.Native == nil || a.Intent.Native.Launch == nil || a.Intent.Native.Launch.SessionBindingID != v.Native.SessionBindingID || b.SessionBindingID != v.Native.SessionBindingID {
					return fmt.Errorf("detach requires a previously associated direct-attach view")
				}
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
	sessionID := n.SessionBindingID
	if n.Launch != nil {
		sessionID = n.Launch.SessionBindingID
	}
	if (r.SessionBindingID == "") != (r.SessionDigest == "") || (r.SessionBindingID != "" && (r.SessionBindingID != sessionID || st.SessionHeads[a.Intent.SurfaceID] != sessionID || r.SessionDigest != model.ApplicationSessionDigest(st.SessionBindings[sessionID]))) {
		return fmt.Errorf("session readback differs from pinned session")
	}
	source := st.DesktopSources[n.SourceID]
	if r.Version != 1 || r.ActionID != a.Intent.ID || r.AttemptID != a.Intent.AttemptID || r.SourceID != n.SourceID || r.SourceEpoch != n.SourceEpoch || !source.Active || st.DesktopSourceHead != n.SourceID || source.Epoch != n.SourceEpoch || !model.TokenHashPattern.MatchString(r.SnapshotID) || r.AfterEventID != e.ID-2 || r.AfterEventID < a.LastEventID || r.StartedAt.Before(a.UpdatedAt) || r.CapturedAt.Before(r.StartedAt) || r.ReceivedAt.Before(r.CapturedAt) || r.ReceivedAt.Sub(r.StartedAt) > 5*time.Second || !r.ReceivedAt.Equal(e.TS) || !o.ObservedAt.Equal(r.CapturedAt) || o.SourceEpoch != n.SourceEpoch || o.Digest != model.ContentDigest(r) || len(o.Detail) > 512 {
		return fmt.Errorf("native readback scope, ordering or freshness changed")
	}
	if n.Launch == nil && (r.Process != nil || r.LaunchCandidates != 0) {
		return fmt.Errorf("launch proof on non-launch action")
	}
	if n.Launch != nil {
		if r.LaunchCandidates < 0 || r.LaunchCandidates > 4096 || (r.Process != nil && !r.Process.Valid()) {
			return fmt.Errorf("invalid launch process proof")
		}
		if r.Window != nil && (r.Process == nil || r.LaunchCandidates != 1 || r.Window.PID != r.Process.PID || r.Window.Class != model.ApplicationClass(a.Intent.AttemptID) || a.Report == nil || a.Report.Native == nil || a.Report.Native.Process == nil || *r.Process != *a.Report.Native.Process) {
			return fmt.Errorf("launch window differs from original process receipt")
		}
	}
	if r.Window != nil && ((n.Launch == nil && (n.Window == nil || r.Window.Identity != *n.Window)) || r.Window.Identity.Validate() != nil || r.Window.Identity.SourceEpoch != n.SourceEpoch || len(r.Window.Title) > 512 || len(r.Window.Class) > 512 || len(r.Window.Address) > 18 || r.Window.PID < 1 || r.Window.Size[0] < 0 || r.Window.Size[1] < 0 || r.Workspace == "" || len(r.Workspace) > 256) {
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
		if !v.Observation.Native.Complete || (n.Launch == nil && v.Observation.Native.Window == nil) || (n.Launch != nil && (v.Observation.Native.Window != nil || v.Observation.Native.LaunchCandidates != 0 || v.Observation.Native.Process != nil)) || v.Observation.Status != "unknown" {
			return fmt.Errorf("dispatch requires fresh existing exact window")
		}
		if sessionID := n.SessionBindingID; sessionID != "" && v.Observation.Native.SessionBindingID != sessionID {
			return fmt.Errorf("detach requires fresh original session survival")
		}
		if n.Launch != nil && n.Launch.SessionBindingID != "" && v.Observation.Native.SessionBindingID != n.Launch.SessionBindingID {
			return fmt.Errorf("attach requires fresh original session survival")
		}
		a.Execution, a.DeliveryID, a.DispatchObservation = "dispatching", v.DeliveryID, v.Observation
	case "report":
		r := v.Report
		if e.Actor != "coordinator" || r == nil || r.Native == nil || a.Report != nil || !model.Contains([]string{"dispatching", "uncertain"}, a.Execution) || r.TabID != 0 || r.WindowID != 0 || r.URL != "" || len(r.Detail) > 512 || (r.Native.Acknowledged && !r.Native.Submitted) {
			return fmt.Errorf("invalid native dispatch receipt")
		}
		status := "uncertain"
		if r.Native.Process != nil && (n.Launch == nil || !r.Native.Process.Valid() || !r.Native.Submitted) {
			return fmt.Errorf("invalid launched process receipt")
		}
		if n.Launch != nil && r.Native.Acknowledged && r.Native.Process == nil {
			return fmt.Errorf("launch acknowledgment requires process identity")
		}
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
		status, detail := model.NativeOutcomeInState(st, *a, *v.Observation.Native)
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
