//go:build linux

package herdr

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type fakeServer struct {
	t               *testing.T
	path            string
	listener        net.Listener
	mu              sync.Mutex
	pane            Pane
	version         string
	protocol        int
	gets            int
	writes          int
	changePane      bool
	wrongReadback   bool
	huge            bool
	workspaceTokens map[string]string
}

func fake(t *testing.T, path, cwd string) *fakeServer {
	t.Helper()
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	s := &fakeServer{t: t, path: path, listener: ln, version: "0.8.2", protocol: 20, pane: Pane{ID: "w1:p1", TerminalID: "terminal-1", WorkspaceID: "w1", TabID: "w1:t1", ForegroundCwd: cwd}}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(conn)
		}
	}()
	return s
}

func (s *fakeServer) serve(conn net.Conn) {
	defer conn.Close()
	var r struct {
		ID, Method string
		Params     map[string]json.RawMessage
	}
	if err := json.NewDecoder(conn).Decode(&r); err != nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var result any
	switch r.Method {
	case "ping":
		if s.huge {
			conn.Write([]byte(strings.Repeat("x", MaxResponse+1)))
			return
		}
		result = map[string]any{"type": "pong", "version": s.version, "protocol": s.protocol}
	case "pane.get":
		var pane string
		json.Unmarshal(r.Params["pane_id"], &pane)
		if pane != s.pane.ID {
			json.NewEncoder(conn).Encode(map[string]any{"id": r.ID, "error": map[string]string{"code": "pane_not_found", "message": "missing"}})
			return
		}
		s.gets++
		p := s.pane
		if s.changePane && s.gets > 1 {
			p.TabID = "w1:t2"
		}
		result = map[string]any{"type": "pane_info", "pane": p}
	case "pane.process_info":
		result = map[string]any{"type": "pane_process_info", "process_info": map[string]any{"pane_id": s.pane.ID, "shell_pid": 1234}}
	case "pane.report_metadata":
		s.writes++
		if !s.wrongReadback {
			json.Unmarshal(r.Params["title"], &s.pane.Title)
			json.Unmarshal(r.Params["tokens"], &s.pane.Tokens)
		}
		result = map[string]string{"type": "ok"}
	case "workspace.report_metadata":
		json.Unmarshal(r.Params["tokens"], &s.workspaceTokens)
		result = map[string]string{"type": "ok"}
	case "workspace.get":
		result = map[string]any{"type": "workspace_info", "workspace": map[string]any{"workspace_id": s.pane.WorkspaceID, "tokens": s.workspaceTokens}}
	default:
		s.t.Errorf("unexpected RPC %s", r.Method)
		return
	}
	json.NewEncoder(conn).Encode(map[string]any{"id": r.ID, "result": result})
}

func setupAdapter(t *testing.T) (Adapter, *fakeServer, string) {
	t.Helper()
	// /tmp keeps Unix socket names below sun_path even in long checkout paths.
	dir, err := os.MkdirTemp("", "herdr-unit-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	// Own the repository boundary so unrelated ancestor .git entries cannot
	// change protocol tests into repository-discovery failures.
	cmd := exec.Command("git", "init", dir)
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_SYSTEM=" + os.DevNull}
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatal(string(out), err)
	}
	s := fake(t, filepath.Join(dir, "api.sock"), dir)
	a := Adapter{process: func(int) (string, string, error) { return "123", dir, nil }}
	return a, s, dir
}

func TestObserveAndExpiringMetadataReadback(t *testing.T) {
	a, s, dir := setupAdapter(t)
	alias := filepath.Join(dir, "alias")
	if err := os.Symlink(dir, alias); err != nil {
		t.Fatal(err)
	}
	s.pane.ForegroundCwd = alias
	o, err := a.Observe(context.Background(), s.path, s.pane.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	if o.Locator.Cwd != dir || o.Locator.Repository != filepath.Join(dir, ".git") || o.Locator.Worktree != dir || o.Locator.Host == "" || o.Locator.SourceEpoch == "" {
		t.Fatal(o)
	}
	err = a.Report(context.Background(), o, "heimdall-test", 1, "Task | Next", map[string]string{"heimdall_task": "alpha", "heimdall_title": "Alpha", "heimdall_next": "Review", "heimdall_checked_at": "2026-09-09T00:00:00Z"}, 30000, true)
	if err != nil || s.writes != 1 {
		t.Fatal(err, s.writes)
	}
	if s.workspaceTokens["heimdall_summary"] != "w1:p1 | Alpha" {
		t.Fatal("workspace summary omitted pane identity", s.workspaceTokens)
	}
	s.mu.Lock()
	s.wrongReadback = true
	s.mu.Unlock()
	if err := a.Report(context.Background(), o, "heimdall-test", 2, "Changed title", map[string]string{"heimdall_task": "beta"}, 30000, false); err == nil {
		t.Fatal("unchecked metadata acknowledgement")
	}
}

func TestObserveRefusesUnstableOrUnsupportedIdentities(t *testing.T) {
	for _, kind := range []string{"protocol", "version", "cwd", "pane-moved", "source", "oversize", "writable-socket", "process-replaced"} {
		t.Run(kind, func(t *testing.T) {
			a, s, _ := setupAdapter(t)
			expected := ""
			switch kind {
			case "protocol":
				s.protocol = 999
			case "version":
				s.version = "0.8.3"
			case "cwd":
				s.pane.ForegroundCwd = "/missing-synthetic-path"
			case "pane-moved":
				s.changePane = true
			case "source":
				expected = strings.Repeat("0", 64)
			case "oversize":
				s.huge = true
			case "writable-socket":
				os.Chmod(s.path, 0666)
			case "process-replaced":
				calls := 0
				a.process = func(int) (string, string, error) {
					calls++
					return string(rune('0' + calls)), filepath.Dir(s.path), nil
				}
			}
			if _, err := a.Observe(context.Background(), s.path, s.pane.ID, expected); err == nil {
				t.Fatal("unsafe observation accepted")
			}
			if s.writes != 0 {
				t.Fatal("observation wrote metadata")
			}
		})
	}
}

func TestReportRefusesSocketReplacementBeforeWrite(t *testing.T) {
	a, s, dir := setupAdapter(t)
	o, err := a.Observe(context.Background(), s.path, s.pane.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	s.listener.Close()
	next := fake(t, s.path, dir)
	err = a.Report(context.Background(), o, "heimdall-test", 1, "Do not publish", map[string]string{}, 30000, false)
	var ae *Error
	if !errors.As(err, &ae) || ae.Code != "source_changed" || next.writes != 0 {
		t.Fatal("new server received stale metadata", err, next.writes)
	}
}

func TestCanonicalGitAndLinkedWorktreeIdentity(t *testing.T) {
	dir := t.TempDir()
	repo := filepath.Join(dir, "repo")
	work := filepath.Join(dir, "linked")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_CONFIG_SYSTEM="+os.DevNull)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatal(string(out), err)
		}
	}
	git("init", repo)
	git("-C", repo, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-m", "Fixture")
	git("-C", repo, "worktree", "add", "-b", "fixture", work)
	common, root, err := gitIdentity(context.Background(), work)
	if err != nil || common != filepath.Join(repo, ".git") || root != work {
		t.Fatal(common, root, err)
	}
	if err := os.WriteFile(filepath.Join(work, ".git"), []byte("not valid"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := gitIdentity(context.Background(), work); err == nil {
		t.Fatal("malformed Git identity treated as non-Git")
	}
}
