package browser

import (
	"heimdall/internal/actions"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"time"
)

func (s Service) continuationAllowed(st model.State, a model.ActionRecord, now time.Time, after int64) bool {
	if a.Pairing == nil || a.Pairing.Ready == nil || a.Pairing.AssociationID == "" || a.Pairing.Abandoned || a.CancelRequested || !model.Contains([]string{"dispatching", "uncertain"}, a.Execution) || !now.Before(a.Intent.ExpiresAt) || !model.ActionInputsCurrent(st, a.Intent) || !model.ActionBrowserCurrent(st, a.Intent) {
		return false
	}
	p := st.Browsers[a.Intent.Browser.Profile]
	proof := st.BrowserAssociations[a.Pairing.AssociationID]
	return a.Pairing.Probe != nil && p.Connection == a.Pairing.Probe.Connection && p.EventGeneration == a.Pairing.Probe.EventGeneration && model.BrowserPairMarker(p, a) && s.fresh(p, after, now) && !now.Before(proof.At) && now.Sub(proof.At) < 5*time.Second && s.AssociationCheck != nil && s.AssociationCheck(proof) == nil
}
func (s Service) continuations(st model.State, m Message, now time.Time) ([]model.BrowserContinuation, []store.Pending) {
	out := []model.BrowserContinuation{}
	events := []store.Pending{}
	for _, a := range st.Actions {
		if a.Intent.Browser.Profile != m.Profile || a.Pairing == nil || a.Pairing.ContinuationDeliveryID != "" || !s.continuationAllowed(st, a, now, a.LastEventID) {
			continue
		}
		r := a.Pairing.Ready
		b := a.Intent.Browser
		c := model.BrowserContinuation{Version: 1, ID: a.Pairing.ContinuationID, ActionRef: *a.BrowserRef(), Profile: b.Profile, Epoch: b.Epoch, MarkerTabID: r.MarkerTabID, WindowID: r.WindowID, OriginalTabID: r.OriginalTabID, Action: b.Action, URL: b.URL, ExpiresAt: a.Intent.ExpiresAt}
		p := st.Browsers[b.Profile]
		v := actions.Transition(a, "pair_continue", "Native association verified; one-time marker continuation delivered", "coordinator", now)
		v.Version = 3
		v.ContinuationID = c.ID
		v.DeliveryID = m.ID
		v.Observation = &model.ActionObservation{ID: model.NewID(), Status: "unknown", SourceEpoch: p.Epoch, Digest: model.ContentDigest(p), ObservedAt: p.ReceivedAt, Detail: "Fresh marker before independently journaled continuation"}
		out = append(out, c)
		events = append(events, actions.Pending(v))
	}
	return out, events
}
