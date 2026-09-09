package workspace

import (
	"heimdall/internal/model"
	"heimdall/internal/store"
	"math"
)

// The supported placement contract uses named workspace membership and the
// full logical display rectangle. Exact tiled split ratios are not a promise.
func recoveryPlacement(point store.PointView, id string, request RecoveryRequest, observed *Placement, live model.DesktopSnapshot) (RecoveryCheck, RecoveryCheck, RecoveryCheck, *Placement, bool) {
	membership, position, state := unknown("Saved workspace unavailable"), unknown("Saved display geometry unavailable"), unknown("Saved window state unavailable")
	if point.Payload == nil || observed == nil {
		return membership, position, state, nil, false
	}
	var desired *Placement
	for _, s := range point.Payload.Surfaces {
		if s.SurfaceID == id {
			desired = placement(s.Window, point.Payload.Monitors, point.Payload.Workspaces)
		}
	}
	if desired == nil {
		return membership, position, state, nil, false
	}
	want := *desired
	ws, wsOK := workspaceNamed(live.Workspaces, want.Workspace)
	if wsOK && observed.Workspace == want.Workspace {
		membership = matched("Owned view is in the saved uniquely named workspace")
	} else {
		membership = mismatch("Saved workspace is missing, ambiguous or the view is in another workspace")
	}
	oldMonitor, oldOK := monitorNamed(point.Payload.Monitors, want.Monitor)
	monitor, monitorOK := monitorNamed(live.Monitors, want.Monitor)
	changed := !oldOK || !monitorOK || monitorShape(oldMonitor) != monitorShape(monitor)
	fallback := false
	if changed && request.PlacementPolicy == "named-monitor-clamp" {
		fallbackMonitor, ok := monitorNamed(live.Monitors, request.FallbackMonitor)
		if !ok || !oldOK || !want.Floating || want.Fullscreen != 0 || want.Hidden || want.Pinned {
			return membership, unknown("Selected fallback requires a unique monitor and ordinary saved floating geometry"), state, &want, false
		}
		width, height, ok := logicalDisplaySize(fallbackMonitor)
		if !ok || want.Size[0] < 1 || want.Size[1] < 1 {
			return membership, unknown("Selected fallback geometry is unusable"), state, &want, false
		}
		want.At[0] = fallbackMonitor.X + want.At[0] - oldMonitor.X
		want.At[1] = fallbackMonitor.Y + want.At[1] - oldMonitor.Y
		want.Size[0], want.Size[1] = min(want.Size[0], width), min(want.Size[1], height)
		want.At[0] = max(fallbackMonitor.X, min(want.At[0], fallbackMonitor.X+width-want.Size[0]))
		want.At[1] = max(fallbackMonitor.Y, min(want.At[1], fallbackMonitor.Y+height-want.Size[1]))
		want.Monitor, monitor, monitorOK, fallback = fallbackMonitor.Name, fallbackMonitor, true, true
	} else if changed {
		return membership, unknown("Saved monitor missing, ambiguous, rescaled or transformed; explicit fallback review required"), state, &want, false
	}
	if observed.Floating == want.Floating && observed.Pinned == want.Pinned && observed.Fullscreen == want.Fullscreen && !observed.Hidden && !want.Hidden {
		state = matched("Supported floating, pinned, hidden and fullscreen window flags match")
	} else {
		state = mismatch("Supported window state differs or the view is hidden")
	}
	if !monitorOK || !wsOK || ws.MonitorName != want.Monitor || observed.Monitor != want.Monitor || !placementFits(observed, monitor) || !placementFits(&want, monitor) {
		position = mismatch("View or expected geometry is outside the selected display, or workspace/display membership differs")
	} else if want.Floating && (want.At != observed.At || want.Size != observed.Size) {
		position = mismatch("Floating geometry does not match the reviewed expectation")
	} else if fallback {
		position = recoveryCheck("degraded", "Observed geometry matches the explicitly selected named-monitor-clamp fallback; no move was dispatched")
	} else {
		position = matched("Usable logical display bounds match; tiled split ratios are not asserted")
	}
	return membership, position, state, &want, fallback
}

func logicalDisplaySize(m model.DesktopMonitor) (int, int, bool) {
	if m.Scale <= 0 || math.IsNaN(m.Scale) || math.IsInf(m.Scale, 0) || m.Width <= 0 || m.Height <= 0 {
		return 0, 0, false
	}
	w, h := m.Width, m.Height
	if m.Transform%2 != 0 {
		w, h = h, w
	}
	w, h = int(math.Floor(float64(w)/m.Scale)), int(math.Floor(float64(h)/m.Scale))
	return w, h, w > 0 && h > 0
}
