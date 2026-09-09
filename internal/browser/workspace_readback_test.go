package browser

import (
	"context"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"reflect"
	"testing"
	"time"
)

func TestWorkspaceObservationChallengesSettledActionsWithoutDispatch(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewService(db)
	now := time.Now().UTC()
	clock := now
	s.Runtime.Clock = func() time.Time { return clock }
	st := model.Empty()
	st.LastEventID = 10
	p := model.BrowserProfile{ID: model.NewID(), Epoch: model.NewID(), Connection: model.NewID(), Paired: true, VerificationProtocol: 1}
	a := model.ActionRecord{Intent: model.ActionIntent{ID: model.NewID(), Target: "alpha", SurfaceID: model.NewID(), Browser: &model.BrowserIntent{Profile: p.ID, Epoch: p.Epoch, Action: "open"}}, Execution: "api_reported", Verification: "matched", LastEventID: 9}
	st.Actions[a.Intent.ID] = a
	if c, e := s.challenge(st, p, now); c != nil || e != nil {
		t.Fatal("idle settled surface requested observations")
	}
	s.Runtime.observations = map[string]observationDemand{p.ID: {After: 10, Epoch: p.Epoch, Connection: p.Connection, Refs: []model.BrowserActionRef{*a.BrowserRef()}, Started: now}}
	before := model.Clone(st)
	c, e := s.challenge(st, p, now)
	if c == nil || e == nil || len(c.Actions) != 1 || c.Actions[0].ID != a.Intent.ID || c.AfterEventID != 10 || !reflect.DeepEqual(before, st) {
		t.Fatal(c, e)
	}
	// The demand emits the existing readback challenge, never a second action.
	if e.Subject != "browser" || e.Verb != "challenge_issued" {
		t.Fatal(e)
	}
	clock = clock.Add(5 * time.Second)
	if c, e := s.challenge(st, p, now); c != nil || e != nil {
		t.Fatal("expired observation demand survived")
	}
	if len(s.Runtime.observations) != 0 {
		t.Fatal("expired demand retained")
	}
	state, _ := db.State(context.Background())
	if len(state.Actions) != 0 {
		t.Fatal("observation created durable actions")
	}
}

func TestRecoveryBrowserRuntimeLeaseRejectsRestartAndClockJump(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewService(db)
	now := time.Now().UTC()
	clock := now
	s.Runtime.Clock = func() time.Time { return clock }
	id := model.NewID()
	p := model.BrowserProfile{ID: model.NewID(), Connection: model.NewID(), VerificationProtocol: 1, Complete: true, LastSequence: 1, ReceivedAt: now}
	p.Freshness = &model.BrowserFreshness{Stable: true, Sequence: 1, Challenge: model.BrowserChallenge{ID: id, Connection: p.Connection, RuntimeID: db.RuntimeID(), AfterEventID: 10}}
	s.Runtime.reads[p.ID] = runtimeLease{ID: id, At: now}
	// The supplied command time precedes demand registration/readback by a second.
	// A fresh received timestamp must not be rejected as being in the future.
	supplied := now.Add(-time.Second)
	if !s.RecoveryFresh(p, 10, s.observationTime(supplied)) {
		t.Fatal("fresh readback rejected after pre-demand delay")
	}
	if s.RecoveryFresh(p, 10, supplied) {
		t.Fatal("future observation guard weakened")
	}
	if !s.RecoveryFresh(p, 10, now) {
		t.Fatal("live lease refused")
	}
	if NewService(db).RecoveryFresh(p, 10, now) {
		t.Fatal("fresh runtime accepted replayed lease")
	}
	clock = now.Add(6 * time.Second)
	if s.RecoveryFresh(p, 10, now) {
		t.Fatal("wall-clock rollback extended monotonic lease")
	}
	clock = now.Add(-time.Second)
	if s.RecoveryFresh(p, 10, now) {
		t.Fatal("negative elapsed lease accepted")
	}
}
