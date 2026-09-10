package actions_test

import (
	"encoding/json"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"heimdall/internal/actions"
	"heimdall/internal/authz"
	"heimdall/internal/client"
	"heimdall/internal/daemon"
	"heimdall/internal/mcpbridge"
	"heimdall/internal/model"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestExternalMCPThroughScopedHTTP(t *testing.T) {
	x := externalSetup(t)
	server := &daemon.Server{Engine: x.e, External: x.service, Clock: time.Now}
	httpServer := httptest.NewUnstartedServer(server)
	server.Host = httpServer.Listener.Addr().String()
	httpServer.Start()
	defer httpServer.Close()
	dir := t.TempDir()
	raw, _ := json.Marshal(daemon.Endpoint{URL: httpServer.URL, PID: os.Getpid()})
	os.WriteFile(filepath.Join(dir, "client-endpoint.json"), raw, 0600)
	cred := client.Credential{Version: 1, DataDir: dir, Token: x.token, Issue: authz.Request{Grant: &authz.IssueInput{TokenHash: authz.HashToken(x.token)}}}
	raw, _ = json.Marshal(cred)
	path := filepath.Join(dir, "credential.json")
	os.WriteFile(path, raw, 0600)
	api, err := client.New(path)
	if err != nil {
		t.Fatal(err)
	}
	a, b := mcp.NewInMemoryTransports()
	ss, err := mcpbridge.New(api).Connect(x.ctx, a, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer ss.Close()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "external-fixture", Version: "1"}, nil).Connect(x.ctx, b, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	call := func(name string, args any, failed bool) *mcp.CallToolResult {
		t.Helper()
		r, err := session.CallTool(x.ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil || r.IsError != failed {
			t.Fatal(name, r, err)
		}
		return r
	}
	// Wire carries only user request fields, never actor/observed provenance.
	intent := map[string]any{"version": 1, "id": x.request.ID, "target": "alpha", "purpose": "MCP focus fixture", "steps": 2, "expected": map[string]string{"kind": "window_focused"}}
	first := call("heimdall_intent", intent, false)
	again := call("heimdall_intent", intent, false)
	left, _ := json.Marshal(first.StructuredContent)
	right, _ := json.Marshal(again.StructuredContent)
	if string(left) != string(right) {
		t.Fatal("intent retry drift")
	}
	bad := map[string]any{"version": 1, "id": model.NewID(), "target": "beta", "purpose": "foreign", "expected": map[string]string{"kind": "window_focused"}}
	call("heimdall_intent", bad, true)
	for i := 1; i <= 2; i++ {
		report := actions.ReportRequest{Version: 1, ID: model.NewID(), Target: "alpha", IntentID: x.request.ID, Step: i, Outcome: "succeeded"}
		call("heimdall_report", report, false)
	}
	if got := x.action(x.request.ID).Verification; got != "pending" {
		t.Fatal("MCP report verified", got)
	}
	if err := x.service.Observe(x.ctx, x.request.ID); err != nil {
		t.Fatal(err)
	}
	if got := x.action(x.request.ID).Verification; got != "matched" {
		t.Fatal(got)
	}
	if _, err := (authz.Service{Store: x.e.Store}).Execute(x.ctx, authz.Request{Version: 1, ID: model.NewID(), Op: "grant.revoke", GrantID: x.grant}, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}
	call("heimdall_report", actions.ReportRequest{Version: 1, ID: model.NewID(), Target: "alpha", IntentID: x.request.ID, Step: 2, Outcome: "succeeded"}, true)
	x.replay()
}
