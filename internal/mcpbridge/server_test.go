package mcpbridge

import (
	"context"
	"encoding/json"
	"heimdall/internal/authz"
	"heimdall/internal/client"
	"heimdall/internal/continuity"
	"heimdall/internal/core"
	"heimdall/internal/daemon"
	"heimdall/internal/model"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestOfficialSDKClientThroughScopedDaemon(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	dir := t.TempDir()
	e, err := core.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	now := time.Now().UTC()
	task := model.Task{ID: "mcp-fixture", Title: "MCP fixture", Type: "project", Status: "active"}
	if _, err = e.Execute(ctx, core.Command{ID: model.NewID(), Op: "add", Task: &task}, "cli", now); err != nil {
		t.Fatal(err)
	}
	st, _ := e.Store.State(ctx)
	revision := st.Tasks[task.ID].Revision
	contract := model.NewID()
	_, err = (continuity.Service{Store: e.Store}).Execute(ctx, continuity.Request{Version: 1, ID: contract, Op: "contract.accept", Target: task.ID, ExpectedTaskRevision: &revision, Contract: &continuity.ContractInput{Previous: "none", Objective: "Verify MCP"}}, "cli", now)
	if err != nil {
		t.Fatal(err)
	}
	token := strings.Repeat("c", 64)
	grant := model.NewID()
	issue := authz.Request{Version: 1, ID: grant, Op: "grant.issue", Grant: &authz.IssueInput{Name: "MCP fixture", Target: task.ID, CheckpointWrite: true, TokenHash: authz.HashToken(token), ExpiresAt: now.Add(time.Hour)}}
	if _, err = (authz.Service{Store: e.Store}).Execute(ctx, issue, now); err != nil {
		t.Fatal(err)
	}
	service := &daemon.Server{Engine: e, Token: strings.Repeat("a", 64), BrowserToken: strings.Repeat("b", 64), Clock: time.Now}
	httpServer := httptest.NewUnstartedServer(service)
	service.Host = httpServer.Listener.Addr().String()
	httpServer.Start()
	defer httpServer.Close()
	endpoint, _ := json.Marshal(map[string]any{"url": httpServer.URL, "token": "", "pid": os.Getpid()})
	os.WriteFile(filepath.Join(dir, "client-endpoint.json"), endpoint, 0600)
	credential := filepath.Join(dir, "fixture.credential.json")
	raw, _ := json.Marshal(client.Credential{Version: 1, DataDir: dir, Token: token, Issue: issue})
	os.WriteFile(credential, raw, 0600)
	api, err := client.New(credential)
	if err != nil {
		t.Fatal(err)
	}
	serverTransport, clientTransport := mcp.NewInMemoryTransports()
	serverSession, err := New(api).Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer serverSession.Close()
	session, err := mcp.NewClient(&mcp.Implementation{Name: "heimdall-test", Version: "1"}, nil).Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if version := session.InitializeResult().ProtocolVersion; version != "2026-07-28" {
		t.Fatal("unexpected SDK negotiation", version)
	}
	tools, err := session.ListTools(ctx, nil)
	if err != nil || len(tools.Tools) != 4 {
		t.Fatal("tool discovery", tools, err)
	}
	call := func(name string, args any, wantError bool) *mcp.CallToolResult {
		result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
		if err != nil {
			t.Fatal(name, err)
		}
		if result.IsError != wantError {
			t.Fatal(name, result)
		}
		return result
	}
	call("heimdall_task", map[string]any{"target": task.ID}, false)
	denied := call("heimdall_task", map[string]any{"target": "outside-project"}, true)
	deniedRaw, _ := json.Marshal(denied.StructuredContent)
	if !strings.Contains(string(deniedRaw), "access_denied") {
		t.Fatal("unstructured denial")
	}
	call("heimdall_context", map[string]any{"target": task.ID, "budget": 1}, true)
	call("heimdall_task", map[string]any{"target": task.ID, "actor": "cli"}, true)
	request := model.NewID()
	args := map[string]any{"target": task.ID, "request_id": request, "expected_task_revision": revision, "previous": "none", "contract_id": contract, "summary": "Progress recorded through MCP", "next_action": "Inspect result", "blockers": []string{}}
	first := call("heimdall_checkpoint", args, false)
	again := call("heimdall_checkpoint", args, false)
	a, _ := json.Marshal(first.StructuredContent)
	b, _ := json.Marshal(again.StructuredContent)
	if string(a) != string(b) {
		t.Fatal("MCP retry changed receipt")
	}
	st, _ = e.Store.State(ctx)
	if st.Checkpoints[request].Actor != "client:"+grant || st.Tasks[task.ID].Task.Status != "active" {
		t.Fatal("provenance or task state")
	}
	if _, err = core.Open(dir); err == nil {
		t.Fatal("second writer acquired daemon database")
	}
	call("heimdall_history", map[string]any{"target": task.ID, "kind": "checkpoint", "limit": 1}, false)
	_, err = (authz.Service{Store: e.Store}).Execute(ctx, authz.Request{Version: 1, ID: model.NewID(), Op: "grant.revoke", GrantID: grant}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	call("heimdall_checkpoint", args, true)
	// Port/daemon loss is a structured tool error; no implicit write retry.
	os.Remove(filepath.Join(dir, "client-endpoint.json"))
	lost := call("heimdall_task", map[string]any{"target": task.ID}, true)
	body, _ := json.Marshal(lost.StructuredContent)
	if !strings.Contains(string(body), "daemon_unavailable") {
		t.Fatal("missing disconnected error")
	}
}

func TestMCPInputLineBound(t *testing.T) {
	reader := &boundedInput{ReadCloser: io.NopCloser(strings.NewReader(strings.Repeat("x", (128<<10)+1)))}
	if _, err := io.ReadAll(reader); err == nil {
		t.Fatal("oversized line accepted")
	}
	reader = &boundedInput{ReadCloser: io.NopCloser(strings.NewReader(strings.Repeat("x", 64000) + "\n" + strings.Repeat("x", 64000) + "\n"))}
	if _, err := io.ReadAll(reader); err != nil {
		t.Fatal("line budget did not reset", err)
	}
}
