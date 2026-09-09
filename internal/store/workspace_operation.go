package store

import (
	"fmt"
	"heimdall/internal/model"
	"slices"
	"strings"
	"time"
)

func applyWorkspaceResidency(st *model.State, e Event) error {
	var r model.WorkspaceResidency
	if err := model.StrictJSON(e.Payload, &r); err != nil {
		return err
	}
	source := st.DesktopSources[r.SourceID]
	if r.Version != 1 || !model.OpaqueID.MatchString(r.ID) || r.ID != e.EntityID || !model.Contains([]string{"cli", "observer:hyprland"}, e.Actor) || !source.Active || st.DesktopSourceHead != source.ID || r.SourceEpoch != source.Epoch || !model.TokenHashPattern.MatchString(r.SnapshotID) || r.AfterEventID != e.ID-2 || r.StartedAt.IsZero() || r.CapturedAt.Before(r.StartedAt) || r.ReceivedAt.Before(r.CapturedAt) || r.ReceivedAt.Sub(r.StartedAt) > 5*time.Second || !r.ReceivedAt.Equal(e.TS) || r.Bindings == nil || len(r.Bindings) > 1024 {
		return fmt.Errorf("invalid complete residency census")
	}
	seen, residents := map[string]bool{}, map[string]string{}
	for _, row := range r.Bindings {
		b := st.ViewportBindings[row.BindingID]
		if seen[row.BindingID] || !b.Active || b.Window == nil || st.ViewportHeads[b.SurfaceID] != b.ID || row.Known != (b.Window.SourceEpoch == r.SourceEpoch) || (!row.Known && row.Present) {
			return fmt.Errorf("residency contains stale or foreign binding")
		}
		seen[b.ID] = true
		if !row.Known {
			residents[b.Target] = "unknown"
		} else if row.Present && residents[b.Target] != "unknown" {
			residents[b.Target] = "resident"
		}
	}
	for _, id := range st.ViewportHeads {
		b := st.ViewportBindings[id]
		if b.Active && b.Window != nil && !seen[id] {
			return fmt.Errorf("residency omitted an active owned binding")
		}
	}
	for _, a := range st.Actions {
		if model.ActionHolds(a) {
			residents[a.Intent.Target] = "unknown"
		}
	}
	occupied := map[string]bool{}
	for number, slot := range st.WorkspaceSlots {
		op := st.WorkspaceOperations[slot.OperationID]
		if slot.PendingTarget != "" && !op.CancelRequested && model.WorkspaceOperationHolds(op) && residents[slot.Target] == "" {
			slot.Target, slot.PendingTarget = slot.PendingTarget, ""
		}
		status := residents[slot.Target]
		if status == "" && model.WorkspaceOperationHolds(op) {
			status = "reserved"
		}
		if status == "" {
			delete(st.WorkspaceSlots, number)
			continue
		}
		if op.CancelRequested || !model.WorkspaceOperationHolds(op) {
			slot.PendingTarget = ""
		}
		if !model.WorkspaceOperationHolds(op) {
			slot.OperationID = ""
		}
		slot.Status, slot.ObservationID, slot.UpdatedAt = status, r.ID, r.ReceivedAt
		st.WorkspaceSlots[number], occupied[slot.Target] = slot, true
	}
	targets := []string{}
	for target := range residents {
		if !occupied[target] {
			targets = append(targets, target)
		}
	}
	slices.Sort(targets)
	for _, target := range targets {
		number := 1
		for st.WorkspaceSlots[number].Number != 0 {
			number++
		}
		st.WorkspaceSlots[number] = model.WorkspaceSlot{Number: number, Target: target, Status: residents[target], ObservationID: r.ID, UpdatedAt: r.ReceivedAt}
	}
	st.WorkspaceResidency = &r
	return nil
}

func operationScopeCurrent(st model.State, target string, revision int64, manifest, context, inputs, snapshot string) bool {
	return st.Tasks[target].Revision == revision && st.WorkspaceHeads[target] == manifest && st.WorkspaceManifests[manifest].TaskRevision == revision && model.ActionContextDigest(st, target) == context && model.SnapshotInputDigest(st, target) == inputs && (st.SnapshotHeads[target].ID == snapshot || st.SnapshotPins[snapshot].Target == target)
}
func operationTargets(i model.WorkspaceOperationIntent) []string {
	targets := []string{i.Target}
	if i.Swap != nil {
		targets = append(targets, i.Swap.Target)
	}
	return targets
}
func applyWorkspaceOperation(st *model.State, e Event) error {
	if e.Verb == "operation_queued" {
		var q model.WorkspaceOperationQueued
		if err := model.StrictJSON(e.Payload, &q); err != nil {
			return err
		}
		i, r := q.Intent, st.WorkspaceResidency
		if i.Validate() != nil || i.ID != e.EntityID || e.Actor != "cli" || e.CommandID != "workspace-operation-"+i.ID || !i.At.Equal(e.TS) || st.WorkspaceOperations[i.ID].Revision != 0 || r == nil || r.SourceID != i.SourceID || r.SourceEpoch != i.SourceEpoch || !r.ReceivedAt.Equal(i.At) || r.AfterEventID != e.ID-3 || !operationScopeCurrent(*st, i.Target, i.TaskRevision, i.ManifestID, i.ContextDigest, i.InputDigest, i.SnapshotID) {
			return fmt.Errorf("workspace operation review or census changed: %w", ErrConflict)
		}
		if s := i.Swap; s != nil && !operationScopeCurrent(*st, s.Target, s.TaskRevision, s.ManifestID, s.ContextDigest, s.InputDigest, s.SnapshotID) {
			return fmt.Errorf("swap review changed: %w", ErrConflict)
		}
		for _, old := range st.WorkspaceOperations {
			if !model.WorkspaceOperationHolds(old) {
				continue
			}
			for _, target := range operationTargets(i) {
				if model.Contains(operationTargets(old.Intent), target) {
					return fmt.Errorf("workspace has an unresolved operation: %w", ErrConflict)
				}
			}
		}
		for _, id := range i.SurfaceIDs {
			found := false
			for _, surface := range st.WorkspaceManifests[i.ManifestID].Surfaces {
				if surface.ID == id {
					found = true
				}
			}
			if !found {
				return fmt.Errorf("selected surface outside reviewed manifest")
			}
		}
		if q.ActionIDs == nil || q.Unsupported == nil || len(q.ActionIDs) > 64 || len(q.Unsupported) > 64 {
			return fmt.Errorf("bounded explicit operation items required")
		}
		seen := map[string]bool{}
		for _, id := range q.ActionIDs {
			if !model.OpaqueID.MatchString(id) || seen[id] || st.Actions[id].Revision != 0 {
				return fmt.Errorf("invalid operation action ID")
			}
			seen[id] = true
		}
		for _, issue := range q.Unsupported {
			if !model.Contains(operationTargets(i), issue.Target) || st.WorkspaceSurfaces[issue.SurfaceID].Target != issue.Target || issue.Reason == "" || len(issue.Reason) > 512 {
				return fmt.Errorf("invalid scoped operation issue")
			}
		}
		number, swapNumber := 0, 0
		for n, slot := range st.WorkspaceSlots {
			if slot.Target == i.Target {
				number = n
			}
			if i.Swap != nil && slot.Target == i.Swap.Target {
				swapNumber = n
			}
		}
		if i.Swap != nil {
			if number != 0 || swapNumber == 0 {
				return fmt.Errorf("swap requires a different resident and a nonresident incoming task")
			}
			number = swapNumber
			slot := st.WorkspaceSlots[number]
			slot.PendingTarget, slot.OperationID = i.Target, i.ID
			st.WorkspaceSlots[number] = slot
		} else if i.Kind == "open" && number == 0 {
			if len(st.WorkspaceSlots) >= model.ResidentWorkspaceLimit {
				residents := []string{}
				for _, slot := range st.WorkspaceSlots {
					residents = append(residents, slot.Target+" ("+slot.Status+")")
				}
				slices.Sort(residents)
				if len(residents) > 20 {
					residents = append(residents[:20], "additional residents omitted")
				}
				return fmt.Errorf("resident capacity %d reached: %s; review an explicit swap: %w", model.ResidentWorkspaceLimit, strings.Join(residents, ", "), ErrConflict)
			}
			number = 1
			for st.WorkspaceSlots[number].Number != 0 {
				number++
			}
			st.WorkspaceSlots[number] = model.WorkspaceSlot{Number: number, Target: i.Target, OperationID: i.ID, Status: "reserved", ObservationID: r.ID, UpdatedAt: i.At}
		} else if number != 0 {
			slot := st.WorkspaceSlots[number]
			slot.OperationID = i.ID
			st.WorkspaceSlots[number] = slot
		}
		st.WorkspaceOperations[i.ID] = model.WorkspaceOperation{Intent: i, Revision: 1, Status: "queued", Outcome: "pending", ActionIDs: q.ActionIDs, Unsupported: q.Unsupported, Slot: number, LastReason: "Explicit reviewed workspace operation", LastEventID: e.ID, UpdatedAt: e.TS}
		return nil
	}
	if e.Verb == "diffed" {
		var d model.WorkspaceDiff
		if err := model.StrictJSON(e.Payload, &d); err != nil {
			return err
		}
		op := st.WorkspaceOperations[d.OperationID]
		if d.Version != 1 || !model.OpaqueID.MatchString(d.ID) || d.ID != e.EntityID || e.CommandID != "workspace-operation-"+d.OperationID || e.Actor != "cli" || op.DiffID != "" || op.Revision != 1 || d.PreviewDigest != op.Intent.PreviewDigest || !d.At.Equal(e.TS) || len(d.Changes) > 128 || (op.Intent.Swap == nil && d.SwapPreviewDigest != "") || (op.Intent.Swap != nil && d.SwapPreviewDigest != op.Intent.Swap.PreviewDigest) {
			return fmt.Errorf("invalid operation diff reference")
		}
		for _, change := range d.Changes {
			if !model.Contains(operationTargets(op.Intent), change.Target) || st.WorkspaceSurfaces[change.SurfaceID].Target != change.Target || len(change.Reason) > 512 {
				return fmt.Errorf("foreign diff row")
			}
		}
		op.DiffID = d.ID
		st.WorkspaceOperations[d.OperationID] = op
		return nil
	}
	var v model.WorkspaceOperationChange
	if err := model.StrictJSON(e.Payload, &v); err != nil {
		return err
	}
	op := st.WorkspaceOperations[v.OperationID]
	if v.Version != 1 || !model.OpaqueID.MatchString(v.ID) || v.OperationID != e.EntityID || op.Revision != v.PreviousRevision || op.Revision < 1 || !v.At.Equal(e.TS) || v.Reason == "" || len(v.Reason) > 512 {
		return fmt.Errorf("invalid workspace operation transition")
	}
	switch e.Verb {
	case "operation_cancelled":
		if e.Actor != "cli" || !model.WorkspaceOperationHolds(op) || op.CancelRequested {
			return fmt.Errorf("workspace operation cannot be cancelled")
		}
		op.CancelRequested = true
	case "operation_reconciled":
		if e.Actor != "cli" {
			return fmt.Errorf("explicit workspace reconciliation required")
		}
		for id, other := range st.WorkspaceOperations {
			if id == op.Intent.ID || !model.WorkspaceOperationHolds(other) {
				continue
			}
			for _, target := range operationTargets(op.Intent) {
				if model.Contains(operationTargets(other.Intent), target) {
					return fmt.Errorf("workspace has another unresolved operation: %w", ErrConflict)
				}
			}
		}
		op.Status, op.Outcome = "uncertain", "pending"
	case "operation_settled", "closed":
		if e.Actor != "coordinator" || !model.WorkspaceOperationHolds(op) || op.DiffID == "" || st.WorkspaceResidency == nil || st.WorkspaceResidency.AfterEventID < op.LastEventID || v.At.Sub(st.WorkspaceResidency.ReceivedAt) > 5*time.Second || v.At.Before(st.WorkspaceResidency.ReceivedAt) {
			return fmt.Errorf("workspace settlement requires fresh post-operation census")
		}
		status, outcome := "complete", "matched"
		if len(op.Unsupported) > 0 {
			outcome = "partial"
		}
		for _, id := range op.ActionIDs {
			a := st.Actions[id]
			if a.Revision == 0 || st.WorkspaceResidency.AfterEventID < a.LastEventID {
				return fmt.Errorf("workspace census predates action")
			}
			if a.Execution == "queued" || a.Execution == "dispatching" {
				status, outcome = "running", "pending"
				break
			}
			if model.ActionHolds(a) {
				status, outcome = "uncertain", "unknown"
			} else if a.Verification != "matched" && outcome != "unknown" {
				outcome = "partial"
			}
		}
		if op.Intent.Kind == "close" && outcome == "matched" {
			for _, slot := range st.WorkspaceSlots {
				if slot.Target == op.Intent.Target && slot.Status != "reserved" {
					outcome = "partial"
				}
			}
		}
		if op.Intent.Swap != nil && outcome == "matched" && !model.WorkspaceSwapReady(*st, op) {
			outcome = "partial"
		}
		if outcome == "matched" {
			i := op.Intent
			if st.Tasks[i.Target].Revision != i.TaskRevision || st.WorkspaceHeads[i.Target] != i.ManifestID || model.OperationInputDigest(*st, op, i.Target) != i.InputDigest || model.ActionContextDigest(*st, i.Target) != i.ContextDigest {
				outcome = "partial"
			}
			if s := i.Swap; s != nil && (st.Tasks[s.Target].Revision != s.TaskRevision || st.WorkspaceHeads[s.Target] != s.ManifestID || model.OperationInputDigest(*st, op, s.Target) != s.InputDigest || model.ActionContextDigest(*st, s.Target) != s.ContextDigest) {
				outcome = "partial"
			}
		}
		if op.CancelRequested && status == "complete" {
			outcome = "cancelled"
		}
		closed := op.Intent.Kind == "close" && status == "complete" && outcome == "matched"
		if (e.Verb == "closed") != closed {
			return fmt.Errorf("workspace.closed requires observed affected closure")
		}
		op.Status, op.Outcome = status, outcome
		if status == "complete" && op.Slot != 0 {
			slot := st.WorkspaceSlots[op.Slot]
			if slot.OperationID == op.Intent.ID {
				if slot.Status == "reserved" {
					delete(st.WorkspaceSlots, op.Slot)
				} else {
					slot.OperationID, slot.PendingTarget = "", ""
					st.WorkspaceSlots[op.Slot] = slot
				}
			}
		}
	default:
		return fmt.Errorf("unknown workspace operation event")
	}
	op.Revision++
	op.LastEventID, op.UpdatedAt, op.LastReason = e.ID, e.TS, v.Reason
	st.WorkspaceOperations[v.OperationID] = op
	return nil
}
