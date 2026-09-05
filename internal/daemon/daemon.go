// Package daemon provides the single-writer command service for local CLI clients.
package daemon

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"heimdall/internal/checks"
	"heimdall/internal/core"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type Endpoint struct {
	URL   string `json:"url"`
	Token string `json:"token"`
	PID   int    `json:"pid"`
}
type Request struct {
	Command core.Command `json:"command"`
	Now     string       `json:"now,omitempty"`
}
type Server struct {
	uiMu              sync.Mutex
	uiCodes           map[string]uiBootstrap
	uiSessions        map[string]uiSession
	EvaluationContext context.Context
	evaluations       sync.WaitGroup
	Engine            *core.Engine
	Token, Host       string
	BrowserToken      string
	Clock             func() time.Time
}

func Serve(ctx context.Context, dir string, clock func() time.Time, ready func(Endpoint)) error {
	e, err := core.Open(dir)
	if err != nil {
		return err
	}
	defer e.Close()
	e.ValidateEvidence = checks.ValidateTarget
	if clock == nil {
		clock = time.Now
	}
	if err := (checks.Service{Store: e.Store}).Recover(ctx, clock().UTC()); err != nil {
		return err
	}
	_ = e.ReconcileFile(ctx, clock())
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		return err
	}
	defer listener.Close()
	var secret [32]byte
	if _, err = rand.Read(secret[:]); err != nil {
		return err
	}
	ep := Endpoint{URL: "http://" + listener.Addr().String(), Token: hex.EncodeToString(secret[:]), PID: os.Getpid()}
	path := filepath.Join(e.Dir, "endpoint.json")
	b, _ := json.Marshal(ep)
	// The OS writer lock protects stale rendezvous replacement after a crash.
	if err = os.WriteFile(path, b, 0600); err != nil {
		return err
	}
	defer os.Remove(path)
	// Public rendezvous lets scoped clients rediscover the port without reading
	// either unrestricted credential file.
	clientPath := filepath.Join(e.Dir, "client-endpoint.json")
	publicEndpoint, _ := json.Marshal(Endpoint{URL: ep.URL, PID: ep.PID})
	if err = os.WriteFile(clientPath, publicEndpoint, 0600); err != nil {
		return err
	}
	defer os.Remove(clientPath)
	if _, err = rand.Read(secret[:]); err != nil {
		return err
	}
	browserToken := hex.EncodeToString(secret[:])
	browserPath := filepath.Join(e.Dir, "browser-endpoint.json")
	browserEndpoint := Endpoint{URL: ep.URL, Token: browserToken, PID: ep.PID}
	bb, _ := json.Marshal(browserEndpoint)
	if err = os.WriteFile(browserPath, bb, 0600); err != nil {
		return err
	}
	defer os.Remove(browserPath)
	service := &Server{Engine: e, Token: ep.Token, BrowserToken: browserToken, Host: listener.Addr().String(), Clock: clock}
	server := &http.Server{Handler: service, ReadHeaderTimeout: 2 * time.Second, ReadTimeout: 5 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second, MaxHeaderBytes: 8192}
	localCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	service.EvaluationContext = localCtx
	watchDone := make(chan struct{})
	go func() { defer close(watchDone); watch(localCtx, e, clock) }()
	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		<-localCtx.Done()
		shutdownCtx, stop := context.WithTimeout(context.Background(), 2*time.Second)
		defer stop()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
		}
	}()
	if ready != nil {
		ready(ep)
	}
	err = server.Serve(listener)
	cancel()
	<-watchDone
	<-stopped
	service.evaluations.Wait()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
func watch(ctx context.Context, e *core.Engine, clock func() time.Time) {
	tick := time.NewTicker(500 * time.Millisecond)
	defer tick.Stop()
	previous := ""
	nextTimer := time.Time{}
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			b, err := os.ReadFile(filepath.Join(e.Dir, "tasks.yaml"))
			if err == nil {
				current := string(b)
				if current == previous {
					_ = e.ReconcileFile(ctx, clock())
				}
				previous = current
			}
			now := clock()
			if !now.Before(nextTimer) {
				_, _ = e.Execute(ctx, core.Command{ID: model.NewID(), Op: "tick"}, "scheduler", now)
				nextTimer = now.Add(time.Second)
			}
		}
	}
}
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if isUIPath(r.URL.Path) {
		s.uiHTTP(w, r)
		return
	}
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if r.Host != s.Host || r.Header.Get("Origin") != "" || !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
		writeError(w, 401, fmt.Errorf("unauthorized local client"))
		return
	}
	if strings.HasPrefix(r.URL.Path, "/client/") {
		s.clientHTTP(w, r, token)
		return
	}
	expected := s.Token
	if r.URL.Path == "/browser/message" {
		expected = s.BrowserToken
	}
	if expected == "" || r.Host != s.Host || r.Header.Get("Origin") != "" || subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
		writeError(w, http.StatusUnauthorized, fmt.Errorf("unauthorized local client"))
		return
	}
	if strings.HasPrefix(r.URL.Path, "/browser/") {
		s.browserHTTP(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/continuity/") {
		s.continuityHTTP(w, r)
		return
	}
	if strings.HasPrefix(r.URL.Path, "/evidence/") {
		s.evidenceHTTP(w, r)
		return
	}
	if r.URL.Path == "/ui-bootstrap" {
		s.uiBootstrapHTTP(w, r)
		return
	}
	if r.URL.Path == "/grants" || strings.HasPrefix(r.URL.Path, "/grants/") {
		s.grantHTTP(w, r)
		return
	}
	if r.Method == http.MethodGet {
		switch r.URL.Path {
		case "/health":
			writeJSON(w, map[string]any{"status": "running", "task_file_error": s.Engine.ViewError(), "capabilities": []string{"core", "capture", "manual_completion", "aggregate_proposals", "review_timers", "replay", "browser_metadata", "browser_commands", "continuity_cli_v1", "database_backup", "scoped_client_reads_v1", "scoped_checkpoint_writes_v1", "mcp_stdio_v1", "evidence_cli_v1", "evidence_revalidation_v1"}})
		case "/state":
			st, err := s.Engine.Store.State(r.Context())
			if err != nil {
				writeError(w, 500, err)
				return
			}
			writeJSON(w, st)
		case "/events":
			events, err := s.Engine.Store.Events(r.Context())
			if err != nil {
				writeError(w, 500, err)
				return
			}
			writeJSON(w, events)
		default:
			writeError(w, 404, fmt.Errorf("unknown route"))
		}
		return
	}
	if r.Method != http.MethodPost {
		writeError(w, 405, fmt.Errorf("method not allowed"))
		return
	}
	if r.Header.Get("Content-Type") != "application/json" {
		writeError(w, 415, fmt.Errorf("expected application/json"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	defer r.Body.Close()
	var req Request
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&req); err != nil {
		writeError(w, 400, err)
		return
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		writeError(w, 400, fmt.Errorf("expected one request"))
		return
	}
	now := s.Clock().UTC()
	if req.Now != "" {
		var err error
		now, err = time.Parse(time.RFC3339Nano, req.Now)
		if err != nil {
			writeError(w, 400, err)
			return
		}
	}
	var result any
	var err error
	switch r.URL.Path {
	case "/commands":
		result, err = s.Engine.Execute(r.Context(), req.Command, "cli", now)
	case "/replay":
		result, err = s.Engine.Replay(r.Context())
	case "/sync":
		err = s.Engine.ReconcileFile(r.Context(), now)
		result = map[string]bool{"synced": err == nil}
	case "/fmt":
		err = s.Engine.Format(r.Context())
		result = map[string]bool{"formatted": err == nil}
	default:
		writeError(w, 404, fmt.Errorf("unknown route"))
		return
	}
	if err != nil {
		status := 400
		if errors.Is(err, store.ErrConflict) {
			status = 409
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, result)
}
func writeJSON(w http.ResponseWriter, v any) { _ = json.NewEncoder(w).Encode(v) }
func writeError(w http.ResponseWriter, status int, err error) {
	w.WriteHeader(status)
	writeJSON(w, map[string]string{"error": err.Error()})
}
