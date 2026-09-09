package store

import (
	"encoding/json"
	"heimdall/internal/model"
	"os"
	"reflect"
	"testing"
)

func TestWorkspaceGoldenEvents(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/workspace/events-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	if err = json.Unmarshal(raw, &events); err != nil || len(events) != 3 {
		t.Fatal(err)
	}
	st := model.Empty()
	st.Tasks["alpha"] = model.TaskRecord{Task: model.Task{ID: "alpha"}, Revision: 1}
	for _, e := range events {
		if err := Apply(&st, e); err != nil {
			t.Fatal(e.Subject, err)
		}
		before := model.Clone(st)
		if err := Apply(&st, e); err == nil {
			t.Fatal("duplicate immutable record accepted")
		}
		if !reflect.DeepEqual(before, st) {
			t.Fatal("duplicate reducer changed state")
		}
	}
	if len(st.WorkspaceManifests) != 1 || len(st.WorkspaceSurfaces) != 1 || len(st.SessionBindings) != 2 || st.SessionBindings[st.SessionHeads["aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"]].Active {
		t.Fatal(st)
	}
}

func TestObservedHerdrGoldenEvent(t *testing.T) {
	st := model.Empty()
	st.Tasks["alpha"] = model.TaskRecord{Task: model.Task{ID: "alpha"}, Revision: 1}
	load := func(file string) []Event {
		t.Helper()
		b, err := os.ReadFile("../../testdata/workspace/" + file)
		if err != nil {
			t.Fatal(err)
		}
		var events []Event
		if err = json.Unmarshal(b, &events); err != nil {
			t.Fatal(err)
		}
		return events
	}
	if err := Apply(&st, load("events-v1.json")[0]); err != nil {
		t.Fatal(err)
	}
	for _, e := range load("events-v2.json") {
		if err := Apply(&st, e); err != nil {
			t.Fatal(err)
		}
	}
	if st.SessionBindings[st.SessionHeads["aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"]].Version != 2 {
		t.Fatal("observed binding not replayed")
	}
}
