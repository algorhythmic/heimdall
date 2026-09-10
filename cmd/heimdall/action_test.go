package main

import (
	"bytes"
	"context"
	"encoding/json"
	"heimdall/internal/actions"
	"heimdall/internal/daemon"
	"heimdall/internal/model"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCancelIntentCLIUsesStableRequest(t *testing.T) {
	intent, request := model.NewID(), model.NewID()
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var got actions.CancelRequest
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Error(err)
		}
		if r.Method != "POST" || r.URL.Path != "/action/cancel" || got.Version != 2 || got.ID != request || got.ActionID != intent || got.Target != "" || got.ExpectedRevision != 0 {
			t.Errorf("wrong cancellation: %+v %s", got, r.URL)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"cancel_requested":true}`))
	}))
	defer server.Close()
	dir := t.TempDir()
	raw, _ := json.Marshal(daemon.Endpoint{URL: server.URL, Token: strings.Repeat("a", 64)})
	if err := os.WriteFile(filepath.Join(dir, "endpoint.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		var out bytes.Buffer
		if err := run(context.Background(), []string{"action", "cancel", intent, "--request-id", request, "--data-dir", dir}, &out); err != nil {
			t.Fatal(err)
		}
	}
	if calls != 2 {
		t.Fatal(calls)
	}
}
