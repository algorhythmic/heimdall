package main

import (
	"bytes"
	"context"
	"encoding/json"
	"heimdall/internal/daemon"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStateActiveCLIRequestsLiveSelection(t *testing.T) {
	calls := 0
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || r.URL.Path != "/state" || r.URL.Query().Get("active") != "1" {
			t.Errorf("wrong active request: %s %s", r.Method, r.URL)
		}
		w.Write([]byte(`{"status":"unbound","focus":{"tab_id":7},"gaps":[]}`))
	}))
	defer s.Close()
	dir := t.TempDir()
	raw, _ := json.Marshal(daemon.Endpoint{URL: s.URL, Token: strings.Repeat("a", 64)})
	if err := os.WriteFile(filepath.Join(dir, "endpoint.json"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(context.Background(), []string{"--data-dir", dir, "state", "--active", "--json"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"unbound"`) || calls != 1 {
		t.Fatal(out.String(), calls)
	}
	if err := run(context.Background(), []string{"--data-dir", dir, "state", "--active", "alpha"}, &out); err == nil {
		t.Fatal("active accepted task argument")
	}
	if calls != 1 {
		t.Fatal("invalid arguments queried state")
	}
}
