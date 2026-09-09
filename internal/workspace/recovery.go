package workspace

import (
	"context"
	"fmt"
	"heimdall/internal/adapters/application"
	"heimdall/internal/adapters/hyprland"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"reflect"
	"time"
)

// RecoveryRequest grants observation authority only. A fallback changes the
// expected placement, never dispatches a move, and is visible in the result.
type RecoveryRequest struct {
	Version         int    `json:"version"`
	Target          string `json:"target"`
	OperationID     string `json:"operation_id,omitempty"`
	SnapshotID      string `json:"snapshot_id,omitempty"`
	PlacementPolicy string `json:"placement_policy"`
	FallbackMonitor string `json:"fallback_monitor,omitempty"`
}

func (r RecoveryRequest) Validate() error {
	if r.Version != 1 || !model.ValidID(r.Target) || (r.OperationID != "" && !model.OpaqueID.MatchString(r.OperationID)) || (r.SnapshotID != "" && !model.OpaqueID.MatchString(r.SnapshotID)) || (r.OperationID != "" && r.SnapshotID != "") {
		return fmt.Errorf("explicit task and at most one operation or snapshot required")
	}
	if r.PlacementPolicy != "saved" && r.PlacementPolicy != "named-monitor-clamp" {
		return fmt.Errorf("placement policy must be saved or named-monitor-clamp")
	}
	if (r.PlacementPolicy == "named-monitor-clamp") != (r.FallbackMonitor != "") || len(r.FallbackMonitor) > 256 {
		return fmt.Errorf("named-monitor-clamp requires an explicit monitor")
	}
	for _, c := range r.FallbackMonitor {
		if c < 32 || c == 127 {
			return fmt.Errorf("invalid fallback monitor")
		}
	}
	return nil
}

type RecoveryCheck struct {
	Status string `json:"status"`
	Reason string `json:"reason"`
}
type RecoverySurface struct {
	Evidence            RecoveryEvidence      `json:"evidence"`
	SurfaceID           string                `json:"surface_id"`
	Kind                string                `json:"kind"`
	Label               string                `json:"label"`
	Required            bool                  `json:"required"`
	Expected            string                `json:"expected"`
	Outcome             string                `json:"outcome"`
	Full                bool                  `json:"full"`
	BindingID           string                `json:"binding_id"`
	Window              *model.WindowIdentity `json:"window,omitempty"`
	Observed            *Placement            `json:"observed,omitempty"`
	ExpectedPlacement   *Placement            `json:"expected_placement,omitempty"`
	FallbackApplied     bool                  `json:"fallback_applied"`
	Existence           RecoveryCheck         `json:"existence"`
	Ownership           RecoveryCheck         `json:"ownership"`
	TaskMembership      RecoveryCheck         `json:"task_membership"`
	WorkspaceMembership RecoveryCheck         `json:"workspace_membership"`
	Placement           RecoveryCheck         `json:"placement"`
	WindowState         RecoveryCheck         `json:"window_state"`
	Application         RecoveryCheck         `json:"application"`
	Action              RecoveryCheck         `json:"action"`
}

type RecoveryEvidence struct {
	ApplicationActionID string    `json:"application_action_id,omitempty"`
	SessionBindingID    string    `json:"session_binding_id,omitempty"`
	SessionCheckedAt    time.Time `json:"session_checked_at,omitempty"`
	BrowserProfile      string    `json:"browser_profile,omitempty"`
	BrowserEpoch        string    `json:"browser_epoch,omitempty"`
	BrowserChallengeID  string    `json:"browser_challenge_id,omitempty"`
	BrowserSequence     int64     `json:"browser_sequence,omitempty"`
	BrowserReceivedAt   time.Time `json:"browser_received_at,omitempty"`
}
type RecoveryReport struct {
	ObservationID string                 `json:"observation_id"`
	Version       int                    `json:"version"`
	Request       RecoveryRequest        `json:"request"`
	ManifestID    string                 `json:"manifest_id"`
	TaskRevision  int64                  `json:"task_revision"`
	SnapshotID    string                 `json:"snapshot_id"`
	InputDigest   string                 `json:"input_digest"`
	SourceID      string                 `json:"source_id"`
	SourceEpoch   string                 `json:"source_epoch"`
	Coverage      RecoveryCheck          `json:"coverage"`
	Operation     RecoveryCheck          `json:"operation"`
	Outcome       string                 `json:"outcome"`
	Full          bool                   `json:"full"`
	Surfaces      []RecoverySurface      `json:"surfaces"`
	Issues        []string               `json:"issues"`
	Boundary      model.SnapshotBoundary `json:"boundary"`
	AsOf          time.Time              `json:"as_of"`
	ExpiresAt     time.Time              `json:"expires_at"`
	Digest        string                 `json:"digest"`
}

func recoveryCheck(status, reason string) RecoveryCheck { return RecoveryCheck{status, reason} }
func matched(reason string) RecoveryCheck               { return recoveryCheck("matched", reason) }
func unknown(reason string) RecoveryCheck               { return recoveryCheck("unknown", reason) }
func mismatch(reason string) RecoveryCheck              { return recoveryCheck("not_matched", reason) }
func notRequired() RecoveryCheck {
	return recoveryCheck("not_required", "Not part of this postcondition")
}
func recoveryChecks(checks ...RecoveryCheck) (string, bool) {
	result := "verified"
	for _, c := range checks {
		switch c.Status {
		case "matched", "not_required":
		case "degraded":
			if result == "verified" {
				result = "degraded"
			}
		case "unknown", "unsupported":
			if result != "partial" {
				result = "unknown"
			}
		default:
			result = "partial"
		}
	}
	return result, result == "verified"
}

func recoverySelection(st model.State, r RecoveryRequest) (string, []string, string, error) {
	if st.Tasks[r.Target].Revision == 0 {
		return "", nil, "", fmt.Errorf("existing task required")
	}
	point, kind := r.SnapshotID, "open"
	ids := []string{}
	if r.OperationID != "" {
		op := st.WorkspaceOperations[r.OperationID]
		if op.Intent.Target == r.Target {
			point, kind, ids = op.Intent.SnapshotID, op.Intent.Kind, op.Intent.SurfaceIDs
			if kind == "close" && op.CloseSnapshotID != "" {
				point = op.CloseSnapshotID
			}
		} else if op.Intent.Swap != nil && op.Intent.Swap.Target == r.Target {
			point, kind = op.Intent.Swap.SnapshotID, "close"
			for _, s := range st.WorkspaceManifests[op.Intent.Swap.ManifestID].Surfaces {
				ids = append(ids, s.ID)
			}
		} else {
			return "", nil, "", fmt.Errorf("operation is not scoped to explicit task")
		}
	} else {
		if point == "" {
			point = st.SnapshotHeads[r.Target].ID
		}
		for _, s := range st.WorkspaceManifests[st.WorkspaceHeads[r.Target]].Surfaces {
			ids = append(ids, s.ID)
		}
	}
	return point, ids, kind, nil
}

// Verify reuses the selected observer and session adapter, outside the writer.
// Reports are point-in-time evidence, not durable success or dispatch tokens.
func (s *PreviewService) Verify(ctx context.Context, r RecoveryRequest, now time.Time) (RecoveryReport, error) {
	if err := r.Validate(); err != nil {
		return RecoveryReport{}, err
	}
	started := time.Now()
	initial, err := s.Store.State(ctx)
	if err != nil {
		return RecoveryReport{}, err
	}
	pointID, ids, kind, err := recoverySelection(initial, r)
	if err != nil {
		return RecoveryReport{}, err
	}
	if s.BrowserReadback != nil {
		// A timeout leaves browser checks unknown; it cannot authorize input.
		_ = s.BrowserReadback.ObserveWorkspace(ctx, r.Target, ids, now)
	}
	st, point, err := s.Store.WorkspacePreviewInputs(ctx, r.Target, pointID)
	if err != nil {
		return RecoveryReport{}, err
	}
	observed := hyprland.Status{}
	if s.Observer != nil {
		observed, _ = s.Observer.Read(ctx, true)
	}
	checks := s.sessions(ctx, st, r.Target, now)
	processes := s.recoveryProcesses(st, ids, observed)
	asOf := now.Add(time.Since(started)).UTC()
	v := planRecovery(st, point, r, ids, kind, observed, checks, processes, asOf)
	after, nextPoint, err := s.Store.WorkspacePreviewInputs(ctx, r.Target, pointID)
	if err != nil {
		return RecoveryReport{}, err
	}
	nextID, nextIDs, nextKind, err := recoverySelection(after, r)
	if err != nil || nextID != pointID || !reflect.DeepEqual(ids, nextIDs) || kind != nextKind || !reflect.DeepEqual(v, planRecovery(after, nextPoint, r, ids, kind, observed, checks, processes, asOf)) {
		return RecoveryReport{}, fmt.Errorf("recovery inputs changed during observation: %w", store.ErrConflict)
	}
	for _, row := range v.Surfaces {
		if row.Kind != "browser" {
			continue
		}
		p := after.Browsers[row.Evidence.BrowserProfile]
		if p.Freshness != nil && (s.BrowserReadback == nil || !s.BrowserReadback.RecoveryFresh(p, p.Freshness.Challenge.AfterEventID, asOf)) {
			return RecoveryReport{}, fmt.Errorf("browser recovery readback lost its runtime freshness lease: %w", store.ErrConflict)
		}
	}
	if observed.Fresh && observed.Snapshot != nil {
		if err := s.Observer.CheckCapture(observed.Snapshot.ID, observed.Snapshot.CapturedAt); err != nil {
			return RecoveryReport{}, fmt.Errorf("recovery observation expired or changed: %w", store.ErrConflict)
		}
	}
	v.Digest = model.ContentDigest(v)
	return v, nil
}

func planRecovery(st model.State, point store.PointView, request RecoveryRequest, ids []string, kind string, observed hyprland.Status, sessions map[string]SessionCheck, processes map[string]RecoveryCheck, now time.Time) RecoveryReport {
	m := st.WorkspaceManifests[st.WorkspaceHeads[request.Target]]
	v := RecoveryReport{Version: 1, Request: request, ManifestID: m.ID, TaskRevision: st.Tasks[request.Target].Revision, SnapshotID: point.Point.ID, InputDigest: model.SnapshotInputDigest(st, request.Target), SourceID: st.DesktopSourceHead, Coverage: unknown("Fresh selected compositor required"), Operation: notRequired(), Surfaces: []RecoverySurface{}, Issues: []string{}, AsOf: now, ExpiresAt: now.Add(2 * time.Second)}
	live := model.DesktopSnapshot{}
	if observed.Snapshot != nil {
		live = *observed.Snapshot
		v.ObservationID = live.ID
		v.SourceEpoch = live.SourceEpoch
		v.Boundary = model.SnapshotBoundary{Method: observed.Coverage, StartedAt: live.StartedAt, FinishedAt: live.CapturedAt, KnownEventGaps: observed.Gaps}
		v.ExpiresAt = live.CapturedAt.Add(2 * time.Second)
	}
	source := st.DesktopSources[v.SourceID]
	fresh := observed.Fresh && observed.Snapshot != nil && source.Active && source.Epoch == live.SourceEpoch && !now.Before(live.CapturedAt) && now.Sub(live.CapturedAt) < 2*time.Second
	if fresh {
		v.Coverage = matched("Fresh bounded compositor inventory; not an atomic cross-application snapshot")
	}
	if point.Payload == nil || point.Pruned || point.Payload.Coverage != "complete" || point.Point.ManifestID != m.ID || point.Point.TaskRevision != v.TaskRevision {
		v.Issues = append(v.Issues, "saved_point_missing_partial_or_scope_changed")
	}
	if m.ID == "" || m.TaskRevision != v.TaskRevision {
		v.Issues = append(v.Issues, "current_manifest_required")
	}
	if len(ids) == 0 {
		v.Issues = append(v.Issues, "empty_selection_is_not_recovery")
	}
	op := st.WorkspaceOperations[request.OperationID]
	if request.OperationID != "" {
		v.Operation = unknown("Operation has not completed with matched action postconditions")
		if op.Status == "complete" && op.Outcome == "matched" && !op.CancelRequested {
			v.Operation = matched("Journal settled; each live postcondition is checked separately")
		}
		if op.Intent.Target == request.Target && (op.Intent.ManifestID != m.ID || op.Intent.TaskRevision != v.TaskRevision) {
			v.Operation = mismatch("Operation task or manifest changed")
		}
		if op.Intent.Swap != nil && op.Intent.Swap.Target == request.Target && (op.Intent.Swap.ManifestID != m.ID || op.Intent.Swap.TaskRevision != v.TaskRevision) {
			v.Operation = mismatch("Outgoing task or manifest changed")
		}
		inputs, contextDigest := op.Intent.InputDigest, op.Intent.ContextDigest
		if op.Intent.Swap != nil && op.Intent.Swap.Target == request.Target {
			inputs, contextDigest = op.Intent.Swap.InputDigest, op.Intent.Swap.ContextDigest
		}
		if model.OperationInputDigest(st, op, request.Target) != inputs || model.ActionContextDigest(st, request.Target) != contextDigest {
			v.Operation = mismatch("Operation inputs or task context changed outside its own proven outputs")
		}
	}
	for _, id := range ids {
		row := recoverySurface(st, point, request, id, kind, op, live, fresh, sessions[id], now)
		if row.Expected == "present" && st.ViewportBindings[row.BindingID].ApplicationActionID != "" {
			c, ok := processes[row.BindingID]
			if !ok {
				c = unknown("Original launched process identity unavailable")
			}
			if c.Status != "matched" {
				row.Ownership = c
				finishRecoverySurface(&row)
			}
		}
		v.Surfaces = append(v.Surfaces, row)
		if row.Kind == "browser" {
			p := st.Browsers[row.Evidence.BrowserProfile]
			if p.Freshness != nil {
				for _, end := range []time.Time{p.ReceivedAt.Add(5 * time.Second), p.Freshness.Challenge.ExpiresAt} {
					if end.Before(v.ExpiresAt) {
						v.ExpiresAt = end
					}
				}
			}
		}
	}
	checks := []RecoveryCheck{v.Coverage, v.Operation}
	if len(v.Issues) != 0 {
		checks = append(checks, unknown("Saved inputs or selection unavailable"))
	}
	for _, row := range v.Surfaces {
		status := row.Outcome
		if status == "verified" {
			status = "matched"
		}
		if status == "partial" {
			status = "not_matched"
		}
		checks = append(checks, recoveryCheck(status, "Selected surface"))
	}
	v.Outcome, v.Full = recoveryChecks(checks...)
	return v
}

func recoverySurface(st model.State, point store.PointView, request RecoveryRequest, id, kind string, op model.WorkspaceOperation, live model.DesktopSnapshot, fresh bool, session SessionCheck, now time.Time) RecoverySurface {
	m := st.WorkspaceManifests[st.WorkspaceHeads[request.Target]]
	r := RecoverySurface{SurfaceID: id, Expected: "present", BindingID: st.ViewportHeads[id], Existence: unknown("Current owned instance is not observed"), Ownership: unknown("Current exact binding required"), TaskMembership: mismatch("Surface is not in the current task manifest"), WorkspaceMembership: unknown("Saved named workspace required"), Placement: unknown("Saved usable display placement required"), WindowState: unknown("Current window state required"), Application: unknown("Supported application state readback required"), Action: notRequired()}
	if kind == "close" {
		r.Expected = "absent"
	}
	for _, s := range m.Surfaces {
		if s.ID == id {
			r.Kind, r.Label, r.Required = s.Kind, s.Label, s.Required
			r.TaskMembership = matched("Logical surface belongs to the current task manifest")
		}
	}
	b := st.ViewportBindings[r.BindingID]
	if b.Target == request.Target {
		r.Evidence.ApplicationActionID = b.ApplicationActionID
		r.Evidence.SessionBindingID = b.SessionBindingID
		if session.BindingID == b.SessionBindingID && session.Target == request.Target {
			r.Evidence.SessionCheckedAt = session.CheckedAt
		}
	}
	current := b.Active && b.Window != nil && b.Target == request.Target && b.SurfaceID == id && b.ManifestID == m.ID && b.TaskRevision == st.Tasks[request.Target].Revision && b.SourceID == st.DesktopSourceHead && b.Window.SourceEpoch == live.SourceEpoch && (b.SessionBindingID == "" || b.SessionBindingID == st.SessionHeads[id])
	if current {
		for other, head := range st.ViewportHeads {
			owner := st.ViewportBindings[head]
			if other != id && owner.Active && owner.Window != nil && *owner.Window == *b.Window {
				current = false
			}
		}
	}
	var window *model.DesktopWindow
	if current && fresh {
		r.Ownership = matched("Exact current task-owned compositor identity")
		count := 0
		for _, w := range live.Windows {
			if w.Identity == *b.Window {
				copy := w
				window = &copy
				count++
			}
		}
		if count > 1 {
			window = nil
			r.Ownership = unknown("Ambiguous exact window identity")
		}
		if count == 0 {
			r.Existence = mismatch("Exact owned native window is absent")
		}
		if count == 1 {
			r.Window = &window.Identity
			r.Observed = placement(window, live.Monitors, live.Workspaces)
			r.Existence = matched("Exact owned native window exists")
		}
	}
	if request.OperationID != "" {
		r.Action = unknown("No matched action for the selected surface")
		for _, aid := range op.ActionIDs {
			a := st.Actions[aid]
			if a.Intent.Target == request.Target && a.Intent.SurfaceID == id {
				if a.Verification == "matched" && a.Execution != "queued" && a.Execution != "dispatching" {
					r.Action = matched("Original action has independently matched postconditions")
				} else {
					r.Action = unknown("Original action is unresolved, refused or not matched")
				}
			}
		}
		for _, issue := range op.Unsupported {
			if issue.Target == request.Target && issue.SurfaceID == id {
				r.Action = recoveryCheck("unsupported", "Selected operation cannot handle this surface")
			}
		}
	}
	// Other unfinished operations may still alter this surface after observation.
	for _, a := range st.Actions {
		if a.Intent.Target == request.Target && a.Intent.SurfaceID == id && model.ActionHolds(a) {
			r.Action = unknown("An unfinished action may still change this surface")
		}
	}
	if kind == "close" {
		r.WorkspaceMembership, r.Placement, r.WindowState, r.Application = notRequired(), notRequired(), notRequired(), notRequired()
		if current && fresh && r.Ownership.Status == "matched" {
			if window == nil {
				r.Existence = matched("Exact owned native window is absent")
			} else {
				r.Existence = mismatch("Owned native window remains open")
			}
		}
		if b.SessionBindingID != "" {
			r.Application = recoverySession(session)
		}
	} else if window != nil && r.Ownership.Status == "matched" {
		r.WorkspaceMembership, r.Placement, r.WindowState, r.ExpectedPlacement, r.FallbackApplied = recoveryPlacement(point, id, request, r.Observed, live)
		switch r.Kind {
		case "native":
			r.Application = matched("Native window state only; no application session restoration requested")
		case "terminal":
			if b.SessionBindingID != "" {
				r.Application = recoverySession(session)
				if r.Application.Status == "matched" {
					r.Application = unknown("Bound Herdr process survives; attachment of this view is not observable")
				}
			} else {
				r.Application = recoveryCheck("degraded", "Terminal view exists; prior process continuity is not restored or verified")
			}
		case "editor":
			r.Application = recoveryCheck("unsupported", "Saved-file launch is not editor buffer, cursor or unsaved-state readback")
		}
	}
	if r.Kind == "browser" {
		recoveryBrowser(st, b, request, kind, op, current && fresh, window != nil, now, &r)
	}
	if recipe := st.ApplicationHeads[id]; recipe != "" && !model.ApplicationRecipeCurrent(st, recipe, request.Target, id) {
		r.Application = unknown("Application recipe or bound session scope changed")
	}
	finishRecoverySurface(&r)
	return r
}

func finishRecoverySurface(r *RecoverySurface) {
	r.Outcome, r.Full = recoveryChecks(r.Existence, r.Ownership, r.TaskMembership, r.WorkspaceMembership, r.Placement, r.WindowState, r.Application, r.Action)
}

func (s *PreviewService) recoveryProcesses(st model.State, ids []string, observed hyprland.Status) map[string]RecoveryCheck {
	checks := map[string]RecoveryCheck{}
	read := s.Process
	if read == nil {
		read = (application.Adapter{}).Process
	}
	for _, id := range ids {
		b := st.ViewportBindings[st.ViewportHeads[id]]
		if b.ApplicationActionID == "" {
			continue
		}
		checks[b.ID] = unknown("Exact original launched process and window are not observed")
		a := st.Actions[b.ApplicationActionID]
		if a.Report == nil || a.Report.Native == nil || a.Report.Native.Process == nil || observed.Snapshot == nil || !observed.Fresh || b.Window == nil {
			continue
		}
		pin := *a.Report.Native.Process
		p, err := read(pin.PID)
		if err != nil || p != pin {
			continue
		}
		count, exact := 0, false
		for _, w := range observed.Snapshot.Windows {
			if w.Class == model.ApplicationClass(a.Intent.AttemptID) {
				count++
				exact = w.PID == pin.PID && w.Identity == *b.Window
			}
		}
		if count == 1 && exact {
			checks[b.ID] = matched("Original PID/start-time and unique attempt window identity match")
		}
	}
	return checks
}

func recoverySession(c SessionCheck) RecoveryCheck {
	if c.Status == "current" {
		return matched("Original bound session and pane process independently observed")
	}
	if c.Status == "unavailable" || c.Status == "" {
		return unknown("Bound session observation unavailable")
	}
	return mismatch("Bound session or pane process changed")
}
