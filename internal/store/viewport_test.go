package store

import (
	"encoding/json"
	"heimdall/internal/model"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestViewportGoldenReplayAndForgery(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/viewport/events-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	if err = json.Unmarshal(raw, &events); err != nil {
		t.Fatal(err)
	}
	st := model.Empty()
	for _, e := range events {
		if e.Subject == "desktop" || e.Subject == "viewport" {
			bad := func(change func(*Event, map[string]any)) {
				t.Helper()
				var payload map[string]any
				json.Unmarshal(e.Payload, &payload)
				broken := e
				change(&broken, payload)
				broken.Payload, _ = json.Marshal(payload)
				before := model.Clone(st)
				if err := Apply(&st, broken); err == nil {
					t.Fatal("forged viewport accepted")
				}
				if !reflect.DeepEqual(st, before) {
					t.Fatal("refused viewport mutated projection")
				}
			}
			bad(func(_ *Event, p map[string]any) { p["unknown"] = true })
			bad(func(_ *Event, p map[string]any) { p["version"] = 2 })
			bad(func(e *Event, p map[string]any) { e.Actor = "browser"; p["actor"] = "browser" })
			bad(func(e *Event, _ map[string]any) { e.CommandID = "wrong-command" })
			bad(func(_ *Event, p map[string]any) { p["previous"] = strings.Repeat("f", 32) })
			if e.Subject == "desktop" {
				bad(func(_ *Event, p map[string]any) { p["compositor_version"] = "0.55.0" })
			} else {
				bad(func(_ *Event, p map[string]any) { p["target"] = "beta" })
				bad(func(_ *Event, p map[string]any) { p["task_revision"] = 999 })
				bad(func(_ *Event, p map[string]any) {
					p["window"].(map[string]any)["source_epoch"] = strings.Repeat("b", 64)
				})
				bad(func(_ *Event, p map[string]any) { p["session_binding_id"] = strings.Repeat("f", 32) })
			}
		}
		if err := Apply(&st, e); err != nil {
			t.Fatal(e.ID, err)
		}
		if st.LastEventID != e.ID {
			t.Fatal("cursor did not advance")
		}
		if e.Subject == "desktop" || e.Subject == "viewport" {
			before := model.Clone(st)
			if err := Apply(&st, e); err == nil {
				t.Fatal("duplicate viewport accepted")
			}
			if !reflect.DeepEqual(st, before) {
				t.Fatal("duplicate mutated state")
			}
		}
	}
	if len(st.DesktopSources) != 1 || len(st.ViewportBindings) != 1 || !st.DesktopSources[st.DesktopSourceHead].Active {
		t.Fatal("incorrect viewport replay")
	}
}
