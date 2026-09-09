package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSchemaFifteenBrowserActionCompatibility(t *testing.T) {
	dir := t.TempDir()
	raw, err := os.ReadFile("../../testdata/actions/schema15.sql")
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "heimdall.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(raw)); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	st, err := s.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(st.Actions) != 0 || len(st.BrowserOperations) != 1 {
		t.Fatal("legacy action scope was invented")
	}
	for _, op := range st.BrowserOperations {
		if op.Status != "succeeded" || op.ActionRef != nil {
			t.Fatal("legacy operation mutated", op)
		}
	}
	before, _ := s.Events(ctx)
	replayed, err := s.Replay(ctx)
	if err != nil || !reflect.DeepEqual(st, replayed) {
		t.Fatal("legacy replay", err)
	}
	after, _ := s.Events(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("replay emitted action records")
	}
	backups, err := filepath.Glob(filepath.Join(dir, "backups", fmt.Sprintf("pre-schema-%d-*.db", SchemaVersion)))
	if err != nil || len(backups) != 1 {
		t.Fatal(backups, err)
	}
	old, err := sql.Open("sqlite", backups[0])
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	var version int
	if err = old.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 15 {
		t.Fatal(version, err)
	}
}

func TestActionEventGoldenAndForgery(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/actions/events-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	if err = json.Unmarshal(raw, &events); err != nil {
		t.Fatal(err)
	}
	st := model.Empty()
	count := 0
	for _, e := range events {
		if e.Subject == "action" {
			count++
			bad := func(change func(*Event, map[string]any)) {
				t.Helper()
				broken := e
				var p map[string]any
				json.Unmarshal(e.Payload, &p)
				change(&broken, p)
				broken.Payload, _ = json.Marshal(p)
				before := model.Clone(st)
				if err := Apply(&st, broken); err == nil {
					t.Fatal("forged action accepted", e.Verb)
				}
				if !reflect.DeepEqual(st, before) {
					t.Fatal("forged event changed state")
				}
			}
			bad(func(_ *Event, p map[string]any) { p["version"] = 2 })
			bad(func(e *Event, _ map[string]any) { e.Actor = "client:forged" })
			bad(func(_ *Event, p map[string]any) { p["unknown_authority"] = true })
			if e.Verb == "queued" {
				bad(func(_ *Event, p map[string]any) { p["task_revision"] = 999 })
				bad(func(_ *Event, p map[string]any) { p["authority"] = "agent" })
				bad(func(_ *Event, p map[string]any) { p["surface_id"] = model.NewID() })
			} else {
				bad(func(_ *Event, p map[string]any) { p["attempt_id"] = model.NewID() })
				bad(func(_ *Event, p map[string]any) { p["previous_revision"] = 999 })
				if p := map[string]any{}; json.Unmarshal(e.Payload, &p) == nil && p["kind"] == "dispatch" {
					bad(func(_ *Event, p map[string]any) { delete(p, "observation") })
				}
			}
		}
		if err := Apply(&st, e); err != nil {
			t.Fatal(e.ID, e.Subject, e.Verb, err)
		}
		if st.LastEventID != e.ID {
			t.Fatal("action cursor did not advance")
		}
	}
	if count != 5 || len(st.Actions) != 1 {
		t.Fatal("action fixture lost transitions", count)
	}
	for _, a := range st.Actions {
		if a.Execution != "api_reported" || a.Verification != "pending" || !a.CancelRequested || a.UncertainSince.IsZero() {
			t.Fatal(a)
		}
	}
}
