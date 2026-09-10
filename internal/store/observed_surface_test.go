package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"heimdall/internal/model"
	"heimdall/internal/surface"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func surfaceFixture() (model.State, Event, model.BrowserSurfaceObservation) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	profile, epoch, connection := model.NewID(), model.NewID(), model.NewID()
	identity, _ := surface.Identify("https://example.test/page/")
	o := model.BrowserSurfaceObservation{Version: 1, Profile: profile, Epoch: epoch, Sequence: 1,
		TabID: 1, WindowID: 1, SurfaceID: identity.ID, Pointer: identity.Pointer, Title: "Page", ObservedAt: now}
	st := model.Empty()
	st.Browsers[profile] = model.BrowserProfile{ID: profile, Epoch: epoch, Connection: connection, Paired: true, LastSequence: 1,
		LastObservedAt: now, ReceivedAt: now, Tabs: []model.BrowserTab{{ID: 1, WindowID: 1, URL: o.Pointer, Title: o.Title}}}
	payload, _ := json.Marshal(o)
	e := Event{ID: 1, Version: 1, TS: now, Subject: "surface", Verb: "observed", Actor: "observer:browser",
		EntityID: identity.ID, CommandID: "browser-" + profile + "-" + model.NewID(), Payload: payload}
	return st, e, o
}

func TestObservedSurfaceRejectsInvalidEvidence(t *testing.T) {
	for name, mutate := range map[string]func(*model.State, *Event, *model.BrowserSurfaceObservation){
		"actor":    func(s *model.State, e *Event, o *model.BrowserSurfaceObservation) { e.Actor = "cli" },
		"command":  func(s *model.State, e *Event, o *model.BrowserSurfaceObservation) { e.CommandID = "other" },
		"version":  func(s *model.State, e *Event, o *model.BrowserSurfaceObservation) { o.Version = 2 },
		"epoch":    func(s *model.State, e *Event, o *model.BrowserSurfaceObservation) { o.Epoch = model.NewID() },
		"sequence": func(s *model.State, e *Event, o *model.BrowserSurfaceObservation) { o.Sequence++ },
		"clock": func(s *model.State, e *Event, o *model.BrowserSurfaceObservation) {
			o.ObservedAt = o.ObservedAt.Add(time.Second)
		},
		"receipt_clock": func(s *model.State, e *Event, o *model.BrowserSurfaceObservation) { e.TS = e.TS.Add(time.Second) },
		"pointer":       func(s *model.State, e *Event, o *model.BrowserSurfaceObservation) { o.Pointer += "?x=1" },
		"identity": func(s *model.State, e *Event, o *model.BrowserSurfaceObservation) {
			o.SurfaceID = "000000000000"
			e.EntityID = o.SurfaceID
		},
		"title":  func(s *model.State, e *Event, o *model.BrowserSurfaceObservation) { o.Title = "forged" },
		"window": func(s *model.State, e *Event, o *model.BrowserSurfaceObservation) { o.WindowID = 2 },
		"gap":    func(s *model.State, e *Event, o *model.BrowserSurfaceObservation) { o.Gap = "invalid_pointer" },
		"unpaired": func(s *model.State, e *Event, o *model.BrowserSurfaceObservation) {
			p := s.Browsers[o.Profile]
			p.Paired = false
			s.Browsers[o.Profile] = p
		},
		"collision": func(s *model.State, e *Event, o *model.BrowserSurfaceObservation) {
			s.ObservedSurfaces[o.SurfaceID] = model.ObservedSurface{ID: o.SurfaceID, Kind: "tab", NormalizedPointer: "https://different.test"}
		},
		"unobserved_close":  func(s *model.State, e *Event, o *model.BrowserSurfaceObservation) { e.Verb = "closed" },
		"unobserved_change": func(s *model.State, e *Event, o *model.BrowserSurfaceObservation) { e.Verb = "changed" },
	} {
		t.Run(name, func(t *testing.T) {
			st, e, o := surfaceFixture()
			mutate(&st, &e, &o)
			e.Payload, _ = json.Marshal(o)
			before := model.Clone(st)
			if err := Apply(&st, e); err == nil {
				t.Fatal("invalid surface evidence accepted")
			}
			if !reflect.DeepEqual(st, before) {
				t.Fatal("rejected surface event mutated state")
			}
		})
	}
	st, e, _ := surfaceFixture()
	if err := Apply(&st, e); err != nil {
		t.Fatal(err)
	}
	if err := Apply(&st, e); err == nil {
		t.Fatal("duplicate surface event accepted")
	}
	var fields map[string]any
	json.Unmarshal(e.Payload, &fields)
	fields["task"] = "unauthorized-binding"
	e.Payload, _ = json.Marshal(fields)
	if err := Apply(&st, e); err == nil {
		t.Fatal("unknown authority-bearing field accepted")
	}
}

func TestSurfaceTransactionRollbackAndSchema21Upgrade(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	db, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	fixture, e, o := surfaceFixture()
	profile := fixture.Browsers[o.Profile]
	// Legacy inventory events have no surface events. Migration/replay must not
	// retroactively invent the new observations from those old snapshots.
	_, err = db.Transact(ctx, "legacy-browser", "observer:browser", []byte("legacy"), e.TS, func(st model.State) (Change, error) {
		return Change{Revision: st.Revision, Events: []Pending{{Subject: "browser", Verb: "inventory_observed", EntityID: profile.ID, Payload: profile}}, Result: "legacy-receipt"}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	before, _ := db.State(ctx)
	eventsBefore, _ := db.Events(ctx)
	_, err = db.db.Exec(`UPDATE projection_state SET body=json_remove(body,'$.observed_surfaces','$.surface_containers'); PRAGMA user_version=21;`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	db, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err := db.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 22 {
		t.Fatal("schema marker", version, err)
	}
	backups, _ := filepath.Glob(filepath.Join(dir, "backups", "pre-schema-22-*.db"))
	if len(backups) != 1 {
		t.Fatal("missing rollback backup")
	}
	backup, err := sql.Open("sqlite", backups[0])
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	if err := backup.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 21 {
		t.Fatal("backup was migrated", version, err)
	}
	upgraded, _ := db.State(ctx)
	if !reflect.DeepEqual(before, upgraded) {
		t.Fatal("upgrade changed legacy state")
	}
	replay, err := db.Replay(ctx)
	if err != nil || !reflect.DeepEqual(before, replay) {
		t.Fatal("legacy replay invented observations", err)
	}
	// The first valid event must roll back too when a later event in the same
	// source command fails. Its receipt cannot survive a failed transaction.
	_, err = db.Transact(ctx, e.CommandID, e.Actor, []byte("batch"), e.TS, func(st model.State) (Change, error) {
		bad := o
		bad.Title = "not observed"
		return Change{Revision: st.Revision, Events: []Pending{
			{Subject: "surface", Verb: "observed", EntityID: o.SurfaceID, Payload: o},
			{Subject: "surface", Verb: "changed", EntityID: o.SurfaceID, Payload: bad},
		}}, nil
	})
	if err == nil {
		t.Fatal("invalid batch committed")
	}
	after, _ := db.State(ctx)
	eventsAfter, _ := db.Events(ctx)
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(eventsBefore, eventsAfter) {
		t.Fatal("partial event/receipt/projection publication")
	}
	var receipts int
	if err := db.db.QueryRow("SELECT count(*) FROM commands WHERE id=?", e.CommandID).Scan(&receipts); err != nil || receipts != 0 {
		t.Fatal("failed batch retained receipt", err)
	}
}
