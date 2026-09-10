package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/adapters/hyprland"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"time"
)

type AttentionService struct{ Store *store.Store }

func (s AttentionService) Record(observed hyprland.AttentionObservation) error {
	if s.Store == nil || (observed.Surface == nil && observed.Workspace == nil) {
		return fmt.Errorf("attention store unavailable")
	}
	at := observed.EndedAt()
	if at.IsZero() {
		return fmt.Errorf("attention span end missing")
	}
	sourceID, epoch := "", ""
	if observed.Surface != nil {
		sourceID, epoch = observed.Surface.SourceID, observed.Surface.SourceEpoch
	} else {
		sourceID, epoch = observed.Workspace.SourceID, observed.Workspace.SourceEpoch
	}
	command := fmt.Sprintf("hyprland-attention-%s-%s-%d", sourceID, epoch, observed.SourceSequence())
	raw, err := json.Marshal(observed)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	st, err := s.Store.State(ctx)
	if err != nil {
		return err
	}
	if observed.Surface != nil && pairedBrowserWindow(st, sourceID, observed.Surface.Window) {
		return hyprland.ErrAttentionExcluded
	}
	_, err = s.Store.Transact(ctx, command, "observer:hyprland", raw, at, func(st model.State) (store.Change, error) {
		events := []store.Pending{}
		if observed.Surface != nil {
			events = append(events, store.Pending{Subject: "surface", Verb: "focused", EntityID: observed.Surface.Window.StableID, Payload: *observed.Surface})
		}
		if observed.Workspace != nil {
			events = append(events, store.Pending{Subject: "workspace", Verb: "focused", EntityID: sourceID, Payload: *observed.Workspace})
		}
		return store.Change{Revision: st.Revision, Events: events}, nil
	})
	return err
}
func pairedBrowserWindow(st model.State, source string, window model.WindowIdentity) bool {
	for _, proof := range st.BrowserAssociations {
		profile := st.Browsers[proof.Profile]
		if proof.SourceID == source && proof.Window == window && profile.Paired && profile.Connection != "" && profile.Epoch == proof.Epoch {
			return true
		}
	}
	return false
}
