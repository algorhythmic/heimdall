package store

import (
	"context"
	"testing"
	"time"

	"heimdall/internal/conversation"
)

func testRoot(now time.Time) conversation.SourceRoot {
	return conversation.SourceRoot{Version: 1, ID: "0123456789abcdef0123456789abcdef", Provider: "claude_code",
		Root: "/home/u/.claude/projects", Namespace: "sr1:namespace:" + conversation.Digest([]byte("ns")),
		Host: "host", RegisteredAt: now}
}

func testStream(root conversation.SourceRoot, now time.Time) conversation.Source {
	return conversation.Source{Version: 1, RootID: root.ID, Provider: "claude_code", Root: root.Root,
		Path: "/home/u/.claude/projects/p/abc.jsonl", NativeID: "native-1", Evidence: "native_id",
		RegisteredAt: now, Key: conversation.SourceKey{AdapterID: "skald/sessioncapture",
			ContractMajor: 1, ContractMinor: 0, Namespace: root.Namespace, LogicalStream: "sr1:stream:" + conversation.Digest([]byte("s1"))}}
}

func TestSourceRootAndStreamLifecycle(t *testing.T) {
	now := time.Now().UTC()
	s := openTestStore(t)
	root := testRoot(now)
	if _, err := s.RegisterSourceRoot(context.Background(), root, now); err != nil {
		t.Fatal(err)
	}
	stream := testStream(root, now)
	if _, err := s.RegisterStream(context.Background(), stream, now); err != nil {
		t.Fatal(err)
	}
	cp := conversation.Checkpoint{Version: 1, Generation: "g1", Epoch: 1, Offset: 100, ParsedOffset: 100, Ordinal: 3,
		PrefixDigest: conversation.SourceRevision([]byte("p")), ConversationID: "native-1"}
	if _, err := s.AdvanceCheckpoint(context.Background(), stream.ID(), cp, now.Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RecordGap(context.Background(), stream.ID(), conversation.Gap{Version: 1, Code: "unparseable_line", Offset: 10, Ordinal: 1, Generation: "g1"}, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MarkSourceLost(context.Background(), stream.ID(), "file_removed", now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	st, _ := s.State(context.Background())
	if len(st.SourceRoots) != 1 || len(st.SessionSources) != 1 {
		t.Fatalf("sources: %d roots %d streams", len(st.SourceRoots), len(st.SessionSources))
	}
	got := st.SessionSources[stream.ID()]
	if got.Active || got.Lost != "file_removed" || got.Checkpoint.Ordinal != 3 {
		t.Fatalf("stream state %+v", got)
	}
}

func TestStreamRegistrationRequiresRootAndDedupes(t *testing.T) {
	now := time.Now().UTC()
	s := openTestStore(t)
	stream := testStream(testRoot(now), now)
	if _, err := s.RegisterStream(context.Background(), stream, now); err == nil {
		t.Fatal("stream registration without a configured root must fail")
	}
	root := testRoot(now)
	if _, err := s.RegisterSourceRoot(context.Background(), root, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.RegisterStream(context.Background(), stream, now); err != nil {
		t.Fatal(err)
	}
	// Re-registering the identical stream dedupes to the stored result.
	if _, err := s.RegisterStream(context.Background(), stream, now); err != nil {
		t.Fatal(err)
	}
	st, _ := s.State(context.Background())
	if len(st.SessionSources) != 1 {
		t.Fatal("duplicate stream registration")
	}
	moved := stream
	moved.Path = "/home/u/.claude/projects/elsewhere/abc.jsonl"
	if _, err := s.RegisterStream(context.Background(), moved, now.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	st, _ = s.State(context.Background())
	if st.SessionSources[stream.ID()].Path != moved.Path {
		t.Fatal("relocation did not update the stream locator")
	}
}

func TestCheckpointMonotonicityAndLostAuthority(t *testing.T) {
	now := time.Now().UTC()
	s := openTestStore(t)
	root := testRoot(now)
	_, _ = s.RegisterSourceRoot(context.Background(), root, now)
	stream := testStream(root, now)
	_, _ = s.RegisterStream(context.Background(), stream, now)
	cp := func(ord, off int64) conversation.Checkpoint {
		return conversation.Checkpoint{Version: 1, Generation: "g1", Epoch: 1, Offset: off, ParsedOffset: off, Ordinal: ord,
			PrefixDigest: conversation.SourceRevision([]byte("p")), ConversationID: "native-1"}
	}
	if _, err := s.AdvanceCheckpoint(context.Background(), stream.ID(), cp(5, 50), now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.AdvanceCheckpoint(context.Background(), stream.ID(), cp(3, 30), now.Add(time.Second)); err == nil {
		t.Fatal("regressing checkpoint must fail")
	}
	if _, err := s.MarkSourceLost(context.Background(), stream.ID(), "file_removed", now.Add(2*time.Second)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.MarkSourceLost(context.Background(), stream.ID(), "file_removed", now.Add(3*time.Second)); err == nil {
		t.Fatal("double loss must fail")
	}
	if _, err := s.AdvanceCheckpoint(context.Background(), stream.ID(), cp(9, 90), now.Add(4*time.Second)); err == nil {
		t.Fatal("checkpoint on lost stream must fail")
	}
}

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}
