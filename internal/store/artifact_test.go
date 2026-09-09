package store

import (
	"encoding/json"
	"heimdall/internal/model"
	"os"
	"reflect"
	"testing"
)

func TestArtifactGoldenEventsAndRefusals(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/artifacts/events-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	if err = json.Unmarshal(raw, &events); err != nil {
		t.Fatal(err)
	}
	st := model.Empty()
	for _, e := range events {
		if e.Subject == "artifact" || e.Subject == "checkpoint" {
			bad := func(change func(*Event, map[string]any)) {
				t.Helper()
				var body map[string]any
				json.Unmarshal(e.Payload, &body)
				broken := e
				change(&broken, body)
				broken.Payload, _ = json.Marshal(body)
				before := model.Clone(st)
				if err := Apply(&st, broken); err == nil {
					t.Fatal("malformed event accepted", broken.Subject, string(broken.Payload))
				}
				if !reflect.DeepEqual(before, st) {
					t.Fatal("rejected event mutated state")
				}
			}
			bad(func(_ *Event, b map[string]any) { b["version"] = 99 })
			bad(func(_ *Event, b map[string]any) { b["undeclared"] = true })
			bad(func(e *Event, b map[string]any) {
				e.Actor = "client:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
				b["actor"] = e.Actor
				b["grant_id"] = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
			})
			if e.Subject == "artifact" && e.Verb == "versioned" {
				bad(func(_ *Event, b map[string]any) { b["previous"] = "ffffffffffffffffffffffffffffffff" })
				bad(func(_ *Event, b map[string]any) { b["path"] = "../outside" })
				bad(func(_ *Event, b map[string]any) { b["path"] = ".git/config" })
				bad(func(_ *Event, b map[string]any) { b["target"] = "other-task" })
				bad(func(_ *Event, b map[string]any) { b["resource_id"] = "ffffffffffffffffffffffffffffffff" })
			}
			if e.Subject == "checkpoint" {
				bad(func(_ *Event, b map[string]any) { b["version"] = 1 })
				bad(func(_ *Event, b map[string]any) { b["version"] = 1; b["artifacts"] = nil })
				bad(func(_ *Event, b map[string]any) { b["artifacts"] = []any{} })
				bad(func(_ *Event, b map[string]any) {
					b["artifacts"].([]any)[0].(map[string]any)["version_id"] = "ffffffffffffffffffffffffffffffff"
				})
			}
		}
		if err = Apply(&st, e); err != nil {
			t.Fatal(e.ID, err)
		}
		if e.Subject == "artifact" || e.Subject == "checkpoint" {
			before := model.Clone(st)
			if err = Apply(&st, e); err == nil {
				t.Fatal("duplicate immutable record accepted")
			}
			if !reflect.DeepEqual(before, st) {
				t.Fatal("duplicate event mutated state")
			}
		}
	}
	if len(st.Artifacts) != 2 || len(st.ArtifactVersions) != 3 || len(st.Checkpoints) != 2 || st.Tasks["alpha"].Task.Status != "active" {
		t.Fatal("wrong artifact replay state")
	}
	for _, cp := range st.Checkpoints {
		if cp.Version != 3 || len(cp.Artifacts) != 1 {
			t.Fatal("lost versioned checkpoint reference")
		}
		if st.ArtifactHeads[cp.Artifacts[0].ArtifactID] == cp.Artifacts[0].VersionID {
			t.Fatal("replay silently repinned historical checkpoint")
		}
	}
}
