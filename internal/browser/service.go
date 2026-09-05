package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"sort"
	"time"
)

type Service struct{ Store *store.Store }

func (s Service) Handle(ctx context.Context, m Message, now time.Time) (json.RawMessage, error) {
	if err := m.Validate(); err != nil {
		return nil, err
	}
	request, _ := json.Marshal(m)
	actor := "observer:browser"
	if m.Type == "poll" {
		actor = "scheduler"
	}
	return s.Store.Transact(ctx, "browser-"+m.Profile+"-"+m.ID, actor, request, now, func(st model.State) (store.Change, error) {
		reply := Reply{V: 1, Type: "ack", ID: m.ID, Profile: m.Profile}
		change := store.Change{Revision: st.Revision}
		p, exists := st.Browsers[m.Profile]
		if m.Type == "hello" {
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
			reply.Type = "welcome"
			reply.Paired = p.Paired
			reply.LastSequence = p.LastSequence
			if !p.Paired {
				reply.Type = "pairing_required"
			}
			change.Events = []store.Pending{{Subject: "browser", Verb: "profile_seen", EntityID: p.ID, Payload: p}}
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
			if m.Sequence <= p.LastSequence {
				return change, fmt.Errorf("stale_sequence: a newer inventory already committed")
			}
			tabs := map[int]model.BrowserTab{}
			if !*m.Complete {
				for _, t := range p.Tabs {
					tabs[t.ID] = t
				}
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
			p.FocusedWindow = *m.FocusedWindow
			p.Complete = *m.Complete
			change.Events = append(change.Events, store.Pending{Subject: "browser", Verb: "inventory_observed", EntityID: p.ID, Payload: p})
			reply.LastSequence = p.LastSequence
		case "poll":
			reply.Type = "commands"
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
				if o.Epoch != p.Epoch || !now.Before(o.ExpiresAt) {
					o.Status = "expired"
					o.Detail = "epoch changed or deadline passed"
					change.Events = append(change.Events, store.Pending{Subject: "browser", Verb: "command_finished", EntityID: o.ID, Payload: o})
					continue
				}
				if len(reply.Commands) < 8 {
					reply.Commands = append(reply.Commands, o)
				}
			}
		case "command_result":
			r := m.Result
			o, ok := st.BrowserOperations[r.OperationID]
			if !ok || o.Profile != p.ID || o.Epoch != p.Epoch {
				return change, fmt.Errorf("result does not belong to this browser epoch")
			}
			late := o.Status == "expired"
			if o.Status != "pending" && !late {
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
					if o.Profile == p.ID && o.Status == "pending" {
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
