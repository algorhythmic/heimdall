package workspace

import (
	"heimdall/internal/adapters/hyprland"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"reflect"
	"slices"
	"time"
)

func placement(w *model.DesktopWindow, monitors []model.DesktopMonitor, workspaces []model.DesktopWorkspace) *Placement {
	if w == nil {
		return nil
	}
	p := &Placement{At: w.At, Size: w.Size, Floating: w.Floating, Pinned: w.Pinned, Hidden: w.Hidden, Fullscreen: w.Fullscreen}
	for _, m := range monitors {
		if m.ID == w.MonitorID {
			p.Monitor = m.Name
		}
	}
	for _, ws := range workspaces {
		if ws.ID == w.WorkspaceID {
			p.Workspace = ws.Name
		}
	}
	return p
}

func monitorNamed(monitors []model.DesktopMonitor, name string) (model.DesktopMonitor, bool) {
	var found model.DesktopMonitor
	count := 0
	for _, m := range monitors {
		if m.Name == name {
			found = m
			count++
		}
	}
	return found, count == 1 && name != ""
}
func workspaceNamed(workspaces []model.DesktopWorkspace, name string) (model.DesktopWorkspace, bool) {
	var found model.DesktopWorkspace
	count := 0
	for _, ws := range workspaces {
		if ws.Name == name {
			found = ws
			count++
		}
	}
	return found, count == 1 && name != ""
}
func monitorShape(m model.DesktopMonitor) model.DesktopMonitor {
	m.ID, m.ActiveWorkspace, m.SpecialWorkspace = 0, 0, 0
	return m
}

// planPreview is deterministic and has no I/O. Titles, PIDs, executable classes
// and foreign task identities never appear in the diff or confer ownership.
func planPreview(st model.State, r PreviewRequest, point store.PointView, observed hyprland.Status, sessions map[string]SessionCheck, now time.Time) Preview {
	v := Preview{Version: 1, Request: r, TaskRevision: st.Tasks[r.Target].Revision, InputDigest: model.SnapshotInputDigest(st, r.Target), SourceID: st.DesktopSourceHead, Fresh: observed.Fresh && observed.Snapshot != nil, Surfaces: []PreviewSurface{}, Displays: []model.DesktopMonitor{}, Issues: []string{}, AsOf: now.UTC(), ExpiresAt: now.UTC().Add(30 * time.Second)}
	m := st.WorkspaceManifests[r.ManifestID]
	if m.ID == "" {
		v.Issues = append(v.Issues, "manifest_missing")
	}
	if m.ID != "" && m.TaskRevision != v.TaskRevision {
		v.Issues = append(v.Issues, "manifest_task_changed")
	}
	payload := model.WorkspacePointPayload{}
	if point.Point.ID != "" {
		v.SnapshotPayloadDigest = point.Point.PayloadDigest
		v.SnapshotObservedAt = point.Point.ObservedAt
		v.SnapshotAgeSeconds = int64(now.Sub(point.Point.ObservedAt).Seconds())
		v.SnapshotProtected = model.SnapshotProtected(st, point.Point.ID)
		if !v.SnapshotProtected {
			v.Issues = append(v.Issues, "snapshot_not_pinned_or_current")
		}
		if v.SnapshotAgeSeconds < 0 {
			v.Issues = append(v.Issues, "snapshot_clock_ahead")
		}
		if r.MaxSnapshotAgeSeconds > 0 && v.SnapshotAgeSeconds > int64(r.MaxSnapshotAgeSeconds) {
			v.Issues = append(v.Issues, "snapshot_age_limit_exceeded")
		}
		if point.Pruned || point.Payload == nil {
			v.Issues = append(v.Issues, "snapshot_payload_unavailable")
		} else {
			payload = *point.Payload
			if payload.Coverage != "complete" {
				v.Issues = append(v.Issues, "saved_coverage_partial")
			}
		}
	} else {
		v.Issues = append(v.Issues, "snapshot_missing")
	}
	live := model.DesktopSnapshot{}
	if v.Fresh {
		live = *observed.Snapshot
		v.SourceEpoch = live.SourceEpoch
		v.Boundary = model.SnapshotBoundary{Method: observed.Coverage, StartedAt: live.StartedAt, FinishedAt: live.CapturedAt, KnownEventGaps: observed.Gaps}
		if !st.DesktopSources[v.SourceID].Active || st.DesktopSources[v.SourceID].Epoch != live.SourceEpoch {
			v.Fresh = false
			v.Issues = append(v.Issues, "source_selection_changed")
		}
	} else {
		v.Issues = append(v.Issues, "source_unavailable", observed.Issue)
	}
	if payload.SourceEpoch != "" && v.Fresh && payload.SourceEpoch != live.SourceEpoch {
		v.Issues = append(v.Issues, "saved_source_epoch_changed")
	}
	owners := map[model.WindowIdentity]model.ViewportBinding{}
	for _, id := range st.ViewportHeads {
		b := st.ViewportBindings[id]
		if b.Active && b.Window != nil {
			owners[*b.Window] = b
		}
	}
	saved := map[string]model.SnapshotSurface{}
	for _, row := range payload.Surfaces {
		saved[row.SurfaceID] = row
	}
	desired := map[string]bool{}
	relevantWorkspaces := map[int]bool{}
	for _, ws := range payload.Workspaces {
		if current, ok := workspaceNamed(live.Workspaces, ws.Name); ok {
			relevantWorkspaces[current.ID] = true
		}
	}
	for _, surface := range m.Surfaces {
		desired[surface.ID] = true
		row := PreviewSurface{SurfaceID: surface.ID, Kind: surface.Kind, Label: surface.Label, Required: surface.Required, Membership: "desired", ViewportBindingID: st.ViewportHeads[surface.ID], SessionBindingID: st.SessionHeads[surface.ID], SessionStatus: "unbound", Disposition: "leave-open", Changes: []string{}, Issues: []string{}, Requirements: []string{}}
		old, had := saved[surface.ID]
		row.ObservationStatus = "unowned"
		row.Desired = placement(old.Window, payload.Monitors, payload.Workspaces)
		if !had {
			row.Changes = append(row.Changes, "added_to_manifest")
		}
		if c, ok := sessions[surface.ID]; ok {
			row.SessionStatus = c.Status
			row.Issues = append(row.Issues, c.Issues...)
		}
		b := st.ViewportBindings[row.ViewportBindingID]
		if b.Active && b.Window != nil && b.Target == r.Target {
			row.ObservationStatus = "unavailable"
			if v.Fresh && b.Window.SourceEpoch == live.SourceEpoch {
				row.ObservationStatus = "missing"
			}
			if b.TaskRevision != v.TaskRevision {
				row.Issues = append(row.Issues, "binding_task_changed")
			}
			if b.ManifestID != r.ManifestID {
				row.Issues = append(row.Issues, "binding_manifest_changed")
			}
			if b.SourceID != v.SourceID {
				row.Issues = append(row.Issues, "binding_source_changed")
			}
			if b.SessionBindingID != "" && b.SessionBindingID != row.SessionBindingID {
				row.Issues = append(row.Issues, "session_binding_changed")
			}
			if old.ViewportBindingID != "" && old.ViewportBindingID != b.ID {
				row.Changes = append(row.Changes, "viewport_rebound_since_snapshot")
			}
			if v.Fresh {
				for _, w := range live.Windows {
					if w.Identity == *b.Window {
						id := w.Identity
						row.Window = &id
						row.ObservationStatus = "observed"
						row.Observed = placement(&w, live.Monitors, live.Workspaces)
						relevantWorkspaces[w.WorkspaceID] = true
					}
				}
			}
		}
		foreignOld := old.Window != nil && owners[old.Window.Identity].Active && owners[old.Window.Identity].Target != r.Target
		if foreignOld {
			row.Issues = append(row.Issues, "saved_window_owned_elsewhere_left_open")
		}
		switch {
		case !v.Fresh:
			row.Disposition = "unavailable"
		case surface.Kind == "browser":
			row.Disposition = "review-required"
			row.Issues = append(row.Issues, "browser_window_pairing_required")
		case row.Desired == nil:
			row.Disposition = "review-required"
			row.Issues = append(row.Issues, "saved_layout_missing")
		case !b.Active || b.Window == nil:
			row.Disposition = "review-required"
			row.Issues = append(row.Issues, "explicit_viewport_binding_required")
		case b.TaskRevision != v.TaskRevision || b.ManifestID != r.ManifestID:
			row.Disposition = "review-required"
		case row.Window == nil:
			row.Changes = append(row.Changes, "owned_window_missing")
			if row.SessionStatus == "current" {
				row.Disposition = "reattach"
				row.Requirements = append(row.Requirements, "reviewed_attach_recipe", "unique_new_window_association", "attachment_verification")
			} else if b.Window.SourceEpoch == live.SourceEpoch {
				row.Disposition = "launch"
				row.Requirements = append(row.Requirements, "reviewed_executable_argv_cwd", "unique_new_window_association", "application_state_verification")
			} else {
				row.Disposition = "review-required"
				row.Issues = append(row.Issues, "window_identity_from_previous_source")
			}
		default:
			comparePlacement(&row)
			if b.SourceID != v.SourceID {
				row.Disposition = "review-required"
			}
		}
		if row.Desired != nil && v.Fresh {
			oldMonitor, oldOK := monitorNamed(payload.Monitors, row.Desired.Monitor)
			newMonitor, newOK := monitorNamed(live.Monitors, row.Desired.Monitor)
			ws, wsOK := workspaceNamed(live.Workspaces, row.Desired.Workspace)
			if !oldOK || !newOK {
				row.Issues = append(row.Issues, "saved_display_missing_or_ambiguous")
				row.Disposition = "review-required"
			} else if monitorShape(oldMonitor) != monitorShape(newMonitor) {
				row.Issues = append(row.Issues, "display_geometry_changed")
				row.Disposition = "review-required"
			}
			if !wsOK {
				row.Issues = append(row.Issues, "saved_workspace_missing_or_ambiguous")
				row.Disposition = "review-required"
			} else if ws.MonitorName != row.Desired.Monitor {
				row.Issues = append(row.Issues, "workspace_display_changed")
				row.Disposition = "review-required"
			}
			if row.Desired.Hidden || row.Desired.Size[0] <= 0 || row.Desired.Size[1] <= 0 {
				row.Issues = append(row.Issues, "saved_placement_not_usable")
				row.Disposition = "review-required"
			}
			if oldOK && !placementFits(row.Desired, oldMonitor) {
				row.Issues = append(row.Issues, "saved_placement_outside_display")
				row.Disposition = "review-required"
			}
		}
		if surface.Kind == "terminal" && row.Window != nil {
			row.Requirements = append(row.Requirements, "pane_attachment_not_verified")
		}
		if model.Contains(v.Issues, "manifest_task_changed") || model.Contains(v.Issues, "snapshot_not_pinned_or_current") || model.Contains(v.Issues, "snapshot_age_limit_exceeded") || model.Contains(v.Issues, "snapshot_clock_ahead") {
			row.Disposition = "review-required"
		}
		if row.Disposition == "move" {
			row.Requirements = append(row.Requirements, "fresh_ownership_and_placement_verification")
		}
		v.Surfaces = append(v.Surfaces, row)
	}
	for id := range saved {
		if desired[id] {
			continue
		}
		row := PreviewSurface{SurfaceID: id, Kind: st.WorkspaceSurfaces[id].Kind, Membership: "removed", Disposition: "leave-open", SessionStatus: "not_checked", Changes: []string{"removed_from_manifest"}, Issues: []string{}, Requirements: []string{}}
		if w := saved[id].Window; w != nil && owners[w.Identity].Active && owners[w.Identity].Target != r.Target {
			row.Issues = append(row.Issues, "saved_window_owned_elsewhere_left_open")
		}
		v.Surfaces = append(v.Surfaces, row)
	}
	if v.Fresh {
		for _, w := range live.Windows {
			if !owners[w.Identity].Active && relevantWorkspaces[w.WorkspaceID] {
				v.UnownedLeftOpen++
			}
		}
	}
	displays := map[string]bool{}
	for _, row := range v.Surfaces {
		if row.Desired != nil {
			displays[row.Desired.Monitor] = true
		}
		if row.Observed != nil {
			displays[row.Observed.Monitor] = true
		}
	}
	for _, m := range live.Monitors {
		if displays[m.Name] {
			v.Displays = append(v.Displays, monitorShape(m))
		}
	}
	slices.SortFunc(v.Displays, func(a, b model.DesktopMonitor) int {
		if a.Name < b.Name {
			return -1
		}
		if a.Name > b.Name {
			return 1
		}
		return 0
	})
	slices.SortFunc(v.Surfaces, func(a, b PreviewSurface) int {
		if a.SurfaceID < b.SurfaceID {
			return -1
		}
		if a.SurfaceID > b.SurfaceID {
			return 1
		}
		return 0
	})
	v.ReviewRequired = len(v.Issues) != 0
	for _, row := range v.Surfaces {
		if row.Disposition == "review-required" || row.Disposition == "unavailable" {
			v.ReviewRequired = true
		}
	}
	return v
}

// This only checks the full logical display rectangle. Reserved work areas,
// decoration extents and application usability require later verification.
func placementFits(p *Placement, m model.DesktopMonitor) bool {
	w, h := float64(m.Width), float64(m.Height)
	if m.Transform%2 != 0 {
		w, h = h, w
	}
	if m.Scale <= 0 {
		return false
	}
	x, y := float64(p.At[0]-m.X), float64(p.At[1]-m.Y)
	return x >= 0 && y >= 0 && p.Size[0] > 0 && p.Size[1] > 0 && x+float64(p.Size[0]) <= w/m.Scale && y+float64(p.Size[1]) <= h/m.Scale
}

func comparePlacement(row *PreviewSurface) {
	a, b := row.Observed, row.Desired
	if a == nil || b == nil {
		return
	}
	if a.Workspace != b.Workspace {
		row.Changes = append(row.Changes, "workspace_changed")
	}
	if a.Monitor != b.Monitor {
		row.Changes = append(row.Changes, "monitor_changed")
	}
	if a.At != b.At || a.Size != b.Size {
		row.Changes = append(row.Changes, "geometry_changed")
	}
	if a.Floating != b.Floating || a.Pinned != b.Pinned || a.Hidden != b.Hidden || a.Fullscreen != b.Fullscreen {
		row.Changes = append(row.Changes, "window_state_changed")
	}
	if !reflect.DeepEqual(a, b) {
		row.Disposition = "move"
	}
	if (!a.Floating || !b.Floating) && (a.At != b.At || a.Size != b.Size) {
		row.Disposition = "review-required"
		row.Issues = append(row.Issues, "tiled_split_geometry_not_restorable")
	}
}
