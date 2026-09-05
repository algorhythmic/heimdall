package store

import (
	"encoding/json"
	"heimdall/internal/model"
	"os"
	"strings"
	"testing"
)

func TestContinuityGoldenEvents(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/continuity/events-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	if err = model.StrictJSON(raw, &events); err != nil {
		t.Fatal(err)
	}
	st := model.Empty()
	for _, e := range events {
		if e.Subject != "task" {
			bad := e
			var payload map[string]json.RawMessage
			if err := json.Unmarshal(e.Payload, &payload); err != nil {
				t.Fatal(err)
			}
			payload["version"] = json.RawMessage(`99`)
			bad.Payload, _ = json.Marshal(payload)
			if err := Apply(&st, bad); err == nil {
				t.Fatal("unknown payload version accepted", e.Subject)
			}
		}
		if err := Apply(&st, e); err != nil {
			t.Fatal(e.ID, e.Subject, err)
		}
	}
	if len(st.Contracts) != 2 || len(st.Checkpoints) != 3 || len(st.Decisions) != 1 || len(st.Grants) != 2 || st.LastEventID != 12 {
		t.Fatal("fixture coverage lost")
	}
	if st.Grants[strings.Repeat("8", 32)].RevokedAt == nil || st.Resources[strings.Repeat("3", 32)].Active {
		t.Fatal("lifecycle projection mismatch")
	}
	// Round-trip projection normalization must not alter golden replay results.
	encoded, _ := json.Marshal(st)
	var round model.State
	if err = json.Unmarshal(encoded, &round); err != nil {
		t.Fatal(err)
	}
	round.Normalize()
	again, _ := json.Marshal(round)
	if string(encoded) != string(again) {
		t.Fatal("golden state round-trip changed")
	}
}
