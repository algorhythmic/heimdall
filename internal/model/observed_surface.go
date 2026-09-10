package model

import (
	"fmt"
	"time"
)

// ObservedSurface is content identity, independent of desired manifest members,
// task bindings and the containers in which a sensor has observed it.
type ObservedSurface struct {
	ID                string    `json:"id"`
	Kind              string    `json:"kind"`
	NormalizedPointer string    `json:"normalized_pointer"`
	FirstRecordedAt   time.Time `json:"first_recorded_at"`
}

// BrowserSurfaceObservation v1 records inventory evidence, never action proof.
// Gap is invalid_pointer when the inventory URL cannot be normalized; such an
// observation has no SurfaceID and cannot manufacture content identity.
type BrowserSurfaceObservation struct {
	Version    int       `json:"version"`
	Profile    string    `json:"profile"`
	Epoch      string    `json:"epoch"`
	Sequence   int64     `json:"sequence"`
	TabID      int       `json:"tab_id"`
	WindowID   int       `json:"window_id"`
	SurfaceID  string    `json:"surface_id,omitempty"`
	Pointer    string    `json:"pointer"`
	Title      string    `json:"title"`
	ObservedAt time.Time `json:"observed_at"`
	Gap        string    `json:"gap,omitempty"`
}

func (o BrowserSurfaceObservation) ContainerKey() string {
	return fmt.Sprintf("browser:%s:%s:%d", o.Profile, o.Epoch, o.TabID)
}

// ObservedSurfaceContainer is the last recorded occurrence in one epoch-scoped
// container. Present describes that observation, not current source coverage.
// Consumers must also check pairing, epoch and fresh sensor coverage. Old epochs
// remain historical observations; source loss is not evidence of closure.
type ObservedSurfaceContainer struct {
	Observation        BrowserSurfaceObservation `json:"observation"`
	ObservedConnection string                    `json:"observed_connection"`
	Present            bool                      `json:"present"`
	LastEventID        int64                     `json:"last_event_id"`
}
