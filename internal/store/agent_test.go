package store

import (
	"encoding/json"
	"heimdall/internal/model"
	"testing"
	"time"
)

func agentFixture() (model.State, Event, model.AgentRecord) {
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	r := model.AgentRecord{Version: 1, Adapter: "herdr", Host: "host1", SourceEpoch: "epoch1",
		SessionID: "/tmp/herdr.sock", WorkspaceID: "w1", PaneID: "w1:p1", TabID: "w1:t1",
		TerminalID: "term_1", Cwd: "/repo", Agent: "codex", AgentSessionID: "as-1",
		Status: "working", StateSeq: 5, Active: true, At: now}
	payload, _ := json.Marshal(r)
	e := Event{ID: 1, Version: 1, TS: now, Subject: "agent", Verb: "observed", Actor: "observer:herdr",
		EntityID: r.ContainerKey(), CommandID: "agent-" + model.NewID(), Payload: payload}
	return model.Empty(), e, r
}

func TestAgentObservedRecordsHead(t *testing.T) {
	st, e, r := agentFixture()
	if err := Apply(&st, e); err != nil {
		t.Fatal(err)
	}
	got := st.AgentHeads[r.ContainerKey()]
	if !got.Active || got.Status != "working" || got.StateSeq != 5 || got.AgentSessionID != "as-1" {
		t.Fatalf("unexpected head %+v", got)
	}
}

func TestAgentRejectsInvalidEnvelope(t *testing.T) {
	for name, mutate := range map[string]func(*Event, *model.AgentRecord){
		"actor":    func(e *Event, r *model.AgentRecord) { e.Actor = "cli" },
		"command":  func(e *Event, r *model.AgentRecord) { e.CommandID = "other-1" },
		"entity":   func(e *Event, r *model.AgentRecord) { e.EntityID = "wrong" },
		"version":  func(e *Event, r *model.AgentRecord) { r.Version = 2 },
		"status":   func(e *Event, r *model.AgentRecord) { r.Status = "running" },
		"pane":     func(e *Event, r *model.AgentRecord) { r.PaneID = "" },
		"sequence": func(e *Event, r *model.AgentRecord) { r.StateSeq = 0 },
		"inactive": func(e *Event, r *model.AgentRecord) { r.Active = false },
	} {
		t.Run(name, func(t *testing.T) {
			st, e, r := agentFixture()
			mutate(&e, &r)
			e.Payload, _ = json.Marshal(r)
			if err := Apply(&st, e); err == nil {
				t.Fatalf("accepted mutated %s", name)
			}
		})
	}
}

func TestAgentRejectsRegressionAndDuplicate(t *testing.T) {
	st, e, r := agentFixture()
	if err := Apply(&st, e); err != nil {
		t.Fatal(err)
	}
	if err := Apply(&st, e); err == nil {
		t.Fatal("reapplied identical observation")
	}
	old := r
	old.StateSeq--
	payload, _ := json.Marshal(old)
	regress := e
	regress.Payload = payload
	if err := Apply(&st, regress); err == nil {
		t.Fatal("accepted sequence regression")
	}
	next := r
	next.StateSeq, next.Status = r.StateSeq+1, "idle"
	payload, _ = json.Marshal(next)
	e.Payload = payload
	if err := Apply(&st, e); err != nil {
		t.Fatal(err)
	}
	if st.AgentHeads[r.ContainerKey()].Status != "idle" {
		t.Fatal("status not updated")
	}
}

func TestAgentDetachRequiresActiveObservation(t *testing.T) {
	st, e, r := agentFixture()
	d := r
	d.Active = false
	payload, _ := json.Marshal(d)
	detach := e
	detach.Verb, detach.Payload = "detached", payload
	if err := Apply(&st, detach); err == nil {
		t.Fatal("detached without a prior active observation")
	}
	if err := Apply(&st, e); err != nil {
		t.Fatal(err)
	}
	if err := Apply(&st, detach); err != nil {
		t.Fatal(err)
	}
	if st.AgentHeads[r.ContainerKey()].Active {
		t.Fatal("record stayed active")
	}
	if err := Apply(&st, detach); err == nil {
		t.Fatal("re-detached inactive record")
	}
}
