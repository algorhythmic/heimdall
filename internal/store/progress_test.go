package store

import (
	"encoding/json"
	"heimdall/internal/model"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestProgressGoldenReplayAndMalformedEvents(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/progress/events-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	if err := json.Unmarshal(raw, &events); err != nil {
		t.Fatal(err)
	}
	st := model.Empty()
	for _, e := range events {
		if e.Subject == "progress" {
			bad := func(change func(*Event, map[string]any), resign bool) {
				t.Helper()
				var body map[string]any
				json.Unmarshal(e.Payload, &body)
				broken := e
				change(&broken, body)
				broken.Payload, _ = json.Marshal(body)
				if resign && e.Verb == "proposed" {
					var p model.ProgressProposal
					json.Unmarshal(broken.Payload, &p)
					p.Digest = p.ContentDigest()
					broken.Payload, _ = json.Marshal(p)
				}
				before := model.Clone(st)
				if err := Apply(&st, broken); err == nil {
					t.Fatal("malformed progress event accepted", string(broken.Payload))
				}
				if !reflect.DeepEqual(before, st) {
					t.Fatal("rejected event mutated state")
				}
			}
			bad(func(_ *Event, b map[string]any) { b["version"] = 2 }, false)
			bad(func(_ *Event, b map[string]any) { b["unknown"] = true }, false)
			bad(func(_ *Event, b map[string]any) { b["digest"] = strings.Repeat("a", 64) }, false)
			bad(func(e *Event, b map[string]any) { e.Actor = "browser"; b["actor"] = "browser" }, true)
			bad(func(e *Event, _ map[string]any) { e.EntityID = model.NewID() }, false)
			bad(func(_ *Event, b map[string]any) { b["target"] = "beta" }, true)
			bad(func(_ *Event, b map[string]any) { b["task_revision"] = 99 }, true)
			bad(func(_ *Event, b map[string]any) { b["previous"] = strings.Repeat("f", 32) }, true)
			if e.Verb == "proposed" {
				bad(func(_ *Event, b map[string]any) { b["text"] = "Tampered content" }, false)
				bad(func(_ *Event, b map[string]any) { b["contract_id"] = strings.Repeat("f", 32) }, true)
				bad(func(_ *Event, b map[string]any) { b["context"] = []any{} }, true)
				bad(func(_ *Event, b map[string]any) { b["decision_digest"] = strings.Repeat("f", 64) }, true)
				var p model.ProgressProposal
				json.Unmarshal(e.Payload, &p)
				if len(p.Artifacts) > 0 {
					bad(func(_ *Event, b map[string]any) {
						b["artifacts"].([]any)[0].(map[string]any)["observation"].(map[string]any)["digest"] = strings.Repeat("f", 64)
					}, true)
				}
			} else {
				bad(func(_ *Event, b map[string]any) { b["proposal_id"] = strings.Repeat("f", 32) }, false)
				bad(func(_ *Event, b map[string]any) { b["status"] = "completed" }, false)
			}
		}
		if err := Apply(&st, e); err != nil {
			t.Fatal(e.ID, err)
		}
		if e.Subject == "progress" {
			before := model.Clone(st)
			if err := Apply(&st, e); err == nil {
				t.Fatal("duplicate progress event accepted")
			}
			if !reflect.DeepEqual(before, st) {
				t.Fatal("duplicate event mutated state")
			}
		}
	}
	if len(st.ProgressProposals) != 4 || len(st.ProgressReviews) != 6 || len(st.Decisions) != 2 || st.Tasks["alpha"].Task.Status != "active" || len(st.Proposals) != 0 {
		t.Fatal("incorrect progress golden replay state")
	}
}
