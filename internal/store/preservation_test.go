package store

import (
	"encoding/json"
	"heimdall/internal/model"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestPreservationGoldenReplayAndForgery(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/preservation/events-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	if err = json.Unmarshal(raw, &events); err != nil {
		t.Fatal(err)
	}
	st := model.Empty()
	for _, e := range events {
		if e.Subject == "preservation" {
			bad := func(change func(*Event, map[string]any)) {
				t.Helper()
				var body map[string]any
				json.Unmarshal(e.Payload, &body)
				broken := e
				change(&broken, body)
				broken.Payload, _ = json.Marshal(body)
				before := model.Clone(st)
				if err := Apply(&st, broken); err == nil {
					t.Fatal("forged preservation event accepted", e.Verb)
				}
				if !reflect.DeepEqual(before, st) {
					t.Fatal("rejected event mutated state")
				}
			}
			bad(func(_ *Event, b map[string]any) { b["version"] = 2 })
			bad(func(e *Event, b map[string]any) { e.Actor = "browser"; b["actor"] = "browser" })
			bad(func(_ *Event, b map[string]any) { b["task_revision"] = 999 })
			bad(func(_ *Event, b map[string]any) { b["target"] = "another-task" })
			bad(func(_ *Event, b map[string]any) { b["unknown"] = true })
			bad(func(_ *Event, b map[string]any) { b["snapshot"].(map[string]any)["stage"] = "completed" })
			bad(func(_ *Event, b map[string]any) {
				b["snapshot"].(map[string]any)["items"].([]any)[0].(map[string]any)["expected_digest"] = strings.Repeat("f", 64)
			})
			if e.Verb == "requested" {
				bad(func(_ *Event, b map[string]any) { b["preview_digest"] = strings.Repeat("f", 64) })
			} else {
				bad(func(_ *Event, b map[string]any) { b["previous"] = strings.Repeat("f", 32) })
				bad(func(_ *Event, b map[string]any) { b["reported_outcome"] = "completed" })
				bad(func(_ *Event, b map[string]any) {
					b["snapshot"].(map[string]any)["remote_status"] = "matched"
					b["snapshot"].(map[string]any)["remote_commit"] = ""
				})
			}
		}
		if err := Apply(&st, e); err != nil {
			t.Fatal(e.ID, err)
		}
		if st.LastEventID != e.ID {
			t.Fatal("preservation reducer did not advance the event cursor")
		}
		if e.Subject == "preservation" {
			before := model.Clone(st)
			if err := Apply(&st, e); err == nil {
				t.Fatal("duplicate accepted")
			}
			if !reflect.DeepEqual(before, st) {
				t.Fatal("duplicate changed projection")
			}
		}
	}
	if len(st.PreservationPlans) != 1 || len(st.PreservationReceipts) != 5 || st.Tasks["alpha"].Task.Status != "active" {
		t.Fatal("incorrect inert replay")
	}
}
