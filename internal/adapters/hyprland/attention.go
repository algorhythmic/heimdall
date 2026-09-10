package hyprland

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"time"

	"heimdall/internal/model"
)

var ErrAttentionExcluded = errors.New("attention observation excluded")

type AttentionObservation struct {
	Surface   *model.CompositorSurfaceFocusSpan
	Workspace *model.CompositorWorkspaceFocusSpan
}

func (o AttentionObservation) EndedAt() time.Time {
	if o.Surface != nil {
		return o.Surface.EndedAt
	}
	if o.Workspace != nil {
		return o.Workspace.EndedAt
	}
	return time.Time{}
}
func (o AttentionObservation) SourceSequence() int64 {
	if o.Surface != nil {
		return o.Surface.Sequence
	}
	if o.Workspace != nil {
		return o.Workspace.Sequence
	}
	return 0
}

type openSurfaceFocus struct {
	window    model.DesktopWindow
	startedAt time.Time
}
type openWorkspaceFocus struct {
	workspace model.DesktopWorkspace
	startedAt time.Time
}

func (o *Observer) resetAttentionLocked() { o.surfaceFocus = nil; o.workspaceFocus = nil }
func (o *Observer) attentionFrameLocked(kind, payload string, sequence int64, at time.Time) *AttentionObservation {
	if o.status.Snapshot == nil || !o.source.Active {
		o.resetAttentionLocked()
		return nil
	}
	snapshot := o.status.Snapshot
	var out *AttentionObservation
	switch kind {
	case "activewindowv2":
		id := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(payload), "0x"))
		var next *model.DesktopWindow
		for i := range snapshot.Windows {
			if snapshot.Windows[i].Identity.StableID == id {
				copy := snapshot.Windows[i]
				next = &copy
				break
			}
		}
		if o.surfaceFocus != nil && (next == nil || next.Identity != o.surfaceFocus.window.Identity) {
			out = o.closeSurfaceLocked(snapshot, sequence, at)
		}
		if next == nil {
			o.surfaceFocus = nil
		} else if o.surfaceFocus == nil {
			o.surfaceFocus = &openSurfaceFocus{window: *next, startedAt: at}
		}
	case "workspacev2", "workspace":
		id, name := 0, strings.TrimSpace(payload)
		if kind == "workspacev2" {
			parts := strings.SplitN(name, ",", 2)
			if len(parts) != 2 {
				o.workspaceFocus = nil
				return out
			}
			id, _ = strconv.Atoi(parts[0])
			name = strings.TrimSpace(parts[1])
		}
		var next *model.DesktopWorkspace
		for i := range snapshot.Workspaces {
			w := snapshot.Workspaces[i]
			if (kind == "workspacev2" && w.ID == id && w.Name == name) || (kind == "workspace" && w.Name == name) {
				copy := w
				next = &copy
				break
			}
		}
		if o.workspaceFocus != nil && (next == nil || next.ID != o.workspaceFocus.workspace.ID || next.Name != o.workspaceFocus.workspace.Name) {
			out = o.closeWorkspaceLocked(snapshot, sequence, at)
		}
		if next == nil {
			o.workspaceFocus = nil
		} else if o.workspaceFocus == nil {
			o.workspaceFocus = &openWorkspaceFocus{workspace: *next, startedAt: at}
		}
	}
	return out
}
func (o *Observer) closeSurfaceLocked(snapshot *model.DesktopSnapshot, sequence int64, at time.Time) *AttentionObservation {
	open := o.surfaceFocus
	o.surfaceFocus = nil
	duration := at.Sub(open.startedAt).Seconds()
	if duration < 2 || duration > 86400 {
		return nil
	}
	gaps := []string{}
	if open.window.Class == "" {
		gaps = append(gaps, "class_unknown")
	}
	workspace, found := model.DesktopWorkspace{}, false
	for _, candidate := range snapshot.Workspaces {
		if candidate.ID == open.window.WorkspaceID {
			workspace, found = candidate, true
			break
		}
	}
	if !found {
		gaps = append(gaps, "compositor_workspace_unknown")
	}
	sort.Strings(gaps)
	span := model.CompositorSurfaceFocusSpan{Version: 1, SourceID: o.source.ID, SourceEpoch: o.source.Epoch, Sequence: sequence, Window: open.window.Identity, Class: open.window.Class, CompositorWorkspaceID: workspace.ID, CompositorWorkspaceName: workspace.Name, Title: open.window.Title, StartedAt: open.startedAt, EndedAt: at, DurationSeconds: duration, Gaps: gaps}
	return &AttentionObservation{Surface: &span}
}
func (o *Observer) closeWorkspaceLocked(snapshot *model.DesktopSnapshot, sequence int64, at time.Time) *AttentionObservation {
	open := o.workspaceFocus
	o.workspaceFocus = nil
	duration := at.Sub(open.startedAt).Seconds()
	if duration < 2 || duration > 86400 {
		return nil
	}
	gaps := []string{}
	if open.workspace.MonitorName == "" {
		gaps = append(gaps, "compositor_monitor_unknown")
	}
	sort.Strings(gaps)
	span := model.CompositorWorkspaceFocusSpan{Version: 1, SourceID: o.source.ID, SourceEpoch: o.source.Epoch, Sequence: sequence, CompositorWorkspaceID: open.workspace.ID, CompositorWorkspaceName: open.workspace.Name, Monitor: open.workspace.MonitorName, StartedAt: open.startedAt, EndedAt: at, DurationSeconds: duration, Gaps: gaps}
	return &AttentionObservation{Workspace: &span}
}
