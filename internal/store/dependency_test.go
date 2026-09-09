package store

import (
	"encoding/json"
	"heimdall/internal/model"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestDependencyGoldenReplayAndForgery(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/dependencies/events-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	if err = json.Unmarshal(raw, &events); err != nil {
		t.Fatal(err)
	}
	st := model.Empty()
	for _, e := range events {
		if e.Subject == "dependency" {
			bad := func(change func(*Event, map[string]any)) {
				t.Helper()
				var b map[string]any
				json.Unmarshal(e.Payload, &b)
				broken := e
				change(&broken, b)
				broken.Payload, _ = json.Marshal(b)
				before := model.Clone(st)
				if err := Apply(&st, broken); err == nil {
					t.Fatal("forged dependency accepted")
				}
				if !reflect.DeepEqual(before, st) {
					t.Fatal("refused dependency changed state")
				}
			}
			bad(func(_ *Event, b map[string]any) { b["version"] = 2 })
			bad(func(_ *Event, b map[string]any) { b["unknown"] = true })
			bad(func(e *Event, b map[string]any) { e.Actor = "browser"; b["actor"] = "browser" })
			bad(func(_ *Event, b map[string]any) { b["task_revision"] = 999 })
			bad(func(_ *Event, b map[string]any) { b["prerequisite_revision"] = 999 })
			bad(func(_ *Event, b map[string]any) { b["previous"] = strings.Repeat("f", 32) })
			bad(func(_ *Event, b map[string]any) { b["depends_on"] = b["target"] })
			var d model.TaskDependency
			json.Unmarshal(e.Payload, &d)
			if d.Target == "alpha" && d.Active {
				bad(func(_ *Event, b map[string]any) { b["depends_on"] = "root" })
			}
		}
		if err := Apply(&st, e); err != nil {
			t.Fatal(e.ID, err)
		}
		if st.LastEventID != e.ID {
			t.Fatal("event cursor did not advance")
		}
		if e.Subject == "dependency" {
			before := model.Clone(st)
			if err := Apply(&st, e); err == nil {
				t.Fatal("duplicate dependency accepted")
			}
			if !reflect.DeepEqual(before, st) {
				t.Fatal("duplicate mutated state")
			}
		}
	}
	if err := model.ValidateDependencyGraph(st); err != nil {
		t.Fatal(err)
	}
	if len(st.Dependencies) != 3 || len(st.DependencyHeads) != 2 || st.Tasks["alpha"].Task.Status != "active" {
		t.Fatal("incorrect dependency replay")
	}
}
