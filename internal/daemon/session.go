package daemon

import (
	"context"
	"fmt"
	"heimdall/internal/conversation"
	"heimdall/internal/model"
	"heimdall/internal/session"
	"heimdall/internal/store"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/algorhythmic/skald/sessionrecord"
)

// serviceSessions returns the running ingest service or a store-backed handle
// for hook delivery in tests.
func serviceSessions(s *Server) *session.Service {
	if s.Sessions != nil {
		return s.Sessions
	}
	return &session.Service{Store: s.Engine.Store}
}

type sourceRootRequest struct {
	Version  int    `json:"version"`
	ID       string `json:"id"`
	Provider string `json:"provider"`
	Root     string `json:"root"`
}

func (s *Server) sessionHTTP(w http.ResponseWriter, r *http.Request) {
	var result any
	var err error
	switch {
	case r.Method == "GET" && r.URL.Path == "/session/sources":
		var st model.State
		st, err = s.Engine.Store.State(r.Context())
		if err == nil {
			roots, streams := session.SourcesView(st)
			result = map[string]any{"roots": roots, "streams": streams}
		}
	case r.Method == "POST" && r.URL.Path == "/session/hook":
		if r.Header.Get("Content-Type") != "application/json" {
			writeError(w, 415, fmt.Errorf("application/json required"))
			return
		}
		defer r.Body.Close()
		var body []byte
		body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
		var p session.HookPayload
		if err == nil {
			err = model.StrictJSON(body, &p)
		}
		if err == nil {
			err = serviceSessions(s).HandleHook(r.Context(), p, s.Clock().UTC())
		}
		result = map[string]bool{"ok": err == nil}
	case r.Method == "POST" && r.URL.Path == "/session/source":
		if r.Header.Get("Content-Type") != "application/json" {
			writeError(w, 415, fmt.Errorf("application/json required"))
			return
		}
		defer r.Body.Close()
		var body []byte
		body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
		var req sourceRootRequest
		if err == nil {
			err = model.StrictJSON(body, &req)
		}
		if err == nil {
			result, err = s.configureSource(r.Context(), req)
		}
	default:
		writeError(w, 404, fmt.Errorf("unknown session route or method"))
		return
	}
	if err != nil {
		status := 400
		if err == store.ErrConflict {
			status = 409
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, result)
}

// configureSource canonicalizes the root and derives the contracted namespace
// before registering. The namespace is authoritative once stored.
func (s *Server) configureSource(ctx context.Context, req sourceRootRequest) (any, error) {
	if req.Version != 1 || req.Provider != "claude_code" && req.Provider != "codex" {
		return nil, fmt.Errorf("invalid source request")
	}
	abs, err := filepath.Abs(req.Root)
	if err != nil {
		return nil, err
	}
	root := filepath.Clean(abs)
	if resolved, err := filepath.EvalSymlinks(root); err == nil {
		root = resolved
	}
	info, err := os.Stat(root)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("source root is not a directory")
	}
	host, err := os.Hostname()
	if err != nil {
		return nil, err
	}
	namespace, err := sessionrecord.Namespace(req.Provider, host, root, "", "")
	if err != nil {
		return nil, err
	}
	id := req.ID
	if id == "" {
		id = model.NewID()
	}
	reg := conversation.SourceRoot{Version: 1, ID: id, Provider: req.Provider, Root: root, Namespace: namespace, Host: host, RegisteredAt: s.Clock().UTC()}
	return s.Engine.Store.RegisterSourceRoot(ctx, reg, s.Clock().UTC())
}
