package store

import (
	"encoding/json"
	"heimdall/internal/conversation"
	"heimdall/internal/model"
	"os"
	"testing"
)

func artifactFixtureState(t *testing.T) model.State {
	t.Helper()
	raw, err := os.ReadFile("../../testdata/artifacts/events-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	if err := json.Unmarshal(raw, &events); err != nil {
		t.Fatal(err)
	}
	st := model.Empty()
	for _, e := range events {
		if err := Apply(&st, e); err != nil {
			t.Fatal("fixture event rejected", e.Subject, e.Verb, err)
		}
	}
	if len(st.ArtifactVersions) == 0 {
		t.Fatal("fixture has no artifact versions")
	}
	return st
}

func startedEvent(t *testing.T, st *model.State) Event {
	t.Helper()
	p := startFixture("native-origin")
	p.ID = "00000000000000000000000000000000"
	e := Event{Version: 1, Subject: "conversation", Verb: "started", Actor: conversationActor,
		EntityID: p.ID, CommandID: conversationCommand(p.Source, p.NativeConversationID, "started", p.AdapterVersion, "", p.Observation),
		TS: p.ObservedAt}
	var err error
	e.Payload, _ = json.Marshal(p)
	if err = Apply(st, e); err != nil {
		t.Fatal(err)
	}
	return e
}

func originEvent(o model.ArtifactOrigin) Event {
	raw, _ := json.Marshal(o)
	return Event{Version: 1, Subject: "artifact", Verb: "origin_observed", Actor: sourceActor,
		EntityID: o.VersionID, CommandID: "origin-test", TS: o.At, Payload: raw}
}

func TestArtifactOriginObserved(t *testing.T) {
	st := artifactFixtureState(t)
	startedEvent(t, &st)
	convID := ""
	for id := range st.Conversations {
		convID = id
	}
	versionID := ""
	for id := range st.ArtifactVersions {
		versionID = id
		break
	}
	at := conversationTime.Add(10_000)
	o := model.ArtifactOrigin{Version: 1, VersionID: versionID, ConversationID: convID,
		Evidence: "prompt", RecordKey: "sr1:record:" + conversation.Digest([]byte("r")), SourceRevision: conversation.SourceRevision([]byte("r")), At: at}
	if err := Apply(&st, originEvent(o)); err != nil {
		t.Fatal(err)
	}
	if st.ArtifactOrigins[versionID].ConversationID != convID {
		t.Fatal("origin not recorded")
	}
	if err := Apply(&st, originEvent(o)); err == nil {
		t.Fatal("duplicate origin accepted")
	}
	// Forged actor, wrong entity and unknown references all reject.
	for _, bad := range []model.ArtifactOrigin{
		{Version: 1, VersionID: "00000000000000000000000000000000", ConversationID: convID, Evidence: "prompt", RecordKey: "r2", SourceRevision: o.SourceRevision, At: at},
		{Version: 1, VersionID: versionID, ConversationID: "00000000000000000000000000000000", Evidence: "prompt", RecordKey: "r3", SourceRevision: o.SourceRevision, At: at},
		{Version: 1, VersionID: versionID, ConversationID: convID, Evidence: "similarity", RecordKey: "r4", SourceRevision: o.SourceRevision, At: at},
	} {
		if err := Apply(&st, originEvent(bad)); err == nil {
			t.Fatalf("invalid origin accepted %+v", bad)
		}
	}
	badActor := originEvent(o)
	badActor.EntityID = "ffffffffffffffffffffffffffffffff"
	if err := Apply(&st, badActor); err == nil {
		t.Fatal("entity mismatch accepted")
	}
	wrongActor := originEvent(model.ArtifactOrigin{Version: 1, VersionID: "00000000000000000000000000000000", ConversationID: convID, Evidence: "prompt", RecordKey: "r5", SourceRevision: o.SourceRevision, At: at})
	wrongActor.Actor = "cli"
	if err := Apply(&st, wrongActor); err == nil {
		t.Fatal("cli actor accepted for observation")
	}
}
