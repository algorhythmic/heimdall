package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/actions"
	"heimdall/internal/adapters/hyprland"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"heimdall/internal/surface"
	"reflect"
	"sort"
	"time"
)

type Service struct {
	AssociationCheck func(model.BrowserAssociation) error
	Compositor       *hyprland.Observer
	Store            *store.Store
	Runtime          *Runtime
}

func (s Service) handle(ctx context.Context, m Message, now time.Time) (json.RawMessage, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	request, _ := json.Marshal(m)
	actor := "observer:browser"
	if m.Type == "poll" {
		actor = "coordinator"
	}
	authorize := func(st model.State) error {
		if m.Type == "hello" {
			return nil
		}
		p, ok := st.Browsers[m.Profile]
		if !ok || p.Epoch != m.Epoch || p.Connection != m.Connection {
			return fmt.Errorf("stale_connection: reconnect before sending messages")
		}
		if (m.Type == "poll" || m.Type == "command_result" || m.Type == "readback" || m.Type == "pairing_ready") && !p.Paired {
			return fmt.Errorf("profile_unpaired")
		}
		if m.Type == "readback" {
			if err := s.readbackAllowed(p, m, now); err != nil {
				return err
			}
		}
		if m.Type == "poll" {
			for _, a := range st.Actions {
				if a.Pairing != nil && a.Pairing.ContinuationDeliveryID == m.ID && !s.continuationAllowed(st, a, now, 0) {
					return fmt.Errorf("cached continuation no longer authorized")
				}
				if a.Intent.Browser != nil && a.DeliveryID == m.ID && a.Intent.Browser.Profile == m.Profile {
					if a.Pairing != nil && a.Pairing.Ready != nil {
						return fmt.Errorf("first pairing phase already reported")
					}
					if a.Execution != "dispatching" || a.CancelRequested || !model.ActionInputsCurrent(st, a.Intent) || !model.ActionBrowserCurrent(st, a.Intent) || !now.Before(a.Intent.ExpiresAt) || !s.fresh(p, 0, now) {
						return fmt.Errorf("cached action delivery no longer authorized")
					}
				}
			}
		}
		return nil
	}
	return s.Store.TransactChecked(ctx, "browser-"+m.Profile+"-"+m.ID, actor, request, now, authorize, func(st model.State) (store.Change, error) {
		reply := Reply{V: 1, Type: "ack", ID: m.ID, Profile: m.Profile}
		change := store.Change{Revision: st.Revision}
		p, exists := st.Browsers[m.Profile]
		if m.Type == "hello" {
			transportChanged := exists && (p.Connection != m.Connection || p.Epoch != m.Epoch)
			if !exists {
				p = model.BrowserProfile{ID: m.Profile, Tabs: []model.BrowserTab{}, FocusedWindow: -1}
			}
			if p.Epoch != m.Epoch {
				p.Tabs = []model.BrowserTab{}
				p.LastSequence = 0
				p.Complete = false
				p.FocusedWindow = -1
			}
			p.Epoch = m.Epoch
			p.Connection = m.Connection
			p.Label = m.Label
			p.ExtensionVersion = m.ExtensionVersion
			p.ActionProtocol = m.ActionProtocol
			p.VerificationProtocol = m.VerificationProtocol
			p.PairingProtocol = m.PairingProtocol
			p.RecoveryProtocol = m.RecoveryProtocol
			p.ExtensionID = m.ExtensionID
			p.EventGeneration = 0
			p.Markers = nil
			p.Challenge = nil
			p.Freshness = nil
			p.PresentTabs = nil
			// Every transport hello requires a fresh daemon-received inventory.
			p.ReceivedAt = time.Time{}
			p.ReceivedEpoch = ""
			reply.Type = "welcome"
			reply.Paired = p.Paired
			reply.LastSequence = p.LastSequence
			if !p.Paired {
				reply.Type = "pairing_required"
			}
			change.Events = []store.Pending{{Subject: "browser", Verb: "profile_seen", EntityID: p.ID, Payload: p}}
			if transportChanged {
				ids := []string{}
				for id, a := range st.Actions {
					if a.Intent.Browser != nil && a.Intent.Browser.Profile == p.ID && a.Execution == "dispatching" {
						ids = append(ids, id)
					}
				}
				sort.Strings(ids)
				for _, id := range ids {
					change.Events = append(change.Events, actions.Pending(actions.Transition(st.Actions[id], "interrupt", "browser transport replaced after possible dispatch", actor, now)))
				}
			}

			ids := []string{}
			for id, a := range st.Actions {
				if a.Intent.Browser != nil && a.Intent.Browser.Profile == p.ID && a.Intent.Browser.Epoch != p.Epoch && model.ActionHolds(a) && model.Contains([]string{"api_reported", "uncertain"}, a.Execution) {
					ids = append(ids, id)
				}
			}
			sort.Strings(ids)
			for _, id := range ids {
				v := actions.Transition(st.Actions[id], "source_lost", "Browser session epoch changed; old runtime outcome requires explicit recovery", actor, now)
				v.Version = 2
				change.Events = append(change.Events, actions.Pending(v))
			}
			change.Result = reply
			return change, nil
		}
		if !exists || p.Epoch != m.Epoch || p.Connection != m.Connection {
			return change, fmt.Errorf("stale_connection: reconnect before sending messages")
		}
		reply.Paired = p.Paired
		reply.LastSequence = p.LastSequence
		if !p.Paired {
			reply.Type = "pairing_required"
			change.Result = reply
			return change, nil
		}
		switch m.Type {
		case "inventory":
			prior := p
			if m.Delta && (m.BaseSequence != p.LastSequence || p.ReceivedAt.IsZero() || p.ReceivedEpoch != s.Store.RuntimeID() || p.InventorySnapshotAt == nil || now.Sub(*p.InventorySnapshotAt) >= 24*time.Hour) {
				return change, fmt.Errorf("snapshot_required: inventory baseline unavailable")
			}
			if m.Sequence <= p.LastSequence {
				return change, fmt.Errorf("stale_sequence: a newer inventory already committed")
			}
			tabs := map[int]model.BrowserTab{}
			if m.Delta || !*m.Complete {
				for _, t := range p.Tabs {
					tabs[t.ID] = t
				}
			}
			for _, id := range m.Removed {
				delete(tabs, id)
			}
			for _, t := range m.Tabs {
				for _, o := range st.BrowserOperations {
					if o.Profile == p.ID && o.Epoch == p.Epoch && o.Action == "open" && o.Status == "succeeded" && o.TabID == t.ID {
						t.OwnerID = o.ID
						break
					}
				}
				tabs[t.ID] = t
			}
			p.Tabs = []model.BrowserTab{}
			for _, t := range tabs {
				p.Tabs = append(p.Tabs, t)
			}
			sort.Slice(p.Tabs, func(i, j int) bool { return p.Tabs[i].ID < p.Tabs[j].ID })
			p.LastSequence = m.Sequence
			p.LastObservedAt, _ = time.Parse(time.RFC3339Nano, m.ObservedAt)
			p.Freshness = nil
			p.PresentTabs = nil
			p.Markers = nil
			p.ReceivedAt = now.UTC()
			p.ReceivedEpoch = s.Store.RuntimeID()
			p.FocusedWindow = *m.FocusedWindow
			p.Complete = *m.Complete
			if !m.Delta && (prior.ReceivedAt.IsZero() || prior.ReceivedEpoch != s.Store.RuntimeID() || prior.InventorySnapshotAt == nil || now.Sub(*prior.InventorySnapshotAt) >= 24*time.Hour) {
				snapshotAt := now.UTC()
				p.InventorySnapshotAt = &snapshotAt
				change.Events = append(change.Events, store.Pending{Subject: "browser", Verb: "inventory_observed", EntityID: p.ID, Payload: p})
			} else {
				delta := model.BrowserInventoryDelta{Profile: p, BaseSequence: prior.LastSequence, Removed: []int{}}
				delta.Profile.Tabs = []model.BrowserTab{}
				before := map[int]model.BrowserTab{}
				after := map[int]bool{}
				for _, tab := range prior.Tabs {
					before[tab.ID] = tab
				}
				for _, tab := range p.Tabs {
					after[tab.ID] = true
					if old, ok := before[tab.ID]; !ok || !reflect.DeepEqual(old, tab) {
						delta.Profile.Tabs = append(delta.Profile.Tabs, tab)
					}
				}
				for _, tab := range prior.Tabs {
					if !after[tab.ID] {
						delta.Removed = append(delta.Removed, tab.ID)
					}
				}
				change.Events = append(change.Events, store.Pending{Subject: "browser", Verb: "inventory_delta", EntityID: p.ID, Payload: delta})
			}
			for _, span := range m.FocusSpans {
				identity, err := surface.Identify(span.Pointer)
				if err != nil {
					return change, fmt.Errorf("focus span has unresolved content identity")
				}
				f := model.SurfaceFocusSpan{BrowserFocusSpan: span, Profile: p.ID, Epoch: p.Epoch, Connection: p.Connection, Sequence: p.LastSequence, SurfaceID: identity.ID}
				change.Events = append(change.Events, store.Pending{Subject: "surface", Verb: "focused", EntityID: f.SurfaceID, Payload: f})
			}
			change.Events = append(change.Events, surfaceEvents(st, p, m)...)
			reply.LastSequence = p.LastSequence
		case "readback":
			var err error
			change.Events, err = s.readback(st, m, now)
			if err != nil {
				return change, err
			}
			reply.LastSequence = m.Sequence
		case "poll":
			reply.Type = "commands"
			if c, event := s.challenge(st, p, now); c != nil {
				reply.Challenge = c
				if event != nil {
					change.Events = []store.Pending{*event}
				}
				change.Result = reply
				return change, nil
			}
			var continuationEvents []store.Pending
			reply.Continuations, continuationEvents = s.continuations(st, m, now)
			change.Events = append(change.Events, continuationEvents...)
			keys := []string{}
			for id := range st.BrowserOperations {
				keys = append(keys, id)
			}
			sort.Strings(keys)
			for _, id := range keys {
				o := st.BrowserOperations[id]
				if o.Profile != p.ID || o.Status != "pending" {
					continue
				}
				if o.ActionRef != nil {
					a := st.Actions[o.ActionRef.ID]
					if a.Execution == "dispatching" && (o.Epoch != p.Epoch || !now.Before(o.ExpiresAt)) {
						v := actions.Transition(a, "interrupt", "delivery deadline or browser epoch changed", "coordinator", now)
						change.Events = append(change.Events, actions.Pending(v))
						continue
					}
					if a.Execution != "queued" {
						continue
					}
					if a.CancelRequested || o.Epoch != p.Epoch || !now.Before(o.ExpiresAt) || !model.ActionInputsCurrent(st, a.Intent) || !model.ActionBrowserCurrent(st, a.Intent) {
						v := actions.Transition(a, "refuse", "authority, inputs, epoch or deadline changed before dispatch", "coordinator", now)
						change.Events = append(change.Events, actions.Pending(v))
						o.Status = "refused"
						o.Detail = v.Reason
						change.Events = append(change.Events, store.Pending{Subject: "browser", Verb: "command_finished", EntityID: o.ID, Payload: o})
						continue
					}
					if !s.fresh(p, a.LastEventID, now) {
						continue
					}
					if len(reply.Commands) < 8 {
						v := actions.Transition(a, "dispatch", "fresh paired browser transport delivery", "coordinator", now)
						if s.Runtime != nil {
							v.Version = 2
						}
						v.DeliveryID = m.ID
						v.Observation = &model.ActionObservation{ID: model.NewID(), Status: "unknown", SourceEpoch: p.Epoch, Digest: model.ContentDigest(p), ObservedAt: p.ReceivedAt, Detail: "Complete browser inventory used as dispatch precondition"}
						change.Events = append(change.Events, actions.Pending(v))
						reply.Commands = append(reply.Commands, o)
					}
					continue
				}
				if o.Epoch != p.Epoch || !now.Before(o.ExpiresAt) {
					o.Status = "expired"
					o.Detail = "epoch changed or deadline passed"
					change.Events = append(change.Events, store.Pending{Subject: "browser", Verb: "command_finished", EntityID: o.ID, Payload: o})
					continue
				}
				pairingActive := false
				for _, a := range st.Actions {
					if a.Intent.Browser != nil && a.Intent.Browser.Profile == p.ID && a.Intent.Browser.Pairing != nil && model.ActionHolds(a) {
						pairingActive = true
					}
				}
				if pairingActive {
					continue
				}
				if len(reply.Commands) < 8 {
					reply.Commands = append(reply.Commands, o)
				}
			}
		case "pairing_ready":
			a, ok := st.Actions[m.PairReady.ActionRef.ID]
			if !ok || a.Intent.Browser == nil || a.Intent.Browser.Profile != p.ID || a.Intent.Browser.Epoch != p.Epoch || !reflect.DeepEqual(&m.PairReady.ActionRef, a.BrowserRef()) {
				return change, fmt.Errorf("pairing report scope differs from issued attempt")
			}
			if a.Pairing != nil && reflect.DeepEqual(a.Pairing.Ready, m.PairReady) {
				change.Result = reply
				return change, nil
			}
			v := actions.Transition(a, "pair_ready", "Temporary pairing page created; native association pending", actor, now)
			v.Version = 3
			v.PairReady = m.PairReady
			change.Events = append(change.Events, actions.Pending(v))
		case "command_result":
			r := m.Result
			o, ok := st.BrowserOperations[r.OperationID]
			if !ok || o.Profile != p.ID || o.Epoch != p.Epoch {
				return change, fmt.Errorf("result does not belong to this browser epoch")
			}
			if !reflect.DeepEqual(r.ActionRef, o.ActionRef) {
				return change, fmt.Errorf("result action/attempt reference differs from issued command")
			}
			if o.ActionRef != nil {
				v := actions.Transition(st.Actions[o.ActionRef.ID], "report", "browser API report; postcondition unverified", actor, now)
				if s.Runtime != nil {
					v.Version = 2
				}
				if st.Actions[o.ActionRef.ID].Intent.Browser.Pairing != nil {
					v.Version = 3
					v.ContinuationID = r.ContinuationID
					a := st.Actions[o.ActionRef.ID]
					if r.ContinuationID != "" && (a.Pairing == nil || a.Pairing.ContinuationDeliveryID == "" || a.Pairing.ContinuationID != r.ContinuationID) {
						return change, fmt.Errorf("result continuation differs from issued delivery")
					}
					if r.ContinuationID == "" && a.Pairing != nil {
						return change, fmt.Errorf("first pairing phase already reported")
					}
				} else if r.ContinuationID != "" {
					return change, fmt.Errorf("continuation on an ordinary browser result")
				}
				v.Report = &model.ActionReport{Status: r.Status, Detail: r.Detail, TabID: r.TabID, WindowID: r.WindowID, URL: r.URL}

				if reflect.DeepEqual(st.Actions[o.ActionRef.ID].Report, v.Report) {
					change.Result = reply
					return change, nil
				}
				change.Events = append(change.Events, actions.Pending(v))
			}
			late := o.Status == "expired"
			if o.Status != "pending" && !late && !(o.ActionRef != nil && model.Contains([]string{"cancelled", "uncertain", "failed"}, o.Status)) {
				return change, fmt.Errorf("operation already finalized")
			}
			if r.Status == "succeeded" && o.Action == "open" {
				if r.TabID < 1 || r.WindowID < 1 || !ValidURL(r.URL) {
					return change, fmt.Errorf("open result missing valid instance")
				}
				o.TabID = r.TabID
				o.WindowID = r.WindowID
				o.ExpectedURL = r.URL
			}
			o.Status = r.Status
			o.Detail = r.Detail
			if late {
				o.Detail = "late result after deadline: " + r.Detail
			}
			change.Events = append(change.Events, store.Pending{Subject: "browser", Verb: "command_finished", EntityID: o.ID, Payload: o})
		}
		change.Result = reply
		return change, nil
	})
}
func (s Service) Control(ctx context.Context, c Control, now time.Time) (json.RawMessage, error) {
	if !IDPattern.MatchString(c.ID) || !IDPattern.MatchString(c.Profile) {
		return nil, fmt.Errorf("invalid command/profile identity")
	}
	request, _ := json.Marshal(c)
	return s.Store.Transact(ctx, "browser-control-"+c.ID, "cli", request, now, func(st model.State) (store.Change, error) {
		change := store.Change{Revision: st.Revision}
		p, ok := st.Browsers[c.Profile]
		if !ok {
			return change, fmt.Errorf("unknown browser profile; connect extension first")
		}
		if c.Action == "pair" || c.Action == "unpair" {
			if p.Paired != (c.Action == "pair") {
				p.InventorySnapshotAt = nil
			}
			p.Paired = c.Action == "pair"
			change.Events = []store.Pending{{Subject: "browser", Verb: "pairing_changed", EntityID: p.ID, Payload: p}}
			if !p.Paired {
				keys := []string{}
				for id := range st.BrowserOperations {
					keys = append(keys, id)
				}
				sort.Strings(keys)
				for _, id := range keys {

					o := st.BrowserOperations[id]
					if o.Profile == p.ID && o.Status != "pending" && o.ActionRef != nil {
						a := st.Actions[o.ActionRef.ID]
						if model.ActionHolds(a) && model.Contains([]string{"api_reported", "uncertain"}, a.Execution) {
							v := actions.Transition(a, "source_lost", "Profile unpaired; outcome readback unavailable", "cli", now)
							v.Version = 2
							change.Events = append(change.Events, actions.Pending(v))
						}
					}
					if o.Profile == p.ID && o.Status == "pending" {
						if o.ActionRef != nil {
							a := st.Actions[o.ActionRef.ID]
							if model.ActionHolds(a) && !a.CancelRequested {
								change.Events = append(change.Events, actions.Pending(actions.Transition(a, "cancel", "profile unpaired", "cli", now)))
							}
						}
						o.Status = "cancelled"
						o.Detail = "profile unpaired"
						change.Events = append(change.Events, store.Pending{Subject: "browser", Verb: "command_finished", EntityID: o.ID, Payload: o})
					}
				}
			}
			change.Result = map[string]any{"profile": p.ID, "paired": p.Paired}
			return change, nil
		}
		if !p.Paired || c.Epoch != p.Epoch {
			return change, fmt.Errorf("unpaired profile or stale epoch")
		}
		for _, a := range st.Actions {
			if a.Intent.Browser != nil && a.Intent.Browser.Profile == p.ID && a.Intent.Browser.Pairing != nil && model.ActionHolds(a) {
				return change, fmt.Errorf("profile has an unresolved pairing action")
			}
		}
		if _, exists := st.BrowserOperations[c.ID]; exists {
			return change, fmt.Errorf("browser operation ID already used: %w", store.ErrConflict)
		}
		if !model.Contains([]string{"open", "navigate", "focus", "move", "close"}, c.Action) {
			return change, fmt.Errorf("unsupported browser action")
		}
		o := model.BrowserOperation{ID: c.ID, Profile: p.ID, Epoch: p.Epoch, Action: c.Action, TabID: c.TabID, WindowID: c.WindowID, ExpectedURL: c.ExpectedURL, URL: c.URL, Status: "pending", CreatedAt: now.UTC(), ExpiresAt: now.Add(30 * time.Second).UTC()}
		if (c.Action == "open" || c.Action == "navigate") && !ValidURL(c.URL) {
			return change, fmt.Errorf("only credential-free http/https URLs are supported")
		}
		if c.Action != "open" {
			found := false
			for _, t := range p.Tabs {
				if t.ID == c.TabID && t.URL == c.ExpectedURL && t.OwnerID != "" {
					if _, scoped := st.Actions[t.OwnerID]; scoped {
						return change, fmt.Errorf("task-owned browser tab requires the shared action API")
					}
					found = true
					o.OwnerID = t.OwnerID
				}
			}
			if !found {
				return change, fmt.Errorf("target is not a current owned tab at the expected URL")
			}
			if c.Action == "move" && c.WindowID < 1 {
				return change, fmt.Errorf("destination window required")
			}
		}
		change.Events = []store.Pending{{Subject: "browser", Verb: "command_queued", EntityID: o.ID, Payload: o}}
		change.Result = o
		return change, nil
	})
}
