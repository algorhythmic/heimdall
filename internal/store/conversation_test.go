package store

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"heimdall/internal/conversation"
	"heimdall/internal/model"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

type descriptionResolverFunc func(context.Context, conversation.Description) (ResolvedDescription, error)

func (f descriptionResolverFunc) ResolveDescription(ctx context.Context, d conversation.Description) (ResolvedDescription, error) {
	return f(ctx, d)
}

var conversationTime = time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)

func sourceFixture() conversation.SourceKey {
	return conversation.SourceKey{AdapterID: "synthetic-v1", ContractMajor: 1, Namespace: "fixture:conversation", LogicalStream: "primary"}
}
func observationFixture(key string, seq int64) conversation.Observation {
	at := conversationTime.Add(time.Duration(seq) * time.Second)
	return conversation.Observation{RecordKey: key, SourceRevision: conversation.SourceRevision([]byte(key)), Epoch: 0, Sequence: seq, SourceTime: at, ObservedAt: at}
}
func startFixture(native string) conversation.Started {
	o := observationFixture("start-"+native, 1)
	return conversation.Started{Version: 1, Kind: "claude_code", Source: sourceFixture(), NativeConversationID: native, StartedAt: o.SourceTime, AdapterVersion: "fixture-v1", ContractVersion: conversation.ContractVersion{Major: 1}, Observation: o}
}
func startConversation(t *testing.T, s *Store, native string) string {
	t.Helper()
	raw, err := s.RecordConversationStarted(context.Background(), startFixture(native))
	if err != nil {
		t.Fatal(err)
	}
	var receipt ConversationReceipt
	if err := json.Unmarshal(raw, &receipt); err != nil {
		t.Fatal(err)
	}
	return receipt.ConversationID
}
func descriptionFixture(id, key string, seq int64) conversation.Description {
	return conversation.Description{Version: 1, ConversationID: id, Source: sourceFixture(), Kind: "recap", AdapterVersion: "fixture-v1", ContractVersion: conversation.ContractVersion{Major: 1}, Provenance: conversation.Provenance{Kind: "native", Producer: "fixture"}, Coverage: conversation.Coverage{Kind: "unknown"}, Availability: "available", Observation: observationFixture(key, seq)}
}
func recordDescription(t *testing.T, s *Store, id, key string, seq int64, text string) conversation.Description {
	t.Helper()
	if _, err := s.RecordDescription(context.Background(), descriptionFixture(id, key, seq), []byte(text)); err != nil {
		t.Fatal(err)
	}
	st, err := s.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return *st.Conversations[id].CurrentDescription
}
func evidence(t *testing.T, s *Store, d conversation.Description) DescriptionEvidence {
	t.Helper()
	v, err := s.DescriptionEvidence(context.Background(), d.ConversationID, d.Source, d.RecordKey, d.SourceRevision)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
func snapshotJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func conversationStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
func purgeEvidence(t *testing.T, s *Store) {
	t.Helper()
	if _, err := s.db.Exec("DELETE FROM conversation_evidence"); err != nil {
		t.Fatal(err)
	}
}

func TestConversationLifecycleResumeDedupeAndScope(t *testing.T) {
	ctx := context.Background()
	s := conversationStore(t)
	p := startFixture("native")
	raw, err := s.RecordConversationStarted(ctx, p)
	if err != nil {
		t.Fatal(err)
	}
	retry := p
	retry.ObservedAt = retry.ObservedAt.Add(time.Hour)
	retry.Sequence++
	again, err := s.RecordConversationStarted(ctx, retry)
	if err != nil || !bytes.Equal(raw, again) {
		t.Fatal("start dedupe", err)
	}
	var receipt ConversationReceipt
	json.Unmarshal(raw, &receipt)
	id := receipt.ConversationID
	if !model.OpaqueID.MatchString(id) {
		t.Fatal("wrong ID")
	}
	locator := "/configured/fixture.jsonl"
	end := conversation.Ended{Version: 1, ConversationID: id, TranscriptRef: conversation.TranscriptRef{Locator: locator, Digest: conversation.Digest([]byte(locator))}, Turns: 2, Artifacts: []string{}, Observation: observationFixture("end", 2)}
	end.EndedAt = end.SourceTime
	if _, err := s.RecordConversationEnded(ctx, end); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordConversationEnded(ctx, end); err != nil {
		t.Fatal("end retry", err)
	}
	p.Observation = observationFixture("resume", 3)
	p.StartedAt = p.SourceTime
	p.AdapterVersion = "fixture-v2"
	resumed, err := s.RecordConversationStarted(ctx, p)
	if err != nil || !bytes.Equal(resumed, raw) {
		t.Fatal("resume allocated another ID", err)
	}
	if _, err := s.RecordConversationEnded(ctx, end); err != nil {
		t.Fatal("end retry after adapter version changed", err)
	}
	st, _ := s.State(ctx)
	c := st.Conversations[id]
	if len(st.Conversations) != 1 || c.ResumeCount != 1 || c.Ended != nil || c.TranscriptRef == nil || c.FirstStartedAt != startFixture("native").StartedAt {
		t.Fatal("resume projection", c)
	}
	view, err := s.Conversations(ctx, "", conversationTime.Add(time.Hour))
	if err != nil || view[0].Lifecycle != "inactive-by-policy" {
		t.Fatal(view, err)
	}
	p = startFixture("unbound-invalid")
	p.Task = &conversation.TaskRef{Target: "missing", Revision: 1}
	before := snapshotJSON(t, st)
	if _, err := s.RecordConversationStarted(ctx, p); err == nil {
		t.Fatal("unknown task accepted")
	}
	after, _ := s.State(ctx)
	if !bytes.Equal(before, snapshotJSON(t, after)) {
		t.Fatal("failed binding leaked state")
	}
	if _, err := s.Conversations(ctx, "missing", conversationTime); err == nil {
		t.Fatal("unknown filter accepted")
	}
}

func TestDescriptionReplayHydrationAndReopenEquality(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { s.Close() }()
	id := startConversation(t, s, "replay")
	raw := "The task is complete.  \r\n"
	d := recordDescription(t, s, id, "recap", 2, raw)
	if evidence(t, s, d).Text != "The task is complete." {
		t.Fatal("normalization")
	}
	st, _ := s.State(ctx)
	before := snapshotJSON(t, st)
	events, _ := s.Events(ctx)
	eventGolden := snapshotJSON(t, events)
	if bytes.Contains(before, []byte("The task is complete")) || bytes.Contains(eventGolden, []byte("The task is complete")) {
		t.Fatal("text leaked into replay")
	}
	var receipts string
	if err := s.db.QueryRow("SELECT group_concat(result) FROM commands").Scan(&receipts); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(receipts, "The task is complete") {
		t.Fatal("receipt text leak")
	}
	replayed, err := s.Replay(ctx)
	if err != nil || !bytes.Equal(before, snapshotJSON(t, replayed)) {
		t.Fatal("replay with bytes", err)
	}
	purgeEvidence(t, s)
	replayed, err = s.Replay(ctx)
	if err != nil || !bytes.Equal(before, snapshotJSON(t, replayed)) {
		t.Fatal("replay without bytes", err)
	}
	if evidence(t, s, d).Gap != "evidence_missing" {
		t.Fatal("missing evidence gap")
	}
	resolver := descriptionResolverFunc(func(_ context.Context, want conversation.Description) (ResolvedDescription, error) {
		return ResolvedDescription{want.SourceRevision, []byte(raw)}, nil
	})
	results, err := s.Hydrate(ctx, resolver, 1)
	if err != nil || len(results) != 1 || !results[0].Hydrated {
		t.Fatal(results, err)
	}
	after, _ := s.State(ctx)
	afterEvents, _ := s.Events(ctx)
	if !bytes.Equal(before, snapshotJSON(t, after)) || !bytes.Equal(eventGolden, snapshotJSON(t, afterEvents)) {
		t.Fatal("hydration changed replay inputs/state")
	}
	if evidence(t, s, d).Text != "The task is complete." {
		t.Fatal("hydration bytes missing")
	}
	if err := s.Backup(ctx, filepath.Join(dir, "copy.db")); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	reopened, _ := s.State(ctx)
	if !bytes.Equal(before, snapshotJSON(t, reopened)) || evidence(t, s, d).Gap != "" {
		t.Fatal("reopen equality")
	}
	// Absence of these two disposable tables is not a schema-22 migration failure.
	if _, err := s.db.Exec("DROP TABLE conversation_description_associations; DROP TABLE conversation_evidence"); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	var schema int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&schema); err != nil || schema != 22 {
		t.Fatal(schema, err)
	}
	replayed, err = s.Replay(ctx)
	if err != nil || !bytes.Equal(before, snapshotJSON(t, replayed)) {
		t.Fatal("missing disposable tables", err)
	}
	if evidence(t, s, d).Gap != "evidence_missing" {
		t.Fatal("cache unexpectedly restored")
	}
}

func TestWithdrawalSharedDigestAndReplayIsolation(t *testing.T) {
	ctx := context.Background()
	s := conversationStore(t)
	a, b := startConversation(t, s, "a"), startConversation(t, s, "b")
	da := recordDescription(t, s, a, "shared-record", 2, "Shared native recap")
	db := recordDescription(t, s, b, "shared-record", 2, "Shared native recap")
	if da.RetainedDigest != db.RetainedDigest {
		t.Fatal("fixture digest")
	}
	withdrawn := da
	withdrawn.Availability = "withdrawn"
	withdrawn.Sequence = 3
	withdrawn.SourceTime = withdrawn.SourceTime.Add(time.Second)
	withdrawn.ObservedAt = withdrawn.SourceTime
	if _, err := s.WithdrawDescription(ctx, withdrawn); err != nil {
		t.Fatal(err)
	}
	var count int
	s.db.QueryRow("SELECT count(*) FROM conversation_evidence").Scan(&count)
	if count != 0 || evidence(t, s, da).Gap != "withdrawn" || evidence(t, s, db).Gap != "evidence_missing" {
		t.Fatal("withdrawal did not purge/isolate")
	}
	if _, err := s.WithdrawDescription(ctx, withdrawn); err != nil {
		t.Fatal("withdrawal retry", err)
	}
	if _, err := s.Replay(ctx); err != nil {
		t.Fatal(err)
	}
	calls := 0
	results, err := s.Hydrate(ctx, descriptionResolverFunc(func(_ context.Context, d conversation.Description) (ResolvedDescription, error) {
		calls++
		if d.ConversationID != b {
			t.Error("withdrawn association resolved")
		}
		return ResolvedDescription{d.SourceRevision, []byte("Shared native recap")}, nil
	}), 10)
	if err != nil || calls != 1 || !results[0].Hydrated || evidence(t, s, da).Text != "" || evidence(t, s, db).Text == "" {
		t.Fatal(results, err)
	}
	if _, err := s.RecordDescription(ctx, da, []byte("Shared native recap")); err == nil {
		t.Fatal("withdrawn association reingested")
	}
	wrong := db.Source
	wrong.Namespace = "foreign"
	v, err := s.DescriptionEvidence(ctx, b, wrong, db.RecordKey, db.SourceRevision)
	if err != nil || v.Gap != "out_of_scope" || v.Text != "" {
		t.Fatal("cross-source leak", v, err)
	}
	v, err = s.DescriptionEvidence(ctx, model.NewID(), db.Source, db.RecordKey, db.SourceRevision)
	if err != nil || v.Gap != "out_of_scope" || v.Text != "" {
		t.Fatal("cross-conversation leak", v, err)
	}
	if _, err := s.Replay(ctx); err != nil {
		t.Fatal(err)
	}
	if evidence(t, s, da).Text != "" {
		t.Fatal("replay resurrected withdrawal")
	}
}

func TestHydrationDiagnosticsAndWithdrawalRace(t *testing.T) {
	ctx := context.Background()
	s := conversationStore(t)
	id := startConversation(t, s, "hydrate")
	d := recordDescription(t, s, id, "recap", 2, "native bytes\r\n")
	for _, tc := range []struct {
		name   string
		result ResolvedDescription
		err    error
		gap    string
	}{
		{"unavailable", ResolvedDescription{}, errors.New("private source details must not leak"), "source_unavailable"},
		{"original", ResolvedDescription{d.SourceRevision, []byte("native bytes\n")}, nil, "digest_mismatch"},
		{"revision", ResolvedDescription{conversation.SourceRevision([]byte("wrong")), []byte("native bytes\r\n")}, nil, "digest_mismatch"},
		{"content", ResolvedDescription{d.SourceRevision, []byte("changed")}, nil, "digest_mismatch"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			purgeEvidence(t, s)
			results, err := s.Hydrate(ctx, descriptionResolverFunc(func(context.Context, conversation.Description) (ResolvedDescription, error) { return tc.result, tc.err }), 1)
			if err != nil || len(results) != 1 || results[0].Gap != tc.gap || evidence(t, s, d).Gap != tc.gap {
				t.Fatal(results, err, evidence(t, s, d))
			}
		})
	}
	purgeEvidence(t, s)
	results, err := s.Hydrate(ctx, descriptionResolverFunc(func(_ context.Context, want conversation.Description) (ResolvedDescription, error) {
		w := want
		w.Availability = "withdrawn"
		w.Sequence++
		w.SourceTime = w.SourceTime.Add(time.Second)
		w.ObservedAt = w.SourceTime
		if _, err := s.WithdrawDescription(ctx, w); err != nil {
			t.Fatal(err)
		}
		return ResolvedDescription{want.SourceRevision, []byte("native bytes\r\n")}, nil
	}), 1)
	if err != nil || results[0].Gap != "withdrawn" || evidence(t, s, d).Text != "" {
		t.Fatal("hydration race", results, err)
	}
}

func TestDescriptionValidationAtomicityHistoryAndTruncation(t *testing.T) {
	ctx := context.Background()
	s := conversationStore(t)
	id := startConversation(t, s, "validation")
	for _, kind := range []string{"prompt", "assistant_message", "message", "agent_summary"} {
		p := descriptionFixture(id, "bad", 2)
		p.Kind = kind
		if _, err := s.RecordDescription(ctx, p, []byte("ineligible")); err == nil {
			t.Fatal("accepted", kind)
		}
	}
	d := recordDescription(t, s, id, "oversized", 2, strings.Repeat("界", 2000))
	if !d.Truncated || d.RetainedBytes != 4095 || !evidence(t, s, d).Truncated {
		t.Fatal("truncation metadata", d)
	}
	before, _ := s.State(ctx)
	events, _ := s.Events(ctx)
	// Force a SQL failure after reducer/event insertion to prove rollback includes
	// the receipt and projection as well as evidence publication.
	if _, err := s.db.Exec("CREATE TRIGGER reject_description BEFORE INSERT ON conversation_evidence BEGIN SELECT RAISE(ABORT,'fixture'); END"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordDescription(ctx, descriptionFixture(id, "rollback", 3), []byte("rollback bytes")); err == nil {
		t.Fatal("expected SQL failure")
	}
	after, _ := s.State(ctx)
	afterEvents, _ := s.Events(ctx)
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(events, afterEvents) {
		t.Fatal("SQL failure leaked event/state")
	}
	if _, err := s.db.Exec("DROP TRIGGER reject_description"); err != nil {
		t.Fatal(err)
	}
	recordDescription(t, s, id, "rollback", 3, "rollback bytes")
	for i := int64(4); i < 40; i++ {
		recordDescription(t, s, id, fmt.Sprintf("record-%d", i), i, "bounded history")
	}
	st, _ := s.State(ctx)
	if len(st.Conversations[id].DescriptionHistory) != conversation.HistoryLimit {
		t.Fatal("unbounded history references")
	}
	if _, err := s.Replay(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestConversationGoldenAndReducerRejections(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/conversation/events-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var events []Event
	if err := model.StrictJSON(raw, &events); err != nil {
		t.Fatal(err)
	}
	st := model.Empty()
	for _, e := range events {
		for _, mutate := range []func(*Event){
			func(e *Event) { e.Actor = "cli" }, func(e *Event) { e.CommandID = "forged" }, func(e *Event) { e.Version = 99 },
			func(e *Event) { e.TS = e.TS.Add(time.Second) }, func(e *Event) { e.EntityID = model.NewID() },
			func(e *Event) {
				var p map[string]any
				json.Unmarshal(e.Payload, &p)
				p["version"] = 99
				e.Payload, _ = json.Marshal(p)
			},
			func(e *Event) {
				var p map[string]any
				json.Unmarshal(e.Payload, &p)
				p["text"] = "forbidden"
				e.Payload, _ = json.Marshal(p)
			},
			func(e *Event) {
				var p map[string]any
				json.Unmarshal(e.Payload, &p)
				p["sequence"] = 0
				e.Payload, _ = json.Marshal(p)
			},
		} {
			bad := e
			mutate(&bad)
			before := snapshotJSON(t, st)
			if err := Apply(&st, bad); err == nil {
				t.Fatal("invalid golden mutation accepted", e.Verb)
			}
			if !bytes.Equal(before, snapshotJSON(t, st)) {
				t.Fatal("rejected event mutated state")
			}
		}
		if err := Apply(&st, e); err != nil {
			t.Fatal(e.Verb, err)
		}
	}
	expected, err := os.ReadFile("../../testdata/conversation/state-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var golden map[string]conversation.Record
	if err := model.StrictJSON(expected, &golden); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(snapshotJSON(t, st.Conversations), snapshotJSON(t, golden)) {
		t.Fatal("conversation state golden mismatch")
	}
}
