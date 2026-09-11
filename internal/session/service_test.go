package session

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"heimdall/internal/conversation"
	"heimdall/internal/store"
)

func claudeLine(kind, extra string) string {
	return `{"type":"` + kind + `","sessionId":"conv-1","uuid":"` + extra + `","version":"2.0.0","cwd":"/repo","timestamp":"2026-09-10T10:00:00.000Z",` + extra + `}`
}

func TestSessionIngest(t *testing.T) {
	now := time.Now().UTC()
	dir := t.TempDir()
	rootDir := filepath.Join(dir, "projects")
	if err := os.MkdirAll(filepath.Join(rootDir, "-repo"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join("-repo", "conv-1.jsonl")
	lines := []string{
		`{"type":"user","sessionId":"conv-1","uuid":"u1","version":"2.0.0","cwd":"/repo","timestamp":"2026-09-10T10:00:00.000Z","message":{"role":"user","content":[{"type":"text","text":"hi"}]}}`,
		`{"type":"ai-title","sessionId":"conv-1","aiTitle":"Ship the feature","timestamp":"2026-09-10T10:01:00.000Z"}`,
		`{"type":"assistant","sessionId":"conv-1","uuid":"u2","version":"2.0.0","timestamp":"2026-09-10T10:02:00.000Z","message":{"role":"assistant","content":[{"type":"text","text":"working"}]}}`,
	}
	writeLines := func(ls []string) {
		data := ""
		for _, l := range ls {
			data += l + "\n"
		}
		if err := os.WriteFile(filepath.Join(rootDir, file), []byte(data), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	writeLines(lines)

	s, err := store.Open(filepath.Join(dir, "store"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	host, _ := os.Hostname()
	root := conversation.SourceRoot{Version: 1, ID: "0123456789abcdef0123456789abcdef", Provider: "claude_code",
		Root: rootDir, Namespace: "sr1:namespace:" + conversation.Digest([]byte("ns")), Host: host, RegisteredAt: now}
	if _, err := s.RegisterSourceRoot(context.Background(), root, now); err != nil {
		t.Fatal(err)
	}
	svc := Service{Store: s}
	svc.poll(context.Background(), now)

	st, err := s.State(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(st.SessionSources) != 1 {
		t.Fatalf("expected 1 stream, got %d", len(st.SessionSources))
	}
	if len(st.Conversations) != 1 {
		t.Fatalf("expected 1 conversation, got %d", len(st.Conversations))
	}
	var conv conversation.Record
	for _, c := range st.Conversations {
		conv = c
	}
	if conv.Kind != "claude_code" || conv.NativeConversationID != "conv-1" || conv.StartedAt.Format("2006-01-02") != "2026-09-10" {
		t.Fatalf("bad conversation %+v", conv.Started)
	}
	if conv.CurrentDescription == nil || conv.CurrentDescription.Kind != "title" {
		t.Fatalf("expected title description, got %+v", conv.CurrentDescription)
	}
	for _, src := range st.SessionSources {
		if src.Checkpoint.Ordinal != 3 || src.Checkpoint.Offset == 0 {
			t.Fatalf("checkpoint not advanced: %+v", src.Checkpoint)
		}
	}

	// Second poll: idempotent — no new events, checkpoint unchanged.
	before := len(st.Conversations)
	svc.poll(context.Background(), now.Add(time.Second))
	st, _ = s.State(context.Background())
	if len(st.Conversations) != before {
		t.Fatal("idle poll created new conversations")
	}

	// Growth: a recap arrives mid-stream and is observed.
	lines = append(lines,
		`{"type":"system","subtype":"away_summary","sessionId":"conv-1","content":"Built the thing and shipped it","timestamp":"2026-09-10T10:05:00.000Z"}`,
		`{"type":"user","sessionId":"conv-1","uuid":"u3","version":"2.0.0","timestamp":"2026-09-10T10:06:00.000Z","message":{"role":"user","content":[{"type":"text","text":"next"}]}}`)
	writeLines(lines)
	svc.poll(context.Background(), now.Add(2*time.Second))
	st, _ = s.State(context.Background())
	for _, c := range st.Conversations {
		if c.CurrentDescription == nil || c.CurrentDescription.Kind != "recap" {
			t.Fatalf("expected recap description, got %+v", c.CurrentDescription)
		}
	}
	for _, src := range st.SessionSources {
		if src.Checkpoint.Ordinal != 5 {
			t.Fatalf("checkpoint did not advance to 5: %+v", src.Checkpoint)
		}
	}

	// Loss: removing the file marks the stream lost, not deleted.
	if err := os.Remove(filepath.Join(rootDir, file)); err != nil {
		t.Fatal(err)
	}
	svc.poll(context.Background(), now.Add(3*time.Second))
	st, _ = s.State(context.Background())
	for _, src := range st.SessionSources {
		if src.Active || src.Lost != "file_removed" {
			t.Fatalf("expected lost stream, got %+v", src)
		}
	}
}
