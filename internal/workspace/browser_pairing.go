package workspace

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/actions"
	"heimdall/internal/adapters/hyprland"
	"heimdall/internal/browser"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"sort"
	"time"
)

type BrowserPairingService struct {
	Store    *store.Store
	Browser  *browser.Service
	Observer *hyprland.Observer
	Clock    func() time.Time
}

func (s *BrowserPairingService) now() time.Time {
	if s.Clock != nil {
		return s.Clock().UTC()
	}
	return time.Now().UTC()
}
func (s *BrowserPairingService) Run(ctx context.Context) {
	tick := time.NewTicker(250 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			_ = s.Tick(ctx)
		}
	}
}
func (s *BrowserPairingService) Tick(ctx context.Context) error {
	st, err := s.Store.State(ctx)
	if err != nil {
		return err
	}
	ids := []string{}
	for id, a := range st.Actions {
		if a.Intent.Browser.Pairing != nil && a.Pairing != nil && a.Pairing.Ready != nil && a.Pairing.ContinuationDeliveryID == "" && !a.Pairing.Abandoned && !a.CancelRequested && a.Pairing.ProbeAttempts <= 3 && s.now().Before(a.Intent.ExpiresAt) {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		if err := s.Observe(ctx, id); err != nil {
			return err
		}
	}
	return nil
}
func (s *BrowserPairingService) Observe(ctx context.Context, id string) error {
	st, err := s.Store.State(ctx)
	if err != nil {
		return err
	}
	a, ok := st.Actions[id]
	if !ok || a.Intent.Browser.Pairing == nil {
		return fmt.Errorf("pairing action missing")
	}
	lease, err := s.Browser.Lease(ctx, a.Intent.Browser.Profile)
	if err != nil {
		return err
	}
	p := lease.Profile
	if !model.BrowserPairFresh(p, a, s.now()) || !model.ActionInputsCurrent(st, a.Intent) || !model.ActionBrowserCurrent(st, a.Intent) {
		return fmt.Errorf("pairing inputs or challenged marker changed")
	}
	if a.Pairing.AssociationID != "" {
		proof := st.BrowserAssociations[a.Pairing.AssociationID]
		if a.Pairing.Probe != nil && p.Connection == a.Pairing.Probe.Connection && p.EventGeneration == a.Pairing.Probe.EventGeneration && s.now().Sub(proof.At) < 5*time.Second && s.Observer.Check(proof.SnapshotID) == nil {
			return nil
		}
	}
	observed, err := s.Observer.Read(ctx, true)
	if err != nil {
		return err
	}
	if !observed.Fresh || observed.Snapshot == nil || observed.Snapshot.SourceEpoch != a.Intent.Browser.Pairing.SourceEpoch {
		return fmt.Errorf("fresh selected compositor unavailable")
	}
	count := 0
	markerTitle := ""
	var window model.WindowIdentity
	for _, w := range observed.Snapshot.Windows {
		if model.IsBrowserPairNativeTitle(w.Title, id) {
			count++
			markerTitle = w.Title
			window = w.Identity
		}
	}
	if count != 1 {
		return fmt.Errorf("pairing requires exactly one nonce window; found %d", count)
	}
	probe := a.Pairing.Probe
	second := a.Pairing.AssociationID == "" && probe != nil && probe.Connection == p.Connection && probe.EventGeneration == p.EventGeneration && probe.Window == window
	recordID := model.NewID()
	command := "browser-association-" + recordID
	now := s.now()
	raw, _ := json.Marshal(map[string]any{"version": 1, "action_id": id, "revision": a.Revision, "browser_digest": model.ContentDigest(p), "snapshot_id": observed.Snapshot.ID, "second": second})
	check := func(current model.State) error {
		if err := lease.Check(current, s.Store.RuntimeID()); err != nil {
			return err
		}
		return s.Observer.Check(observed.Snapshot.ID)
	}
	_, err = s.Store.TransactChecked(ctx, command, "coordinator", raw, now, check, func(current model.State) (store.Change, error) {
		change := store.Change{Revision: current.Revision}
		if current.Actions[id].Revision != a.Revision {
			return change, fmt.Errorf("pairing action changed: %w", store.ErrConflict)
		}
		if !second {
			v := actions.Transition(a, "pair_probe", "First unique nonce window observed; requesting second browser read", "coordinator", now)
			v.Version = 3
			v.PairProbe = &model.BrowserPairProbe{ID: recordID, BrowserDigest: model.ContentDigest(p), Connection: p.Connection, EventGeneration: p.EventGeneration, SourceID: a.Intent.Browser.Pairing.SourceID, Window: window, SnapshotID: observed.Snapshot.ID, CapturedAt: observed.Snapshot.CapturedAt, MarkerTitle: markerTitle, MatchingWindows: count}
			change.Events = []store.Pending{actions.Pending(v)}
			change.Result = v
			return change, nil
		}
		proof := model.BrowserAssociation{Version: 1, ID: recordID, ProbeID: probe.ID, ActionRef: *a.BrowserRef(), Profile: p.ID, Epoch: p.Epoch, WindowID: a.Pairing.Ready.WindowID, MarkerTabID: a.Pairing.Ready.MarkerTabID, SourceID: a.Intent.Browser.Pairing.SourceID, Window: window, SnapshotID: observed.Snapshot.ID, CapturedAt: observed.Snapshot.CapturedAt, BrowserDigest: model.ContentDigest(p), MatchingWindows: count, MarkerTitle: markerTitle, ContinuationID: model.NewID(), At: now}
		binding := model.ViewportBinding{Version: 2, ID: proof.ID, BrowserAssociationID: proof.ID, Target: a.Intent.Target, TaskRevision: a.Intent.TaskRevision, ManifestID: a.Intent.ManifestID, SurfaceID: a.Intent.SurfaceID, Previous: current.ViewportHeads[a.Intent.SurfaceID], Active: true, SourceID: proof.SourceID, SnapshotID: proof.SnapshotID, Window: &proof.Window, Actor: "coordinator", At: now}
		v := actions.Transition(a, "pair_bound", "Browser window associated through two ordered nonce observations", "coordinator", now)
		v.Version = 3
		v.AssociationID = proof.ID
		v.ContinuationID = proof.ContinuationID
		change.Events = []store.Pending{{Subject: "browser", Verb: "association_observed", EntityID: proof.ID, Payload: proof}, {Subject: "viewport", Verb: "bound", EntityID: binding.ID, Payload: binding}, actions.Pending(v)}
		change.Result = proof
		return change, nil
	})
	return err
}
