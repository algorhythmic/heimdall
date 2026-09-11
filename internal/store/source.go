package store

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/conversation"
	"heimdall/internal/model"
	"strings"
	"time"
)

const sourceActor = "observer:session"

func sourceCommand(verb string, body any) string {
	raw, _ := json.Marshal(body)
	return "source-" + conversation.Digest([]byte(conversation.Identity("heimdall-source-event", verb, string(raw))))
}

type sourceCheckpoint struct {
	Version    int                     `json:"version"`
	SourceID   string                  `json:"source_id"`
	Checkpoint conversation.Checkpoint `json:"checkpoint"`
	At         time.Time               `json:"at"`
}

func validSourceID(s string) bool {
	return strings.HasPrefix(s, "sr1:heimdall-source:") && conversation.ValidDigest(strings.TrimPrefix(s, "sr1:heimdall-source:"))
}

func (c sourceCheckpoint) Validate() error {
	if c.Version != 1 || !validSourceID(c.SourceID) || c.At.IsZero() {
		return fmt.Errorf("invalid source checkpoint event")
	}
	return c.Checkpoint.Validate()
}

type sourceLost struct {
	Version  int       `json:"version"`
	SourceID string    `json:"source_id"`
	Reason   string    `json:"reason"`
	At       time.Time `json:"at"`
}

func (l sourceLost) Validate() error {
	if l.Version != 1 || !validSourceID(l.SourceID) || len(l.Reason) < 1 || len(l.Reason) > 256 || l.At.IsZero() {
		return fmt.Errorf("invalid source loss event")
	}
	return nil
}

// A configured root requires explicit CLI authority. Stream registration,
// checkpoint advance, gaps and loss are observer facts under a live root.
// SetSourceRootActive journals an explicit activation toggle. Deactivation
// stops polling; the root's streams and checkpoints are retained.
func (s *Store) SetSourceRootActive(ctx context.Context, key string, active bool, now time.Time) (json.RawMessage, error) {
	verb := map[bool]string{true: "activated", false: "deactivated"}[active]
	id := "source-" + conversation.Digest([]byte(conversation.Identity("heimdall-source-root", key, verb, now.UTC().Format(time.RFC3339Nano))))
	return s.Transact(ctx, id, "cli", nil, now, func(st model.State) (Change, error) {
		p, ok := st.SourceRoots[key]
		if !ok {
			return Change{}, fmt.Errorf("unknown source root %s", key)
		}
		if p.Active == active {
			return Change{}, fmt.Errorf("source root already %s", verb)
		}
		p.Active = active
		return Change{Revision: st.Revision, Events: []Pending{{"source", verb, key, p}}}, nil
	})
}

func applySource(st *model.State, e Event) error {
	switch e.Verb {
	case "configured":
		var p conversation.SourceRoot
		if err := model.StrictJSON(e.Payload, &p); err != nil {
			return err
		}
		if err := p.Validate(); err != nil {
			return err
		}
		if e.Actor != "cli" || e.EntityID != p.ID || !p.Active || !strings.HasPrefix(e.CommandID, "source-") {
			return fmt.Errorf("invalid source root provenance")
		}
		if old, ok := st.SourceRoots[p.ID]; ok {
			if old.Provider != p.Provider || old.Root != p.Root || old.Namespace != p.Namespace {
				return fmt.Errorf("source root identity conflict")
			}
			if !old.Active {
				old.Active = true
				st.SourceRoots[p.ID] = old
			}
			return nil
		}
		st.SourceRoots[p.ID] = p
	case "activated", "deactivated":
		var p conversation.SourceRoot
		if err := model.StrictJSON(e.Payload, &p); err != nil {
			return err
		}
		if err := p.Validate(); err != nil {
			return err
		}
		old, ok := st.SourceRoots[p.ID]
		if !ok || old.Provider != p.Provider || old.Root != p.Root || old.Namespace != p.Namespace || old.Active == p.Active {
			return fmt.Errorf("invalid source root activation")
		}
		if e.Actor != "cli" || e.EntityID != p.ID || p.Active != (e.Verb == "activated") || !strings.HasPrefix(e.CommandID, "source-") {
			return fmt.Errorf("invalid source root activation provenance")
		}
		st.SourceRoots[p.ID] = p
	case "registered":
		var p conversation.Source
		if err := model.StrictJSON(e.Payload, &p); err != nil {
			return err
		}
		if err := p.Validate(); err != nil {
			return err
		}
		if e.Actor != sourceActor || e.EntityID != p.ID() || !p.Active || p.Lost != "" || p.Checkpoint.Offset != 0 || !strings.HasPrefix(e.CommandID, "source-") {
			return fmt.Errorf("invalid stream registration provenance")
		}
		root, ok := st.SourceRoots[p.RootID]
		if !ok || !root.Active || root.Provider != p.Provider || root.Namespace != p.Key.Namespace {
			return fmt.Errorf("stream registration requires a live configured root")
		}
		if old, ok := st.SessionSources[p.ID()]; ok {
			if old.Key != p.Key || old.NativeID != p.NativeID || old.Provider != p.Provider {
				return fmt.Errorf("stream identity conflict")
			}
			if old.RootID != p.RootID {
				// A root consolidation may re-register the same stream under a
				// different live root of the same provider and namespace.
				nr, ok := st.SourceRoots[p.RootID]
				or, ok2 := st.SourceRoots[old.RootID]
				if !ok || !ok2 || !nr.Active || nr.Provider != or.Provider || nr.Namespace != or.Namespace {
					return fmt.Errorf("stream identity conflict")
				}
				old.RootID = p.RootID
			}
			if old.Path != p.Path || old.RootID != p.RootID {
				old.Path = p.Path
				st.SessionSources[p.ID()] = old
			}
			return nil
		}
		st.SessionSources[p.ID()] = p
	case "checkpointed":
		var p sourceCheckpoint
		if err := model.StrictJSON(e.Payload, &p); err != nil {
			return err
		}
		if err := p.Validate(); err != nil {
			return err
		}
		old, ok := st.SessionSources[p.SourceID]
		if !ok || !old.Active || e.Actor != sourceActor || e.EntityID != p.SourceID || !strings.HasPrefix(e.CommandID, "source-") {
			return fmt.Errorf("invalid source checkpoint provenance")
		}
		c, prev := p.Checkpoint, old.Checkpoint
		if c.Epoch < prev.Epoch || (c.Epoch == prev.Epoch && (c.Ordinal < prev.Ordinal || c.Offset < prev.Offset)) || p.At.Before(old.RegisteredAt) {
			return fmt.Errorf("nonmonotonic source checkpoint")
		}
		old.Checkpoint = c
		st.SessionSources[p.SourceID] = old
	case "gap":
		var p conversation.Gap
		if err := model.StrictJSON(e.Payload, &p); err != nil {
			return err
		}
		if err := p.Validate(); err != nil {
			return err
		}
		src, ok := st.SessionSources[p.SourceID]
		if !ok || e.Actor != sourceActor || e.EntityID != p.SourceID || !strings.HasPrefix(e.CommandID, "source-") || p.At.Before(src.RegisteredAt) {
			return fmt.Errorf("invalid source gap provenance")
		}
		// Gaps are journaled events only; no projection field is required.
	case "lost":
		var p sourceLost
		if err := model.StrictJSON(e.Payload, &p); err != nil {
			return err
		}
		if err := p.Validate(); err != nil {
			return err
		}
		old, ok := st.SessionSources[p.SourceID]
		if !ok || !old.Active || e.Actor != sourceActor || e.EntityID != p.SourceID || !strings.HasPrefix(e.CommandID, "source-") {
			return fmt.Errorf("invalid source loss provenance")
		}
		old.Active, old.Lost = false, p.Reason
		st.SessionSources[p.SourceID] = old
	default:
		return fmt.Errorf("unsupported source verb")
	}
	return nil
}

// RegisterSourceRoot records an explicitly configured scan root.
func (s *Store) RegisterSourceRoot(ctx context.Context, r conversation.SourceRoot, now time.Time) (json.RawMessage, error) {
	r.Active = true
	if err := r.Validate(); err != nil {
		return nil, err
	}
	id := sourceCommand("configured", r)
	return s.Transact(ctx, id, "cli", deliveryRequest(r), now, func(st model.State) (Change, error) {
		// The canonical locator (provider + canonical root) is the dedupe
		// key: re-adding an existing root returns its identity rather than
		// splitting streams across duplicate roots.
		var match *conversation.SourceRoot
		for _, old := range st.SourceRoots {
			if old.Provider == r.Provider && old.Root == r.Root {
				// An active root always wins over an inactive duplicate.
				if match == nil || (old.Active && !match.Active) {
					old := old
					match = &old
				}
			}
		}
		if match != nil {
			if !match.Active {
				reactivated := *match
				reactivated.Active = true
				return Change{Revision: st.Revision, Events: []Pending{{"source", "activated", reactivated.ID, reactivated}}, Result: map[string]string{"source_root_id": reactivated.ID}}, nil
			}
			return Change{Revision: st.Revision, Result: map[string]string{"source_root_id": match.ID}}, nil
		}
		return Change{Revision: st.Revision, Events: []Pending{{"source", "configured", r.ID, r}}, Result: map[string]string{"source_root_id": r.ID}}, nil
	})
}

// RegisterStream persists a discovered stream identity before first capture.
// The command ID derives from the stream identity, so a poller rediscovering
// the same stream dedupes safely.
func (s *Store) RegisterStream(ctx context.Context, src conversation.Source, now time.Time) (json.RawMessage, error) {
	src.Checkpoint = conversation.Checkpoint{Version: 1}
	src.Active = true
	if err := src.Validate(); err != nil {
		return nil, err
	}
	id := sourceCommand("registered", src)
	return s.Transact(ctx, id, sourceActor, deliveryRequest(src), now, func(st model.State) (Change, error) {
		return Change{Revision: st.Revision, Events: []Pending{{"source", "registered", src.ID(), src}}}, nil
	})
}

// AdvanceCheckpoint commits a read position after its records are journaled.
func (s *Store) AdvanceCheckpoint(ctx context.Context, sourceID string, cp conversation.Checkpoint, now time.Time) (json.RawMessage, error) {
	p := sourceCheckpoint{Version: 1, SourceID: sourceID, Checkpoint: cp, At: now.UTC()}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	id := sourceCommand("checkpointed", p)
	return s.Transact(ctx, id, sourceActor, deliveryRequest(p), now, func(st model.State) (Change, error) {
		return Change{Revision: st.Revision, Events: []Pending{{"source", "checkpointed", sourceID, p}}}, nil
	})
}

// RecordGap journals one observed coverage gap for a stream.
func (s *Store) RecordGap(ctx context.Context, sourceID string, g conversation.Gap, now time.Time) (json.RawMessage, error) {
	g.SourceID = sourceID
	g.At = now.UTC()
	if err := g.Validate(); err != nil {
		return nil, err
	}
	id := sourceCommand("gap", g)
	return s.Transact(ctx, id, sourceActor, deliveryRequest(g), now, func(st model.State) (Change, error) {
		return Change{Revision: st.Revision, Events: []Pending{{"source", "gap", sourceID, g}}}, nil
	})
}

// MarkSourceLost records source disappearance as a coverage gap; it never
// deletes records or rewrites history.
func (s *Store) MarkSourceLost(ctx context.Context, sourceID, reason string, now time.Time) (json.RawMessage, error) {
	p := sourceLost{Version: 1, SourceID: sourceID, Reason: reason, At: now.UTC()}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	id := sourceCommand("lost", p)
	return s.Transact(ctx, id, sourceActor, deliveryRequest(p), now, func(st model.State) (Change, error) {
		return Change{Revision: st.Revision, Events: []Pending{{"source", "lost", sourceID, p}}}, nil
	})
}
