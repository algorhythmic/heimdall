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

func TestPairingSchemaSeventeenAndEighteenCompatibility(t *testing.T) {
	for _, name := range []string{"../browser-verification/schema17.sql", "schema18.sql"} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			raw, err := os.ReadFile(filepath.Join("../../testdata/browser-pairing", name))
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
			st, err := s.State(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			replay, err := s.Replay(context.Background())
			if err != nil || !reflect.DeepEqual(st, replay) {
				t.Fatal("state/replay drift", err)
			}
			if name == "schema18.sql" {
				if len(st.BrowserAssociations) != 2 {
					t.Fatal("pairing history lost")
				}
				for _, a := range st.Actions {
					if a.Intent.Version != 2 || a.Pairing == nil || a.Verification != "matched" {
						t.Fatal(a)
					}
				}
			} else if len(st.BrowserAssociations) != 0 {
				t.Fatal("legacy browser promoted to native ownership")
			}
		})
	}
}
func TestPairingGoldenRejectsForgedAssociationAndContinuation(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/browser-pairing/events-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	if err = json.Unmarshal(raw, &events); err != nil {
		t.Fatal(err)
	}
	st := model.Empty()
	proofs := 0
	for _, e := range events {
		var p map[string]any
		json.Unmarshal(e.Payload, &p)
		if e.Verb == "association_observed" || (e.Subject == "action" && model.Contains([]string{"pair_probe", "pair_bound", "pair_continue"}, kindString(p))) {
			bad := func(change func(*Event, map[string]any)) {
				t.Helper()
				broken := e
				var b map[string]any
				json.Unmarshal(e.Payload, &b)
				change(&broken, b)
				broken.Payload, _ = json.Marshal(b)
				copy := model.Clone(st)
				if err := Apply(&copy, broken); err == nil {
					t.Fatal("forged pairing event accepted", e.ID, e.Verb)
				}
				if !reflect.DeepEqual(copy, st) {
					t.Fatal("rejected pairing changed projection")
				}
			}
			bad(func(e *Event, _ map[string]any) { e.Actor = "observer:browser" })
			if e.Verb == "association_observed" {
				proofs++
				bad(func(_ *Event, p map[string]any) { p["matching_windows"] = 2 })
				bad(func(_ *Event, p map[string]any) { p["probe_id"] = model.NewID() })
				bad(func(_ *Event, p map[string]any) { p["window"].(map[string]any)["stable_id"] = "deadbeef" })
			}
			if p["kind"] == "pair_continue" {
				bad(func(_ *Event, p map[string]any) { p["continuation_id"] = model.NewID() })
			}
		}
		if err := Apply(&st, e); err != nil {
			t.Fatal(e.ID, e.Verb, err)
		}
	}
	if proofs != 2 {
		t.Fatal("missing association coverage", proofs)
	}
}

func kindString(p map[string]any) string { s, _ := p["kind"].(string); return s }
