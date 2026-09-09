package browser

import (
	"context"
	"encoding/json"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"reflect"
	"testing"
	"time"
)

func TestInventoryDeltasReplayBoundariesAndVolume(t *testing.T) {
	dir := t.TempDir()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { db.Close() }()
	s := NewService(db)
	ctx := context.Background()
	now := time.Now().UTC()
	profile, epoch, connection := model.NewID(), model.NewID(), model.NewID()
	message := func(kind string) Message {
		return Message{V: 1, Type: kind, ID: model.NewID(), Profile: profile, Epoch: epoch, Connection: connection}
	}
	send := func(m Message) {
		t.Helper()
		if _, err := s.Handle(ctx, m, now); err != nil {
			t.Fatal(err)
		}
	}
	hello := message("hello")
	hello.ExtensionVersion = "0.6.0"
	send(hello)
	if _, err = s.Control(ctx, Control{ID: model.NewID(), Action: "pair", Profile: profile}, now); err != nil {
		t.Fatal(err)
	}
	complete, focus := true, 1
	inv := message("inventory")
	inv.Sequence = 1
	inv.ObservedAt = now.Format(time.RFC3339Nano)
	inv.Complete = &complete
	inv.FocusedWindow = &focus
	for id := 1; id <= 100; id++ {
		inv.Tabs = append(inv.Tabs, model.BrowserTab{ID: id, WindowID: 1, URL: "https://example.test/", Title: "ordinary public tab"})
	}
	send(inv)
	delta := inv
	delta.Delta = true
	delta.Tabs = []model.BrowserTab{inv.Tabs[0]}
	delta.Removed = []int{100}
	for seq := int64(2); seq <= 302; seq++ {
		delta.ID = model.NewID()
		delta.BaseSequence = seq - 1
		delta.Sequence = seq
		delta.Tabs[0].Title = delta.ID
		send(delta)
		delta.Removed = nil
	}
	st, _ := db.State(ctx)
	if len(st.Browsers[profile].Tabs) != 99 {
		t.Fatal("closed tab retained")
	}
	replayed, err := db.Replay(ctx)
	if err != nil || !reflect.DeepEqual(st, replayed) {
		t.Fatal("delta replay drift", err)
	}
	events, _ := db.Events(ctx)
	snapshots, bytes := 0, 0
	for _, e := range events {
		b, _ := json.Marshal(e)
		bytes += len(b)
		if e.Subject == "browser" {
			if e.Verb == "inventory_observed" {
				snapshots++
			}
		}
	}
	if snapshots != 1 || bytes >= 1000000 {
		t.Fatal("synthetic 100-tab/300-change volume", snapshots, bytes)
	}
	t.Logf("synthetic browser event bytes: %d", bytes)
	delta.ID = model.NewID()
	delta.Sequence++
	delta.BaseSequence = 1
	if _, err = s.Handle(ctx, delta, now); err == nil {
		t.Fatal("gap accepted")
	}
	now = now.Add(24 * time.Hour)
	delta.ID = model.NewID()
	delta.BaseSequence = 302
	if _, err = s.Handle(ctx, delta, now); err == nil {
		t.Fatal("daily snapshot omitted")
	}
	inv.ID = model.NewID()
	inv.Sequence = 303
	send(inv)
	// A worker reconnect requires another bounding snapshot, even in the same epoch.
	connection = model.NewID()
	hello = message("hello")
	hello.ExtensionVersion = "0.6.0"
	send(hello)
	delta.Connection = connection
	delta.ID = model.NewID()
	delta.BaseSequence = 303
	delta.Sequence = 304
	if _, err = s.Handle(ctx, delta, now); err == nil {
		t.Fatal("reconnect delta without snapshot")
	}
	inv.Connection = connection
	inv.ID = model.NewID()
	inv.Sequence = 304
	send(inv)
	// A daemon restart also requires a snapshot, even if the sender has not
	// replaced its transport yet. Persisted receive timestamps are not a lease.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s = NewService(db)
	delta.ID = model.NewID()
	delta.BaseSequence = 304
	delta.Sequence = 305
	if _, err = s.Handle(ctx, delta, now); err == nil {
		t.Fatal("restart accepted delta from old runtime")
	}
	inv.ID = model.NewID()
	inv.Sequence = 305
	send(inv)
	// An epoch changes container identity; no old delta is applicable.
	epoch = model.NewID()
	hello = message("hello")
	hello.ExtensionVersion = "0.6.0"
	send(hello)
	delta.ID = model.NewID()
	if _, err = s.Handle(ctx, delta, now); err == nil {
		t.Fatal("old epoch delta accepted")
	}
}
