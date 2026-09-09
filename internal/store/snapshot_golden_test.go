package store

import (
	"encoding/json"
	"heimdall/internal/model"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestSnapshotGoldenEnvelopes(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/snapshots/events-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	if err = json.Unmarshal(raw, &events); err != nil {
		t.Fatal(err)
	}
	st := model.Empty()
	for _, e := range events {
		if e.Subject == "snapshot" {
			bad := func(change func(*Event, map[string]any)) {
				t.Helper()
				var p map[string]any
				json.Unmarshal(e.Payload, &p)
				broken := e
				change(&broken, p)
				broken.Payload, _ = json.Marshal(p)
				before := model.Clone(st)
				if err := Apply(&st, broken); err == nil {
					t.Fatal("forged snapshot envelope accepted", e.Verb)
				}
				if !reflect.DeepEqual(before, st) {
					t.Fatal("refused envelope changed state")
				}
			}
			bad(func(_ *Event, p map[string]any) { p["version"] = 2 })
			bad(func(_ *Event, p map[string]any) { p["unknown"] = true })
			bad(func(e *Event, p map[string]any) { e.Actor = "browser"; p["actor"] = "browser" })
			bad(func(e *Event, _ map[string]any) { e.CommandID = "wrong-command" })
			switch e.Verb {
			case "captured":
				bad(func(_ *Event, p map[string]any) { p["input_digest"] = strings.Repeat("f", 64) })
				bad(func(_ *Event, p map[string]any) { p["task_revision"] = 999 })
				bad(func(_ *Event, p map[string]any) { p["payload_bytes"] = 0 })
				bad(func(_ *Event, p map[string]any) { p["source_epoch"] = strings.Repeat("f", 64) })
			case "policy":
				bad(func(_ *Event, p map[string]any) { p["max_dirty_seconds"] = 0 })
			case "pin":
				bad(func(_ *Event, p map[string]any) { p["previous"] = strings.Repeat("f", 32) })
			case "pruned":
				bad(func(_ *Event, p map[string]any) { p["snapshot_ids"] = []string{st.SnapshotHeads["alpha"].ID} })
			}
		}
		if err := Apply(&st, e); err != nil {
			t.Fatal(e.ID, err)
		}
		if st.LastEventID != e.ID {
			t.Fatal("snapshot event cursor did not advance")
		}
	}
	if len(st.SnapshotHeads) != 1 || len(st.SnapshotPins) != 1 || len(st.SnapshotPolicies) != 1 {
		t.Fatal("wrong bounded snapshot projection")
	}
}
