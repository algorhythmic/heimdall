package model

import (
	"testing"
	"time"
)

func TestCheckpointActionEnvelopeAndCitationBoundary(t *testing.T) {
	c := Checkpoint{Version: 4, ID: NewID(), Target: "alpha", TaskRevision: 1, Actor: "cli", At: time.Now().UTC(), Actions: []string{NewID()}, SourceEvent: 8}
	if err := ValidCheckpoint(c); err != nil {
		t.Fatal(err)
	}
	legacy := c
	legacy.Version = 1
	if ValidCheckpoint(legacy) == nil {
		t.Fatal("legacy accepted new references")
	}
	duplicated := c
	duplicated.Actions = append(duplicated.Actions, c.Actions[0])
	if ValidCheckpoint(duplicated) == nil {
		t.Fatal("duplicate reference accepted")
	}
	st := State{}
	st.Normalize()
	if ValidateCheckpointActions(st, c) == nil {
		t.Fatal("missing action accepted")
	}
	// A report cannot stand in for an independently matched observation.
	st.Actions[c.Actions[0]] = ActionRecord{Intent: ActionIntent{ID: c.Actions[0], Target: "alpha", TaskRevision: 1}, Report: &ActionReport{Status: "succeeded"}, Execution: "api_reported", Verification: "pending", LastEventID: 7}
	if ValidateCheckpointActions(st, c) == nil {
		t.Fatal("report used as checkpoint evidence")
	}
	client := c
	client.GrantID = NewID()
	client.Actor = "client:" + client.GrantID
	if err := ValidCheckpoint(client); err != nil {
		t.Fatal(err)
	}
	client.Artifacts = []ArtifactRef{}
	if ValidCheckpoint(client) == nil {
		t.Fatal("action references widened artifact authority")
	}
}
