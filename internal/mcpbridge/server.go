// Package mcpbridge maps MCP tools to the scoped daemon API; it never owns state.
package mcpbridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"heimdall/internal/client"
	"heimdall/internal/continuity"
	"heimdall/internal/model"
	"io"
	"net/url"
	"os"
	"strconv"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type Caller interface {
	Call(context.Context, string, string, any) (json.RawMessage, error)
}
type taskArgs struct {
	Target string `json:"target"`
}
type contextArgs struct {
	Target string `json:"target"`
	Budget *int   `json:"budget,omitempty"`
}
type historyArgs struct {
	Target string `json:"target"`
	Kind   string `json:"kind"`
	Limit  *int   `json:"limit,omitempty"`
	Cursor string `json:"cursor,omitempty"`
}
type checkpointArgs struct {
	Target               string   `json:"target"`
	RequestID            string   `json:"request_id"`
	ExpectedTaskRevision int64    `json:"expected_task_revision"`
	Previous             string   `json:"previous"`
	ContractID           string   `json:"contract_id"`
	Summary              string   `json:"summary"`
	CurrentStep          string   `json:"current_step,omitempty"`
	NextAction           string   `json:"next_action"`
	Blockers             []string `json:"blockers"`
}

func New(api Caller) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "heimdall", Version: "0.5.0"}, nil)
	stringField := func() any { return map[string]any{"type": "string"} }
	integerField := func() any { return map[string]any{"type": "integer"} }
	add := func(name, description string, readOnly bool, properties map[string]any, required []string, handler func(context.Context, json.RawMessage) (json.RawMessage, error)) {
		closed, destructive := false, false
		s.AddTool(&mcp.Tool{Name: name, Description: description, InputSchema: map[string]any{"type": "object", "additionalProperties": false, "properties": properties, "required": required}, Annotations: &mcp.ToolAnnotations{ReadOnlyHint: readOnly, IdempotentHint: true, DestructiveHint: &destructive, OpenWorldHint: &closed}}, func(ctx context.Context, r *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			if r.Params == nil || len(r.Params.Arguments) > continuity.MaxRequest {
				return toolError(fmt.Errorf("tool arguments exceed 64 KiB or are missing")), nil
			}
			raw, err := handler(ctx, r.Params.Arguments)
			if err != nil {
				return toolError(err), nil
			}
			return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}, StructuredContent: raw}, nil
		})
	}
	add("heimdall_task", "Read one task through the configured scoped credential.", true, map[string]any{"target": stringField()}, []string{"target"}, func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
		var a taskArgs
		if err := model.StrictJSON(raw, &a); err != nil {
			return nil, err
		}
		return api.Call(ctx, "GET", "/client/task?target="+url.QueryEscape(a.Target), nil)
	})
	add("heimdall_context", "Read mandatory task/ancestor context and authorized resource observations. A ready result is not execution authority or completion evidence.", true, map[string]any{"target": stringField(), "budget": integerField()}, []string{"target"}, func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
		var a contextArgs
		if err := model.StrictJSON(raw, &a); err != nil {
			return nil, err
		}
		budget := 16000
		if a.Budget != nil {
			budget = *a.Budget
		}
		return api.Call(ctx, "GET", "/client/context?target="+url.QueryEscape(a.Target)+"&budget="+strconv.Itoa(budget), nil)
	})
	add("heimdall_history", "Read a bounded history page (contract, decision, resource, checkpoint). Reuse next_cursor only for the same target/kind; restart after conflict.", true, map[string]any{"target": stringField(), "kind": map[string]any{"type": "string", "enum": []string{"contract", "decision", "resource", "checkpoint"}}, "limit": integerField(), "cursor": stringField()}, []string{"target", "kind"}, func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
		var a historyArgs
		if err := model.StrictJSON(raw, &a); err != nil {
			return nil, err
		}
		limit := 20
		if a.Limit != nil {
			limit = *a.Limit
		}
		q := url.Values{"target": {a.Target}, "kind": {a.Kind}, "limit": {strconv.Itoa(limit)}, "cursor": {a.Cursor}}
		return api.Call(ctx, "GET", "/client/history?"+q.Encode(), nil)
	})
	add("heimdall_checkpoint", "Record progress under an explicit checkpoint-write grant. Does not complete tasks or accept contracts/decisions. Supply a new 32-hex request_id per logical write; retry the exact arguments with the same ID after uncertain transport results. Previous must be the observed checkpoint ID or none.", false, map[string]any{"target": stringField(), "request_id": map[string]any{"type": "string", "pattern": "^[a-f0-9]{32}$"}, "expected_task_revision": integerField(), "previous": stringField(), "contract_id": stringField(), "summary": stringField(), "current_step": stringField(), "next_action": stringField(), "blockers": map[string]any{"type": "array", "items": stringField()}}, []string{"target", "request_id", "expected_task_revision", "previous", "contract_id", "summary", "next_action", "blockers"}, func(ctx context.Context, raw json.RawMessage) (json.RawMessage, error) {
		var a checkpointArgs
		if err := model.StrictJSON(raw, &a); err != nil {
			return nil, err
		}
		r := continuity.Request{Version: 1, ID: a.RequestID, Op: "checkpoint.record", Target: a.Target, ExpectedTaskRevision: &a.ExpectedTaskRevision, Checkpoint: &continuity.CheckpointInput{Previous: a.Previous, ContractID: a.ContractID, Summary: a.Summary, CurrentStep: a.CurrentStep, NextAction: a.NextAction, Blockers: a.Blockers}}
		if err := r.Validate(); err != nil {
			return nil, err
		}
		return api.Call(ctx, "POST", "/client/checkpoint", r)
	})
	return s
}

func toolError(err error) *mcp.CallToolResult {
	code, message, retryable := "invalid_request", err.Error(), false
	var details any
	var api *client.APIError
	if errors.As(err, &api) {
		code = api.Code
		message = api.Message
		retryable = code == "daemon_unavailable"
		if len(api.Details) > 0 {
			details = api.Details
		}
	}
	body := map[string]any{"code": code, "message": message, "retryable_with_same_request": retryable}
	if details != nil {
		body["details"] = details
	}
	raw, _ := json.Marshal(body)
	return &mcp.CallToolResult{IsError: true, Content: []mcp.Content{&mcp.TextContent{Text: string(raw)}}, StructuredContent: body}
}

// Line bound is a byte-stream guard; the official SDK owns JSON-RPC and MCP.
type boundedInput struct {
	io.ReadCloser
	size int
}

func (r *boundedInput) Read(p []byte) (int, error) {
	n, err := r.ReadCloser.Read(p)
	for i, b := range p[:n] {
		if b == '\n' {
			r.size = 0
		} else {
			r.size++
			if r.size > 128<<10 {
				return i, fmt.Errorf("MCP input line exceeds 128 KiB")
			}
		}
	}
	return n, err
}
func Serve(ctx context.Context, credential string) error {
	api, err := client.New(credential)
	if err != nil {
		return err
	}
	return New(api).Run(ctx, &mcp.IOTransport{Reader: &boundedInput{ReadCloser: os.Stdin}, Writer: os.Stdout})
}
