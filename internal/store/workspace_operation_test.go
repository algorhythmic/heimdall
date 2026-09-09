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

func TestWorkspaceOperationSchemaNineteenReplay(t *testing.T) {
	dir := t.TempDir()
	raw, err := os.ReadFile("../../testdata/workspace-operations/schema19.sql")
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
		t.Fatal("operation replay drift", err)
	}
	if len(st.WorkspaceOperations) != 3 || len(st.Actions) != 3 || len(st.WorkspaceSlots) != 0 || st.Tasks["alpha"].Task.Status != "active" {
		t.Fatal("operation fixture scope changed")
	}
	uncertain := 0
	for _, a := range st.Actions {
		if a.Execution == "uncertain" && a.Verification == "matched" && a.Report == nil {
			uncertain++
		}
	}
	if uncertain != 1 {
		t.Fatal("lost ACK history was rewritten")
	}
	for _, op := range st.WorkspaceOperations {
		if op.Intent.Kind == "close" {
			point, err := s.WorkspacePoint(context.Background(), "alpha", op.CloseSnapshotID)
			if err != nil || point.Payload == nil || point.Point.Kind != "operation" {
				t.Fatal("close capture lost", err)
			}
		}
	}
}

func TestWorkspaceOperationGoldenRefusesForgedAuthorityAndReadback(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/workspace-operations/events-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	if err = json.Unmarshal(raw, &events); err != nil {
		t.Fatal(err)
	}
	st := model.Empty()
	checks := 0
	for _, e := range events {
		if e.Subject == "workspace" || e.Subject == "action" || (e.Subject == "snapshot" && e.Verb == "captured") {
			bad := e
			bad.Actor = "observer:browser"
			copy := model.Clone(st)
			if err := Apply(&copy, bad); err == nil {
				t.Fatal("browser gained native authority", e.ID, e.Verb)
			}
			if !reflect.DeepEqual(copy, st) {
				t.Fatal("rejected authority changed projection", e.ID)
			}
			checks++
		}
		if e.Subject == "action" && e.Verb == "transitioned" {
			var v model.ActionTransition
			json.Unmarshal(e.Payload, &v)
			if v.Observation != nil && v.Observation.Native != nil {
				mutations := []func(*model.ActionTransition){
					func(v *model.ActionTransition) { v.Observation.Native.AfterEventID-- },
					func(v *model.ActionTransition) { v.Observation.Native.AttemptID = model.NewID() },
					func(v *model.ActionTransition) { v.Observation.Native.SourceID = model.NewID() },
					func(v *model.ActionTransition) { v.Observation.Status = "unsupported" },
					func(v *model.ActionTransition) {
						v.Observation.Native.FocusedWindow = &model.WindowIdentity{SourceEpoch: v.Observation.SourceEpoch, StableID: "18009999"}
					},
				}
				for _, mutate := range mutations {
					changed := model.Clone(v)
					mutate(&changed)
					changed.Observation.Digest = model.ContentDigest(changed.Observation.Native)
					bad := e
					bad.Payload, _ = json.Marshal(changed)
					copy := model.Clone(st)
					if err := Apply(&copy, bad); err == nil {
						t.Fatal("forged readback accepted", e.ID)
					}
					if !reflect.DeepEqual(copy, st) {
						t.Fatal("forged readback leaked projection")
					}
					checks++
				}
			}
		}
		if err := Apply(&st, e); err != nil {
			t.Fatal(e.ID, e.Subject, e.Verb, err)
		}
	}
	if checks < 50 || len(st.WorkspaceOperations) != 3 {
		t.Fatal("native negative coverage missing", checks)
	}
}
