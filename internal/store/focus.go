package store

import (
	"fmt"
	"heimdall/internal/model"
	"heimdall/internal/surface"
	"strings"
)

func applySurfaceFocus(st *model.State, e Event) error {
	var f model.SurfaceFocusSpan
	if err := model.StrictJSON(e.Payload, &f); err != nil {
		return err
	}
	if f.Validate() != nil || e.Actor != "observer:browser" || !strings.HasPrefix(e.CommandID, "browser-"+f.Profile+"-") ||
		!model.OpaqueID.MatchString(f.Profile) || !model.OpaqueID.MatchString(f.Epoch) || !model.OpaqueID.MatchString(f.Connection) {
		return fmt.Errorf("invalid surface focus provenance")
	}
	p := st.Browsers[f.Profile]
	if !p.Paired || !p.Complete || p.Epoch != f.Epoch || p.Connection != f.Connection || p.LastSequence != f.Sequence ||
		!p.LastObservedAt.Equal(f.EndedAt) || !p.ReceivedAt.Equal(e.TS) {
		return fmt.Errorf("focus span requires matching received inventory")
	}
	i, err := surface.Identify(f.Pointer)
	key := (model.BrowserSurfaceObservation{Profile: f.Profile, Epoch: f.Epoch, TabID: f.TabID}).ContainerKey()
	c := st.SurfaceContainers[key]
	if err != nil || i.ID != f.SurfaceID || e.EntityID != f.SurfaceID || !c.Present || c.ObservedConnection != f.Connection || c.Observation.SurfaceID != f.SurfaceID ||
		c.Observation.Pointer != f.Pointer || c.Observation.WindowID != f.WindowID || c.Observation.Sequence >= f.Sequence {
		return fmt.Errorf("focus span requires a previously observed container and content")
	}
	for _, tab := range p.Tabs {
		if tab.ID == f.TabID && tab.WindowID == f.WindowID && tab.Active && p.FocusedWindow == f.WindowID && tab.URL == f.Pointer {
			return fmt.Errorf("focus span has no observed blur or navigation")
		}
	}
	if previous, ok := st.SurfaceFocusSpans[f.Profile]; ok && previous.EndedAt.After(f.StartedAt) {
		return fmt.Errorf("overlapping or duplicate surface focus span")
	}
	st.SurfaceFocusSpans[f.Profile] = f
	return nil
}
