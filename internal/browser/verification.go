package browser

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/actions"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"slices"
	"sync"
	"time"
)

// Runtime has no replayable authority. time.Time carries monotonic elapsed time
// in production; replacing a daemon/connection discards every freshness lease.
type Runtime struct {
	mu         sync.Mutex
	Clock      func() time.Time
	challenges map[string]runtimeLease
	reads      map[string]runtimeLease
}
type runtimeLease struct {
	ID string
	At time.Time
}

func NewService(s *store.Store) *Service {
	return &Service{Store: s, Runtime: &Runtime{Clock: time.Now, challenges: map[string]runtimeLease{}, reads: map[string]runtimeLease{}}}
}
func (s Service) Handle(ctx context.Context, m Message, now time.Time) (json.RawMessage, error) {
	if s.Runtime == nil {
		return nil, fmt.Errorf("browser runtime required; construct with NewService")
	}
	rt := s.Runtime
	rt.mu.Lock()
	defer rt.mu.Unlock()
	started := rt.Clock()
	raw, err := s.handle(ctx, m, now)
	if err != nil {
		return nil, err
	}
	if m.Type == "hello" {
		delete(rt.challenges, m.Profile)
		delete(rt.reads, m.Profile)
	}
	var reply Reply
	if err = json.Unmarshal(raw, &reply); err != nil {
		return nil, err
	}
	current, err := s.Store.State(ctx)
	if err != nil {
		return nil, err
	}
	p := current.Browsers[m.Profile]
	if c := reply.Challenge; c != nil && p.Challenge != nil && p.Challenge.ID == c.ID {
		if len(rt.challenges) >= 128 {
			for key, v := range rt.challenges {
				if rt.Clock().Sub(v.At) >= 5*time.Second {
					delete(rt.challenges, key)
					delete(rt.reads, key)
				}
			}
		}
		if rt.challenges[m.Profile].ID != c.ID && len(rt.challenges) < 128 {
			rt.challenges[m.Profile] = runtimeLease{c.ID, started}
		}
	}
	if m.Type == "readback" && p.Freshness != nil && p.Freshness.Challenge.ID == m.ChallengeID && p.LastSequence == m.Sequence && rt.reads[m.Profile].ID != m.ChallengeID {
		rt.reads[m.Profile] = runtimeLease{m.ChallengeID, started}
	}
	return raw, nil
}
func (s Service) fresh(p model.BrowserProfile, after int64, now time.Time) bool {
	if s.Runtime == nil {
		return false
	}
	f := p.Freshness
	if f == nil || !f.Stable || !p.Complete || p.VerificationProtocol != 1 || f.Sequence != p.LastSequence || f.Challenge.AfterEventID < after || f.Challenge.RuntimeID != s.Store.RuntimeID() || f.Challenge.Connection != p.Connection || now.Before(p.ReceivedAt) || now.Sub(p.ReceivedAt) > 5*time.Second {
		return false
	}
	lease := s.Runtime.reads[p.ID]
	elapsed := s.Runtime.Clock().Sub(lease.At)
	return lease.ID == f.Challenge.ID && elapsed >= 0 && elapsed <= 5*time.Second
}
func (s Service) readbackAllowed(p model.BrowserProfile, m Message, now time.Time) error {
	if m.Type == "readback" && p.PairingProtocol == 1 && m.EventGeneration == nil {
		return fmt.Errorf("pairing readback requires event generation")
	}
	if s.Runtime == nil || p.VerificationProtocol != 1 || p.Challenge == nil || p.Challenge.ID != m.ChallengeID || p.Challenge.RuntimeID != s.Store.RuntimeID() || now.Before(p.Challenge.IssuedAt) || !now.Before(p.Challenge.ExpiresAt) {
		return fmt.Errorf("stale_challenge: request a fresh browser read")
	}
	lease := s.Runtime.challenges[p.ID]
	elapsed := s.Runtime.Clock().Sub(lease.At)
	if lease.ID != m.ChallengeID || elapsed < 0 || elapsed >= 5*time.Second {
		return fmt.Errorf("stale_challenge: monotonic deadline or daemon changed")
	}
	return nil
}
func (s Service) challenge(st model.State, p model.BrowserProfile, now time.Time) (*model.BrowserChallenge, *store.Pending) {
	if s.Runtime == nil || p.VerificationProtocol != 1 {
		return nil, nil
	}
	required := int64(0)
	refs := []model.BrowserActionRef{}
	for _, a := range st.Actions {
		if a.Intent.Browser != nil && a.Intent.Browser.Profile == p.ID && a.Intent.Browser.Epoch == p.Epoch && model.ActionHolds(a) && a.VerificationAttempts < 8 {
			if a.LastEventID > required {
				required = a.LastEventID
			}
			refs = append(refs, *a.BrowserRef())
		}
	}
	if required == 0 || s.fresh(p, required, now) {
		return nil, nil
	}
	slices.SortFunc(refs, func(a, b model.BrowserActionRef) int {
		if a.ID < b.ID {
			return -1
		}
		if a.ID > b.ID {
			return 1
		}
		return 0
	})
	if c := p.Challenge; c != nil && c.AfterEventID >= required && s.readbackAllowed(p, Message{ChallengeID: c.ID}, now) == nil {
		return c, nil
	}
	c := &model.BrowserChallenge{Version: 1, ID: model.NewID(), Profile: p.ID, Epoch: p.Epoch, Connection: p.Connection, RuntimeID: s.Store.RuntimeID(), AfterEventID: st.LastEventID, IssuedAt: now.UTC(), ExpiresAt: now.UTC().Add(5 * time.Second), Actions: refs}
	return c, &store.Pending{Subject: "browser", Verb: "challenge_issued", EntityID: p.ID, Payload: c}
}
func (s Service) readback(st model.State, m Message, now time.Time) ([]store.Pending, error) {
	observed, _ := time.Parse(time.RFC3339Nano, m.ObservedAt)
	r := model.BrowserReadback{Version: 1, Profile: m.Profile, ChallengeID: m.ChallengeID, Sequence: m.Sequence, ObservedAt: observed, ReceivedAt: now.UTC(), Complete: *m.Complete, Stable: *m.Stable, Tabs: m.Tabs, PresentTabs: m.PresentTabs, Instances: m.Instances, FocusedWindow: *m.FocusedWindow}
	if m.EventGeneration != nil {
		r.EventGeneration = *m.EventGeneration
	}
	r.Markers = m.Markers
	pending := store.Pending{Subject: "browser", Verb: "readback_observed", EntityID: m.Profile, Payload: r}
	if err := actions.ApplyPending(&st, pending, "browser-"+m.Profile+"-"+m.ID, "observer:browser", now); err != nil {
		return nil, err
	}
	events := []store.Pending{pending}
	p := st.Browsers[m.Profile]
	ids := []string{}
	for id, a := range st.Actions {
		if a.Intent.Browser != nil && a.Intent.Browser.Profile == p.ID && a.Intent.Browser.Epoch == p.Epoch && a.Execution != "queued" && model.ActionHolds(a) && a.VerificationAttempts < 8 {
			ids = append(ids, id)
		}
	}
	slices.Sort(ids)
	for _, id := range ids {

		a := st.Actions[id]
		if a.Intent.Browser.Pairing != nil && (a.Pairing == nil || a.Pairing.ContinuationDeliveryID == "") {
			if a.Pairing != nil && a.Pairing.Ready != nil && p.Complete && p.Freshness.Stable && p.Freshness.Challenge.AfterEventID >= a.LastEventID {
				present := false
				for _, tabID := range p.PresentTabs {
					if tabID == a.Pairing.Ready.MarkerTabID {
						present = true
					}
				}
				if !present {
					v := actions.Transition(a, "pair_abandoned", "Temporary marker disappeared before any continuation was dispatched", "observer:browser", now)
					v.Version = 3
					events = append(events, actions.Pending(v))
					continue
				}
			}
			if now.Before(a.Intent.ExpiresAt) {
				continue
			}
		}
		status, detail := model.BrowserOutcomeInState(st, a, p)
		v := actions.Transition(a, "verify", "Independent challenged browser readback", "observer:browser", now)
		v.Version = 2
		v.Observation = &model.ActionObservation{ID: model.NewID(), Status: status, SourceEpoch: p.Epoch, Digest: model.ContentDigest(p), ObservedAt: p.ReceivedAt, Detail: detail}
		events = append(events, actions.Pending(v))
	}
	return events, nil
}
