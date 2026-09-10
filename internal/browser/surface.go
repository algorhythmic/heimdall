package browser

import (
	"heimdall/internal/model"
	"heimdall/internal/store"
	"heimdall/internal/surface"
	"sort"
)

// surfaceEvents runs inside the inventory transaction. Only supplied tabs are
// positive observations: a partial inventory's inherited tabs are not re-seen.
func surfaceEvents(st model.State, p model.BrowserProfile, m Message) []store.Pending {
	events := []store.Pending{}
	emit := func(verb string, o model.BrowserSurfaceObservation) {
		events = append(events, store.Pending{Subject: "surface", Verb: verb, EntityID: o.SurfaceID, Payload: o})
	}
	current := map[int]bool{}
	for _, tab := range p.Tabs {
		current[tab.ID] = true
	}
	removed := map[int]bool{}
	for _, id := range m.Removed {
		removed[id] = true
	}
	// A full inventory or explicit delta removal can establish absence in this
	// epoch. A reconnect or a new epoch alone cannot establish closure.
	closed := []model.BrowserSurfaceObservation{}
	for _, container := range st.SurfaceContainers {
		o := container.Observation
		absenceCovered := (!m.Delta && *m.Complete) || removed[o.TabID]
		if container.Present && o.Profile == p.ID && o.Epoch == p.Epoch && !current[o.TabID] && absenceCovered {
			o.Sequence, o.ObservedAt = p.LastSequence, p.LastObservedAt
			closed = append(closed, o)
		}
	}
	sort.Slice(closed, func(i, j int) bool { return closed[i].TabID < closed[j].TabID })
	for _, o := range closed {
		emit("closed", o)
	}
	tabs := append([]model.BrowserTab(nil), m.Tabs...)
	sort.Slice(tabs, func(i, j int) bool { return tabs[i].ID < tabs[j].ID })
	for _, tab := range tabs {
		o := model.BrowserSurfaceObservation{Version: 1, Profile: p.ID, Epoch: p.Epoch,
			Sequence: p.LastSequence, TabID: tab.ID, WindowID: tab.WindowID,
			Pointer: tab.URL, Title: tab.Title, ObservedAt: p.LastObservedAt}
		identity, err := surface.Identify(tab.URL)
		if err != nil {
			o.Gap = "invalid_pointer"
		} else {
			o.SurfaceID = identity.ID
		}
		old, exists := st.SurfaceContainers[o.ContainerKey()]
		if !exists || !old.Present {
			verb := "observed"
			// An initial snapshot proves existence, not when the tab opened.
			prior := st.Browsers[p.ID]
			previouslyListed := false
			for _, t := range prior.Tabs {
				previouslyListed = previouslyListed || t.ID == tab.ID
			}
			if prior.Epoch == p.Epoch && prior.Complete && !prior.ReceivedAt.IsZero() && !previouslyListed {
				verb = "opened"
			}
			if o.Gap != "" {
				verb = "observed"
			}
			emit(verb, o)
			continue
		}
		before := old.Observation
		if before.Pointer == o.Pointer && before.Title == o.Title && before.WindowID == o.WindowID && before.Gap == o.Gap {
			continue
		}
		if o.Gap != "" || before.Gap != "" {
			// Loss/recovery of identity resolution is explicit, not navigation
			// to fabricated content. Raw inventory retention remains unchanged.
			emit("observed", o)
		} else if before.SurfaceID != o.SurfaceID {
			before.Sequence, before.ObservedAt = o.Sequence, o.ObservedAt
			emit("closed", before)
			emit("opened", o)
		} else {
			emit("changed", o)
		}
	}
	return events
}
