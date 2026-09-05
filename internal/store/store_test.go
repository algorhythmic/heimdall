package store

import (
	"context"
	"encoding/json"
	"heimdall/internal/model"
	"testing"
	"time"
)

func TestAtomicReplayAndRetry(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if other, err := Open(dir); err == nil {
		other.Close()
		t.Fatal("second writer allowed")
	}
	now := time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
	build := func(st model.State) (Change, error) {
		return Change{Revision: 1, Events: []Pending{{"capture", "created", "cap", model.Capture{ID: "cap", Targets: []string{"unassigned"}, CreatedAt: now}}}, Result: map[string]string{"id": "cap"}}, nil
	}
	r, err := s.Transact(ctx, "cmd", "cli", []byte("same"), now, build)
	if err != nil {
		t.Fatal(err)
	}
	retry, err := s.Transact(ctx, "cmd", "cli", []byte("same"), now.Add(time.Hour), func(model.State) (Change, error) { t.Fatal("retry rebuilt"); return Change{}, nil })
	if err != nil || string(r) != string(retry) {
		t.Fatal(err)
	}
	if _, err = s.Transact(ctx, "cmd", "cli", []byte("different"), now, build); err != ErrConflict {
		t.Fatal(err)
	}
	before, _ := s.State(ctx)
	bb, _ := json.Marshal(before)
	after, err := s.Replay(ctx)
	ab, _ := json.Marshal(after)
	if err != nil || string(bb) != string(ab) {
		t.Fatalf("replay mismatch %s %s %v", bb, ab, err)
	}
	_, err = s.Transact(ctx, "bad", "cli", nil, now, func(model.State) (Change, error) { return Change{Events: []Pending{{"unknown", "verb", "", nil}}}, nil })
	if err == nil {
		t.Fatal("unknown event accepted")
	}
	events, _ := s.Events(ctx)
	if len(events) != 2 {
		t.Fatal("failed transaction leaked", len(events))
	}
	// Corrupt a version to simulate a log from a newer release. Failed replay retains projections.
	_, err = s.db.Exec("UPDATE events SET event_version=99 WHERE id=2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.Replay(ctx); err == nil {
		t.Fatal("unknown version replayed")
	}
	unchanged, _ := s.State(ctx)
	ub, _ := json.Marshal(unchanged)
	if string(ub) != string(bb) {
		t.Fatal("failed replay changed state")
	}
}
