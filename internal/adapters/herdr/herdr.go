// Package herdr implements the explicitly selected local Herdr 0.8.2 / protocol
// 20 API. It never uses focused-pane or inherited caller-context fallbacks.
package herdr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"time"
)

const MaxResponse = 1 << 20

type Error struct {
	Code   string
	Detail string
}

func (e *Error) Error() string       { return e.Code + ": " + e.Detail }
func fail(code, detail string) error { return &Error{code, detail} }

type AgentSession struct{ Source, Agent, Kind, Value string }

// AgentInfo is one entry of an agent.list inventory. It reports Herdr's own
// agent detection; it never attests task ownership or action authority.
type AgentInfo struct {
	Agent          string        `json:"agent"`
	AgentSession   *AgentSession `json:"agent_session"`
	AgentStatus    string        `json:"agent_status"`
	Cwd            string        `json:"cwd"`
	ForegroundCwd  string        `json:"foreground_cwd"`
	Focused        bool          `json:"focused"`
	PaneID         string        `json:"pane_id"`
	Revision       int           `json:"revision"`
	StateChangeSeq int64         `json:"state_change_seq"`
	TabID          string        `json:"tab_id"`
	TerminalID     string        `json:"terminal_id"`
	TerminalTitle  string        `json:"terminal_title"`
	WorkspaceID    string        `json:"workspace_id"`
}
type Pane struct {
	ID            string            `json:"pane_id"`
	TerminalID    string            `json:"terminal_id"`
	WorkspaceID   string            `json:"workspace_id"`
	TabID         string            `json:"tab_id"`
	ForegroundCwd string            `json:"foreground_cwd"`
	AgentSession  *AgentSession     `json:"agent_session"`
	Title         string            `json:"title"`
	Tokens        map[string]string `json:"tokens"`
}

type Observation struct {
	Locator model.SessionLocator `json:"locator"`
	Herdr   model.HerdrIdentity  `json:"herdr"`
}

type Adapter struct {
	// Injection is package-private: daemon callers cannot submit observations.
	process func(int) (string, string, error)
}

type connection struct{ socket, epoch, host string }

func (c *connection) call(ctx context.Context, method string, params any, out any) error {
	conn, socket, epoch, host, err := dial(ctx, c.socket)
	if err != nil {
		return err
	}
	defer conn.Close()
	if c.epoch != "" && (c.epoch != epoch || c.host != host || c.socket != socket) {
		return fail("source_changed", "Herdr server or socket identity changed")
	}
	c.socket, c.epoch, c.host = socket, epoch, host
	deadline := time.Now().Add(2 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	if err := conn.SetDeadline(deadline); err != nil {
		return err
	}
	id := model.NewID()
	if err := json.NewEncoder(conn).Encode(map[string]any{"id": id, "method": method, "params": params}); err != nil {
		return fail("disconnected", "Herdr request could not be sent")
	}
	// Herdr closes each ordinary RPC connection after its single response.
	body, err := io.ReadAll(io.LimitReader(conn, MaxResponse+1))
	if err != nil {
		return fail("disconnected", "Herdr response unavailable")
	}
	if len(body) > MaxResponse {
		return fail("response_too_large", "Herdr response exceeds 1 MiB")
	}
	var envelope struct {
		ID     string          `json:"id"`
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    string `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := model.StrictJSON(body, &envelope); err != nil || envelope.ID != id || (len(envelope.Result) > 0) == (envelope.Error != nil) {
		return fail("invalid_response", "invalid Herdr response envelope")
	}
	if envelope.Error != nil {
		return fail("herdr_"+envelope.Error.Code, "Herdr refused "+method)
	}
	// The pinned provider protocol may add unrelated display fields. Required
	// identity fields are validated separately; Heimdall event decoders stay strict.
	if err := json.Unmarshal(envelope.Result, out); err != nil {
		return fail("invalid_response", "invalid Herdr result")
	}
	return nil
}

func (c *connection) pane(ctx context.Context, id string) (Pane, error) {
	var r struct {
		Type string `json:"type"`
		Pane Pane   `json:"pane"`
	}
	err := c.call(ctx, "pane.get", map[string]string{"pane_id": id}, &r)
	if err == nil && (r.Type != "pane_info" || r.Pane.ID != id || r.Pane.TerminalID == "" || r.Pane.WorkspaceID == "" || r.Pane.TabID == "") {
		err = fail("identity_changed", "Herdr did not return the exact requested pane")
	}
	return r.Pane, err
}

func samePane(a, b Pane) bool {
	return a.ID == b.ID && a.TerminalID == b.TerminalID && a.WorkspaceID == b.WorkspaceID && a.TabID == b.TabID && a.ForegroundCwd == b.ForegroundCwd && reflect.DeepEqual(a.AgentSession, b.AgentSession)
}

func (a Adapter) Observe(ctx context.Context, socket, pane, epoch string) (Observation, error) {
	var o Observation
	if socket == "" || pane == "" {
		return o, fail("invalid_selector", "explicit socket and pane ID required")
	}
	c := connection{socket: socket}
	var ping struct {
		Type     string `json:"type"`
		Version  string `json:"version"`
		Protocol int    `json:"protocol"`
	}
	if err := c.call(ctx, "ping", struct{}{}, &ping); err != nil {
		return o, err
	}
	if epoch != "" && c.epoch != epoch {
		return o, fail("source_changed", "Herdr server instance changed; explicitly rebind")
	}
	if ping.Type != "pong" || ping.Protocol != 20 || ping.Version != "0.8.2" {
		return o, fail("unsupported_herdr", "requires verified Herdr 0.8.2 / protocol 20")
	}
	p, err := c.pane(ctx, pane)
	if err != nil {
		return o, err
	}
	var info struct {
		Type    string `json:"type"`
		Process struct {
			PaneID   string `json:"pane_id"`
			ShellPID int    `json:"shell_pid"`
		} `json:"process_info"`
	}
	if err = c.call(ctx, "pane.process_info", map[string]string{"pane_id": pane}, &info); err != nil {
		return o, err
	}
	if info.Type != "pane_process_info" || info.Process.PaneID != pane || info.Process.ShellPID < 1 {
		return o, fail("process_unavailable", "Herdr did not identify the pane process")
	}
	process := a.process
	if process == nil {
		process = processInfo
	}
	start, cwd, err := process(info.Process.ShellPID)
	if err != nil {
		return o, err
	}
	foreground, err := canonicalDir(p.ForegroundCwd)
	if err != nil || foreground != cwd {
		return o, fail("cwd_unconfirmed", "pane foreground and live shell working directories do not agree")
	}
	repo, worktree, err := gitIdentity(ctx, cwd)
	if err != nil {
		return o, err
	}
	endStart, endCwd, err := process(info.Process.ShellPID)
	if err != nil {
		return o, err
	}
	p2, err := c.pane(ctx, pane)
	if err != nil {
		return o, err
	}
	if !samePane(p, p2) || start != endStart || cwd != endCwd {
		return o, fail("identity_changed", "pane or process changed during observation")
	}
	o.Locator = model.SessionLocator{Adapter: "herdr", Environment: "local", Host: c.host, SourceEpoch: c.epoch, SessionID: c.socket, WorkspaceID: p.WorkspaceID, PaneID: p.ID, Platform: "linux", Cwd: cwd, Repository: repo, Worktree: worktree}
	if p.AgentSession != nil {
		if p.AgentSession.Kind != "id" || p.AgentSession.Value == "" {
			return o, fail("agent_session_unconfirmed", "agent session has no explicit ID")
		}
		o.Locator.AgentSessionID = p.AgentSession.Value
	}
	o.Herdr = model.HerdrIdentity{Protocol: ping.Protocol, ServerVersion: ping.Version, TerminalID: p.TerminalID, TabID: p.TabID, ShellPID: info.Process.ShellPID, ShellStart: start}
	if p.AgentSession != nil {
		o.Herdr.AgentSource = p.AgentSession.Source
		o.Herdr.AgentKind = p.AgentSession.Agent
	}
	if err := model.ValidHerdrIdentity(o.Locator, o.Herdr); err != nil {
		return Observation{}, fail("invalid_response", err.Error())
	}
	return o, nil
}

func (a Adapter) Report(ctx context.Context, o Observation, source string, seq int64, title string, tokens map[string]string, ttl int, workspaceSummary bool) error {
	c := connection{socket: o.Locator.SessionID, epoch: o.Locator.SourceEpoch, host: o.Locator.Host}
	var result struct {
		Type string `json:"type"`
	}
	params := map[string]any{"pane_id": o.Locator.PaneID, "source": source, "seq": seq, "title": title, "tokens": tokens, "ttl_ms": ttl}
	if err := c.call(ctx, "pane.report_metadata", params, &result); err != nil {
		return err
	}
	if result.Type != "ok" {
		return fail("metadata_unconfirmed", "Herdr did not acknowledge metadata")
	}
	p, err := c.pane(ctx, o.Locator.PaneID)
	if err != nil {
		return err
	}
	if p.TerminalID != o.Herdr.TerminalID || p.WorkspaceID != o.Locator.WorkspaceID || p.TabID != o.Herdr.TabID || p.Title != title {
		return fail("metadata_unconfirmed", "metadata target or title changed before readback")
	}
	for k, v := range tokens {
		if p.Tokens[k] != v {
			return fail("metadata_unconfirmed", "metadata readback differs")
		}
	}
	if workspaceSummary {
		// This explicitly labels ONE selected pane. It does not assign ownership
		// to other panes or overwrite Herdr's workspace label/runtime status.
		summary := map[string]string{"heimdall_summary": o.Locator.PaneID + " | " + tokens["heimdall_title"], "heimdall_next": tokens["heimdall_next"], "heimdall_review": tokens["heimdall_review"], "heimdall_checked_at": tokens["heimdall_checked_at"]}
		if err := c.call(ctx, "workspace.report_metadata", map[string]any{"workspace_id": o.Locator.WorkspaceID, "source": source, "seq": seq, "tokens": summary, "ttl_ms": ttl}, &result); err != nil {
			return err
		}
		if result.Type != "ok" {
			return fail("metadata_unconfirmed", "workspace summary was not acknowledged")
		}
		var wr struct {
			Type      string `json:"type"`
			Workspace struct {
				ID     string            `json:"workspace_id"`
				Tokens map[string]string `json:"tokens"`
			} `json:"workspace"`
		}
		if err := c.call(ctx, "workspace.get", map[string]string{"workspace_id": o.Locator.WorkspaceID}, &wr); err != nil {
			return err
		}
		if wr.Type != "workspace_info" || wr.Workspace.ID != o.Locator.WorkspaceID {
			return fail("metadata_unconfirmed", "workspace summary target changed")
		}
		for k, v := range summary {
			if wr.Workspace.Tokens[k] != v {
				return fail("metadata_unconfirmed", "workspace summary readback differs")
			}
		}
	}
	return nil
}

// Agents returns the live agent inventory of one epoch-checked Herdr session.
// Observations are attributed to the exact socket/host/epoch of a bound
// session; a changed server instance refuses rather than cross-attributing.
func (a Adapter) Agents(ctx context.Context, socket, epoch, host string) ([]AgentInfo, error) {
	if socket == "" {
		return nil, fail("invalid_selector", "explicit socket required")
	}
	c := connection{socket: socket, epoch: epoch, host: host}
	var r struct {
		Type   string      `json:"type"`
		Agents []AgentInfo `json:"agents"`
	}
	if err := c.call(ctx, "agent.list", struct{}{}, &r); err != nil {
		return nil, err
	}
	if r.Type != "agent_list" {
		return nil, fail("invalid_response", "unexpected agent list type")
	}
	for _, agent := range r.Agents {
		if agent.PaneID == "" || agent.WorkspaceID == "" || agent.TerminalID == "" || agent.Agent == "" ||
			agent.AgentSession == nil || agent.AgentSession.Kind != "id" || agent.AgentSession.Value == "" ||
			agent.StateChangeSeq < 1 || !validAgentStatus(agent.AgentStatus) {
			return nil, fail("invalid_response", "agent entry lacks explicit identity or status")
		}
	}
	return r.Agents, nil
}

func validAgentStatus(s string) bool {
	switch s {
	case "idle", "working", "blocked", "done", "unknown":
		return true
	}
	return false
}

func canonicalDir(s string) (string, error) {
	if !filepath.IsAbs(s) {
		return "", fail("cwd_unconfirmed", "absolute directory unavailable")
	}
	path, err := filepath.EvalSymlinks(s)
	if err != nil {
		return "", fail("cwd_unavailable", "directory cannot be resolved")
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "", fail("cwd_unavailable", "directory unavailable")
	}
	return filepath.Clean(path), nil
}

type boundedBuffer struct{ bytes.Buffer }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 16384 {
		return 0, fmt.Errorf("Git identity output too large")
	}
	return b.Buffer.Write(p)
}

func gitIdentity(ctx context.Context, cwd string) (string, string, error) {
	// Check only paths, not history, hooks, status, credentials or remote state.
	ctx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "--no-optional-locks", "-C", cwd, "rev-parse", "--path-format=absolute", "--show-toplevel", "--git-common-dir")
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "LANG=C", "LC_ALL=C", "GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_SYSTEM=" + os.DevNull, "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0"}
	var out, stderr boundedBuffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// A malformed/inaccessible .git must not be mistaken for non-Git work.
		for path := cwd; ; path = filepath.Dir(path) {
			_, e := os.Lstat(filepath.Join(path, ".git"))
			if !os.IsNotExist(e) {
				return "", "", fail("repository_unavailable", "Git identity cannot be read")
			}
			if filepath.Dir(path) == path {
				break
			}
		}
		if strings.Contains(stderr.String(), "not a git repository") {
			return "", "", nil
		}
		return "", "", fail("repository_unavailable", "Git identity check failed")
	}
	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != 2 {
		return "", "", fail("repository_unavailable", "unexpected Git identity response")
	}
	worktree, err := canonicalDir(lines[0])
	if err != nil {
		return "", "", err
	}
	repo, err := canonicalDir(lines[1])
	return repo, worktree, err
}
