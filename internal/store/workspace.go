package store

import (
	"fmt"
	"heimdall/internal/model"
)

func applyWorkspace(st *model.State, e Event) error {
	if e.Actor != "cli" || e.CommandID != "workspace-"+e.EntityID {
		return fmt.Errorf("invalid workspace command authority or identity")
	}
	if _, ok := st.WorkspaceManifests[e.EntityID]; ok {
		return fmt.Errorf("duplicate workspace record")
	}
	if _, ok := st.SessionBindings[e.EntityID]; ok {
		return fmt.Errorf("duplicate workspace record")
	}
	switch e.Subject + "." + e.Verb {
	case "workspace.accepted":
		var v model.WorkspaceManifest
		if err := model.StrictJSON(e.Payload, &v); err != nil {
			return err
		}
		if err := model.ValidWorkspaceRecord(v.Version, v.ID, v.Target, v.Previous, v.Actor, v.At, v.TaskRevision); err != nil {
			return err
		}
		if e.EntityID != v.ID || !e.TS.Equal(v.At) || st.WorkspaceHeads[v.Target] != v.Previous || st.Tasks[v.Target].Revision != v.TaskRevision {
			return fmt.Errorf("invalid workspace envelope, head or task revision")
		}
		if err := model.ValidDesiredWorkspace(v.Name, v.Surfaces); err != nil {
			return err
		}
		present := map[string]bool{}
		for _, surface := range v.Surfaces {
			if old, ok := st.WorkspaceSurfaces[surface.ID]; ok && (old.Target != v.Target || old.Kind != surface.Kind) {
				return fmt.Errorf("surface identity belongs to another task or kind")
			}
			present[surface.ID] = true
		}
		for id, head := range st.SessionHeads {
			b := st.SessionBindings[head]
			if b.Target == v.Target && b.Active && !present[id] {
				return fmt.Errorf("unbind session before removing its desired surface")
			}
		}
		for _, surface := range v.Surfaces {
			st.WorkspaceSurfaces[surface.ID] = model.SurfaceIdentity{Target: v.Target, Kind: surface.Kind}
		}
		st.WorkspaceManifests[v.ID] = v
		st.WorkspaceHeads[v.Target] = v.ID
	case "session.bound", "session.unbound":
		var v model.SessionBinding
		if err := model.StrictJSON(e.Payload, &v); err != nil {
			return err
		}
		if err := model.ValidSessionBinding(v); err != nil {
			return err
		}
		if e.EntityID != v.ID || !e.TS.Equal(v.At) || st.Tasks[v.Target].Revision != v.TaskRevision || !model.OpaqueID.MatchString(v.SurfaceID) || st.SessionHeads[v.SurfaceID] != v.Previous {
			return fmt.Errorf("invalid session envelope, head or task revision")
		}
		m, ok := st.WorkspaceManifests[v.ManifestID]
		if !ok || m.Target != v.Target || st.WorkspaceHeads[v.Target] != m.ID {
			return fmt.Errorf("session requires the current task-owned manifest")
		}
		present := false
		for _, surface := range m.Surfaces {
			if surface.ID == v.SurfaceID && surface.Kind == "terminal" {
				present = true
			}
		}
		if !present {
			return fmt.Errorf("session requires a terminal surface in the selected manifest")
		}
		if old, ok := st.SessionBindings[v.Previous]; v.Previous != "" && (!ok || old.Target != v.Target || old.SurfaceID != v.SurfaceID) {
			return fmt.Errorf("invalid previous session scope")
		}
		if v.Active != (e.Verb == "bound") || v.Active != (v.Locator != nil) {
			return fmt.Errorf("invalid session lifecycle")
		}
		if v.Active {
			if m.TaskRevision != v.TaskRevision {
				return fmt.Errorf("review stale workspace manifest before binding")
			}
			for surface, head := range st.SessionHeads {
				old := st.SessionBindings[head]
				same := old.Locator != nil && v.Locator.SameSessionPane(*old.Locator)
				if v.Herdr != nil && old.Herdr != nil && old.Locator != nil && v.Locator.Host == old.Locator.Host && v.Locator.SourceEpoch == old.Locator.SourceEpoch && v.Herdr.TerminalID == old.Herdr.TerminalID {
					same = true
				}
				if surface != v.SurfaceID && old.Active && same {
					return fmt.Errorf("runtime pane already bound; explicitly unbind it first")
				}
			}
		} else if !st.SessionBindings[v.Previous].Active {
			return fmt.Errorf("session is not bound")
		}
		st.SessionBindings[v.ID] = v
		st.SessionHeads[v.SurfaceID] = v.ID
	default:
		return fmt.Errorf("unknown workspace event")
	}
	return nil
}
