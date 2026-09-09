package store

import (
	"fmt"
	"heimdall/internal/model"
)

func applyViewport(st *model.State, e Event) error {
	if e.Actor != "cli" || e.CommandID != "viewport-"+e.EntityID {
		return fmt.Errorf("invalid viewport command authority")
	}
	if _, ok := st.DesktopSources[e.EntityID]; ok {
		return fmt.Errorf("duplicate viewport record")
	}
	if _, ok := st.ViewportBindings[e.EntityID]; ok {
		return fmt.Errorf("duplicate viewport record")
	}
	if e.Subject == "desktop" && e.Verb == "selected" {
		var v model.DesktopSource
		if err := model.StrictJSON(e.Payload, &v); err != nil {
			return err
		}
		if err := v.Validate(); err != nil {
			return err
		}
		if v.ID != e.EntityID || !v.At.Equal(e.TS) || v.Previous != st.DesktopSourceHead {
			return fmt.Errorf("invalid desktop selection envelope or head")
		}
		if !v.Active && !st.DesktopSources[v.Previous].Active {
			return fmt.Errorf("desktop source not selected")
		}
		st.DesktopSources[v.ID] = v
		st.DesktopSourceHead = v.ID
		return nil
	}
	var v model.ViewportBinding
	if err := model.StrictJSON(e.Payload, &v); err != nil {
		return err
	}
	if err := v.Validate(); err != nil {
		return err
	}
	if v.ID != e.EntityID || !v.At.Equal(e.TS) || v.TaskRevision != st.Tasks[v.Target].Revision || v.Previous != st.ViewportHeads[v.SurfaceID] || v.Active != (e.Verb == "bound") {
		return fmt.Errorf("invalid viewport envelope, revision or head")
	}
	m, ok := st.WorkspaceManifests[v.ManifestID]
	if !ok || m.Target != v.Target || st.WorkspaceHeads[v.Target] != m.ID {
		return fmt.Errorf("current task-owned workspace required")
	}
	kind := ""
	for _, surface := range m.Surfaces {
		if surface.ID == v.SurfaceID {
			kind = surface.Kind
		}
	}
	if kind == "" {
		return fmt.Errorf("desired surface missing")
	}
	if v.Active {
		if kind == "browser" {
			return fmt.Errorf("browser viewport pairing requires the C13 profile/window handshake")
		}
		source := st.DesktopSources[v.SourceID]
		if !source.Active || st.DesktopSourceHead != v.SourceID || v.Window.SourceEpoch != source.Epoch || m.TaskRevision != v.TaskRevision {
			return fmt.Errorf("current source and reviewed manifest required")
		}
		for surface, id := range st.ViewportHeads {
			old := st.ViewportBindings[id]
			if surface != v.SurfaceID && old.Active && old.Window != nil && *old.Window == *v.Window {
				return fmt.Errorf("window already bound; explicitly unbind it first")
			}
		}
		if v.SessionBindingID != "" {
			b := st.SessionBindings[v.SessionBindingID]
			if kind != "terminal" || !b.Active || b.Target != v.Target || b.SurfaceID != v.SurfaceID || st.SessionHeads[v.SurfaceID] != b.ID {
				return fmt.Errorf("current same-surface session binding required")
			}
		}
	} else if !st.ViewportBindings[v.Previous].Active {
		return fmt.Errorf("viewport is not bound")
	}
	st.ViewportBindings[v.ID] = v
	st.ViewportHeads[v.SurfaceID] = v.ID
	return nil
}
