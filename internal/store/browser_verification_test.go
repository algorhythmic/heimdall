package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"heimdall/internal/model"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestSchemaSixteenActionCompatibility(t *testing.T) {
	dir := t.TempDir()
	raw, err := os.ReadFile("../../testdata/browser-verification/schema16.sql")
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
	if len(st.Actions) != 1 {
		t.Fatal("schema-16 action lost")
	}
	for _, a := range st.Actions {
		if a.Intent.Version != 1 || a.Execution != "api_reported" || a.Verification != "pending" || !a.CancelRequested || a.UncertainSince.IsZero() || a.Observation != nil {
			t.Fatal("old success promoted or history changed", a)
		}
	}
	replay, err := s.Replay(ctx)
	if err != nil || !reflect.DeepEqual(st, replay) {
		t.Fatal("schema-16 replay", err)
	}
}
func TestBrowserVerificationGoldenAndForgery(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/browser-verification/events-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	if err = json.Unmarshal(raw, &events); err != nil {
		t.Fatal(err)
	}
	st := model.Empty()
	verified := 0
	for _, e := range events {
		var payload map[string]any
		json.Unmarshal(e.Payload, &payload)
		if e.Subject == "browser" && (e.Verb == "challenge_issued" || e.Verb == "readback_observed") || e.Subject == "action" && payload["kind"] == "verify" {
			bad := func(change func(*Event, map[string]any)) {
				t.Helper()
				broken := e
				var p map[string]any
				json.Unmarshal(e.Payload, &p)
				change(&broken, p)
				broken.Payload, _ = json.Marshal(p)
				copy := model.Clone(st)
				if err := Apply(&copy, broken); err == nil {
					t.Fatal("forged browser proof accepted", e.ID, e.Verb)
				}
				if !reflect.DeepEqual(copy, st) {
					t.Fatal("failed proof mutated projection")
				}
			}
			bad(func(_ *Event, p map[string]any) { p["version"] = 99 })
			bad(func(e *Event, _ map[string]any) { e.Actor = "cli" })
			if e.Verb == "challenge_issued" {
				bad(func(_ *Event, p map[string]any) { p["after_event_id"] = float64(9999999) })
			}
			if e.Verb == "readback_observed" {
				bad(func(_ *Event, p map[string]any) { p["challenge_id"] = model.NewID() })
			}
			if payload["kind"] == "verify" {
				verified++
				bad(func(_ *Event, p map[string]any) {
					p["observation"].(map[string]any)["digest"] = model.ContentDigest("forged")
				})
				bad(func(_ *Event, p map[string]any) {
					o := p["observation"].(map[string]any)
					if o["status"] == "matched" {
						o["status"] = "not_matched"
					} else {
						o["status"] = "matched"
					}
				})
			}
		}
		if err := Apply(&st, e); err != nil {
			t.Fatal(e.ID, e.Subject, e.Verb, err)
		}
		if st.LastEventID != e.ID {
			t.Fatal("cursor lost")
		}
	}
	if verified < 7 {
		t.Fatal("postconditions missing from fixture", verified)
	}
	if st.Tasks["alpha"].Task.Status != "active" {
		t.Fatal("browser action completed task")
	}
}
