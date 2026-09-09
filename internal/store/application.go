package store

import (
	"fmt"
	"heimdall/internal/model"
)

func applyApplication(st *model.State, e Event) error {
	var v model.ApplicationRecipe
	if err := model.StrictJSON(e.Payload, &v); err != nil {
		return err
	}
	if err := v.Validate(); err != nil {
		return err
	}
	if e.Actor != "cli" || e.CommandID != "application-"+v.ID || e.EntityID != v.ID || !e.TS.Equal(v.At) || v.Actor != e.Actor || st.ApplicationRecipes[v.ID].ID != "" || st.ApplicationHeads[v.SurfaceID] != v.Previous || st.Tasks[v.Target].Revision != v.TaskRevision || st.WorkspaceHeads[v.Target] != v.ManifestID {
		return fmt.Errorf("application recipe authority, revision or head changed")
	}
	m := st.WorkspaceManifests[v.ManifestID]
	owned := false
	for _, surface := range m.Surfaces {
		if surface.ID == v.SurfaceID && (surface.Kind == "terminal" || surface.Kind == "editor" || surface.Kind == "browser") {
			owned = true
			if v.Active && (surface.Kind == "browser") != (v.Spec.Adapter == "browser") {
				return fmt.Errorf("browser recipe differs from surface kind")
			}
			if v.Active && (surface.Kind == "editor") != (v.Spec.Adapter == "foot-nvim") {
				return fmt.Errorf("recipe adapter differs from surface kind")
			}
		}
	}
	if !owned || m.TaskRevision != v.TaskRevision || (!v.Active && !st.ApplicationRecipes[v.Previous].Active) {
		return fmt.Errorf("current application surface required")
	}
	if v.Active && !model.ApplicationSessionCurrent(*st, v) {
		return fmt.Errorf("recipe requires exact current session binding or an unbound generic surface")
	}
	st.ApplicationRecipes[v.ID] = v
	st.ApplicationHeads[v.SurfaceID] = v.ID
	return nil
}

// The binding and its readback commit together. Replay checks the exact proof
// and never launches a process or queries an application.
func applicationBinding(st model.State, v model.ViewportBinding, e Event) error {
	a := st.Actions[v.ApplicationActionID]
	if a.Intent.Native == nil || a.Intent.Native.Launch == nil || a.Observation == nil || a.Observation.Native == nil || a.Verification != "matched" || !model.ApplicationAssociationCurrent(st, a.Intent) {
		return fmt.Errorf("application binding requires current verified launch")
	}
	r := a.Observation.Native
	if v.Target != a.Intent.Target || v.ManifestID != a.Intent.ManifestID || v.SurfaceID != a.Intent.SurfaceID || v.TaskRevision != a.Intent.TaskRevision || v.Previous != a.Intent.Native.Launch.PreviousViewport || v.SourceID != r.SourceID || v.SnapshotID != r.SnapshotID || r.Window == nil || *v.Window != r.Window.Identity || v.SessionBindingID != a.Intent.Native.Launch.SessionBindingID || !v.At.Equal(a.UpdatedAt) || e.ID != a.LastEventID+1 || e.CommandID != "native-verify-"+v.ID {
		return fmt.Errorf("application binding differs from observed launch")
	}
	return nil
}
