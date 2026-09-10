package main

import (
	"bytes"
	"context"
	"encoding/json"
	"heimdall/internal/daemon"
	"heimdall/internal/store"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConversationsCLIAndTerminalEscaping(t *testing.T) {
	calls := 0
	views := []store.ConversationView{{ID: strings.Repeat("a", 32), Kind: "codex", Lifecycle: "inactive-by-policy", TranscriptGap: "not_observed", Description: store.DescriptionEvidence{Availability: "available", Kind: "recap", Digest: strings.Repeat("b", 64), Truncated: true, Text: "native\x1b]0;title\a\u202erecap\n"}}}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/conversations" || r.URL.Query().Get("target") != "fixture#step" || r.Header.Get("Authorization") != "Bearer "+strings.Repeat("a", 64) {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL)
		}
		json.NewEncoder(w).Encode(views)
	}))
	defer s.Close()
	dir := t.TempDir()
	raw, _ := json.Marshal(daemon.Endpoint{URL: s.URL, Token: strings.Repeat("a", 64)})
	if err := os.WriteFile(filepath.Join(dir, "endpoint.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	for _, flags := range [][]string{{}, {"--json"}} {
		var out bytes.Buffer
		args := append([]string{"--data-dir", dir, "conversations", "fixture#step"}, flags...)
		if err := run(context.Background(), args, &out); err != nil {
			t.Fatal(err)
		}
		if strings.ContainsAny(out.String(), "\x1b\a\u202e") || !strings.Contains(out.String(), "\\u202e") || !strings.Contains(out.String(), "recap") {
			t.Fatal("unsafe rendering", out.String())
		}
	}
	if calls != 2 {
		t.Fatal("CLI did not use daemon")
	}
	if err := run(context.Background(), []string{"--data-dir", dir, "conversations", "a", "b"}, &bytes.Buffer{}); err == nil {
		t.Fatal("unexpected arguments accepted")
	}
	if calls != 2 {
		t.Fatal("invalid command queried daemon")
	}
	var out bytes.Buffer
	if err := renderConversations(&out, []store.ConversationView{{ID: "fixture", Description: store.DescriptionEvidence{Availability: "withdrawn", Gap: "withdrawn"}}}); err != nil || !strings.Contains(out.String(), "Evidence gap: withdrawn") {
		t.Fatal(out.String(), err)
	}
}
