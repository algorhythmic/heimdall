package model

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"
)

func SnapshotHash(raw []byte) string { return fmt.Sprintf("%x", sha256.Sum256(raw)) }
func SnapshotInputDigest(st State, target string) string {
	m := st.WorkspaceManifests[st.WorkspaceHeads[target]]
	type binding struct{ Surface, Viewport, Session string }
	bindings := []binding{}
	for _, surface := range m.Surfaces {
		bindings = append(bindings, binding{surface.ID, st.ViewportHeads[surface.ID], st.SessionHeads[surface.ID]})
	}
	raw, _ := json.Marshal(struct {
		Target           string
		Revision         int64
		Manifest, Source string
		Bindings         []binding
	}{target, st.Tasks[target].Revision, m.ID, st.DesktopSourceHead, bindings})
	return SnapshotHash(raw)
}
func SnapshotContentDigest(p WorkspacePointPayload) string {
	p.ObservedAt = WorkspacePointPayload{}.ObservedAt
	p.Boundary = SnapshotBoundary{Method: p.Boundary.Method}
	raw, _ := json.Marshal(p)
	return SnapshotHash(raw)
}
func ValidateSnapshotPayload(st State, v WorkspacePoint, raw []byte) error {
	if len(raw) != v.PayloadBytes || SnapshotHash(raw) != v.PayloadDigest {
		return fmt.Errorf("snapshot payload bytes or digest mismatch")
	}
	var p WorkspacePointPayload
	if err := StrictJSON(raw, &p); err != nil {
		return err
	}
	if p.Version != 1 || p.Target != v.Target || p.TaskRevision != v.TaskRevision || p.ManifestID != v.ManifestID || p.SourceEpoch != v.SourceEpoch || !p.ObservedAt.Equal(v.ObservedAt) || p.Coverage != v.Coverage || SnapshotContentDigest(p) != v.ContentDigest {
		return fmt.Errorf("snapshot payload envelope mismatch")
	}
	if p.Boundary.Method != "double_inventory_with_buffered_unsequenced_events" || p.Boundary.StartedAt.IsZero() || !p.Boundary.FinishedAt.Equal(p.ObservedAt) || p.Boundary.StartedAt.After(p.Boundary.FinishedAt) || p.Boundary.FinishedAt.Sub(p.Boundary.StartedAt) > 3*time.Second {
		return fmt.Errorf("invalid snapshot source capture boundary")
	}
	m := st.WorkspaceManifests[v.ManifestID]
	if p.Surfaces == nil || len(p.Surfaces) != len(m.Surfaces) || p.Monitors == nil || len(p.Monitors) < 1 || len(p.Monitors) > 32 || p.Workspaces == nil || len(p.Workspaces) > 128 {
		return fmt.Errorf("snapshot topology bounds")
	}
	monitors := map[int]bool{}
	workspaces := map[int]bool{}
	for _, mon := range p.Monitors {
		if monitors[mon.ID] || mon.ID < 0 || mon.Name == "" || len(mon.Name) > 256 || mon.Width <= 0 || mon.Height <= 0 || mon.Scale <= 0 || mon.Scale > 16 || mon.Transform < 0 || mon.Transform > 7 {
			return fmt.Errorf("snapshot monitor topology")
		}
		monitors[mon.ID] = true
	}
	for _, ws := range p.Workspaces {
		if workspaces[ws.ID] || ws.ID == 0 || ws.Name == "" || len(ws.Name) > 256 || !monitors[ws.MonitorID] {
			return fmt.Errorf("snapshot workspace topology")
		}
		workspaces[ws.ID] = true
	}
	complete := len(p.Surfaces) > 0
	seen := map[WindowIdentity]bool{}
	for i, surface := range p.Surfaces {
		if surface.SurfaceID != m.Surfaces[i].ID || surface.ViewportBindingID != st.ViewportHeads[surface.SurfaceID] || surface.SessionBindingID != st.SessionHeads[surface.SurfaceID] {
			return fmt.Errorf("snapshot surface binding changed")
		}
		b := st.ViewportBindings[surface.ViewportBindingID]
		if surface.Status == "observed" {
			w := surface.Window
			if w == nil || !b.Active || b.Window == nil || w.Identity != *b.Window || w.Identity.SourceEpoch != v.SourceEpoch || seen[w.Identity] || !monitors[w.MonitorID] || !workspaces[w.WorkspaceID] || w.PID < 1 || len(w.Title) > 512 || len(w.Class) > 512 {
				return fmt.Errorf("snapshot window identity or topology mismatch")
			}
			seen[w.Identity] = true
		} else {
			if !Contains([]string{"unowned", "missing", "unavailable"}, surface.Status) || surface.Window != nil {
				return fmt.Errorf("invalid missing snapshot surface")
			}
			complete = false
		}
	}
	if complete != (v.Coverage == "complete") {
		return fmt.Errorf("snapshot completeness differs from payload")
	}
	return nil
}
