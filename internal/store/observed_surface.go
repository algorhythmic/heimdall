package store

import (
	"fmt"
	"heimdall/internal/model"
	"heimdall/internal/surface"
	"strings"
)

func applyObservedSurface(st *model.State, e Event) error {
	var o model.BrowserSurfaceObservation
	if err := model.StrictJSON(e.Payload, &o); err != nil {
		return err
	}
	if o.Version != 1 || e.Actor != "observer:browser" ||
		!model.OpaqueID.MatchString(o.Profile) || !model.OpaqueID.MatchString(o.Epoch) ||
		!strings.HasPrefix(e.CommandID, "browser-"+o.Profile+"-") ||
		o.TabID < 1 || o.WindowID < 1 || o.Sequence < 1 || o.ObservedAt.IsZero() || e.EntityID != o.SurfaceID {
		return fmt.Errorf("invalid browser surface observation envelope")
	}
	p, ok := st.Browsers[o.Profile]
	if !ok || !p.Paired || p.Epoch != o.Epoch || p.LastSequence != o.Sequence ||
		!p.LastObservedAt.Equal(o.ObservedAt) || !p.ReceivedAt.Equal(e.TS) {
		return fmt.Errorf("surface observation requires matching received inventory")
	}
	identity, identityErr := surface.Identify(o.Pointer)
	if identityErr != nil {
		if o.Gap != "invalid_pointer" || o.SurfaceID != "" {
			return fmt.Errorf("unresolved surface requires an explicit identity gap")
		}
	} else if o.Gap != "" || o.SurfaceID != identity.ID {
		return fmt.Errorf("surface content identity mismatch")
	}
	if prior, exists := st.ObservedSurfaces[o.SurfaceID]; identityErr == nil && exists &&
		(prior.Kind != identity.Kind || prior.NormalizedPointer != identity.NormalizedPointer) {
		return fmt.Errorf("observed surface hash collision")
	}
	var tab *model.BrowserTab
	for i := range p.Tabs {
		if p.Tabs[i].ID == o.TabID {
			tab = &p.Tabs[i]
			break
		}
	}
	key := o.ContainerKey()
	old, exists := st.SurfaceContainers[key]
	if exists && old.Observation.Sequence > o.Sequence {
		return fmt.Errorf("surface observation sequence regressed")
	}
	if e.Verb == "closed" {
		before := old.Observation
		before.Sequence, before.ObservedAt = o.Sequence, o.ObservedAt
		if !exists || !old.Present || before != o || old.Observation.Sequence >= o.Sequence {
			return fmt.Errorf("surface closure requires the previous present occurrence")
		}
		if tab != nil {
			next, err := surface.Identify(tab.URL)
			if err != nil || o.Gap != "" || next.ID == o.SurfaceID {
				return fmt.Errorf("surface closure requires absence or observed navigation")
			}
		}
	} else {
		if tab == nil || tab.URL != o.Pointer || tab.Title != o.Title || tab.WindowID != o.WindowID {
			return fmt.Errorf("surface content differs from inventory")
		}
		switch e.Verb {
		case "observed":
			if exists && old.Present && old.Observation.Gap == "" && o.Gap == "" {
				return fmt.Errorf("existing surface requires a change or navigation event")
			}
		case "opened":
			if o.Gap != "" || (exists && old.Present) {
				return fmt.Errorf("surface opening requires resolved content and a vacant container")
			}
		case "changed":
			if !exists || !old.Present || o.Gap != "" || old.Observation.Gap != "" || old.Observation.SurfaceID != o.SurfaceID {
				return fmt.Errorf("surface change must retain the current content identity")
			}
		default:
			return fmt.Errorf("unsupported surface observation verb")
		}
		if exists && old.Present && old.Observation.Sequence >= o.Sequence {
			return fmt.Errorf("duplicate surface observation")
		}
	}
	if identityErr == nil {
		if _, exists := st.ObservedSurfaces[o.SurfaceID]; !exists {
			st.ObservedSurfaces[o.SurfaceID] = model.ObservedSurface{ID: identity.ID, Kind: identity.Kind,
				NormalizedPointer: identity.NormalizedPointer, FirstRecordedAt: e.TS}
		}
	}
	st.SurfaceContainers[key] = model.ObservedSurfaceContainer{Observation: o, ObservedConnection: p.Connection, Present: e.Verb != "closed", LastEventID: e.ID}
	return nil
}
