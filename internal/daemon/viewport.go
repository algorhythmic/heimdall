package daemon

import (
	"fmt"
	"heimdall/internal/adapters/hyprland"
	"heimdall/internal/workspace"
	"io"
	"net/http"
)

func (s *Server) viewportHTTP(w http.ResponseWriter, r *http.Request) (any, error) {
	if s.Viewport == nil {
		return nil, fmt.Errorf("viewport observer unavailable")
	}
	q := r.URL.Query()
	if r.Method == "GET" {
		switch r.URL.Path {
		case "/workspace/viewport/probe":
			return hyprland.Probe(r.Context(), q.Get("socket_dir"), s.Viewport.Observer.Connector)
		case "/workspace/viewport/status":
			st, err := s.Engine.Store.State(r.Context())
			if err != nil {
				return nil, err
			}
			status, _ := s.Viewport.Observer.Read(r.Context(), false)
			status.Snapshot = nil
			return map[string]any{"source_head": st.DesktopSourceHead, "source": st.DesktopSources[st.DesktopSourceHead], "observation": status}, nil
		case "/workspace/viewport/inventory":
			v, _ := s.Viewport.Observer.Read(r.Context(), true)
			return v, nil
		case "/workspace/viewport/list":
			return s.Viewport.View(r.Context(), q.Get("target"), q.Get("cached") != "true")
		}
	}
	if r.Method != "POST" || r.URL.Path != "/workspace/viewport/command" {
		return nil, fmt.Errorf("unknown viewport route or method")
	}
	if r.Header.Get("Content-Type") != "application/json" {
		return nil, fmt.Errorf("application/json required")
	}
	defer r.Body.Close()
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, workspace.MaxRequest))
	if err != nil {
		return nil, err
	}
	input, err := workspace.DecodeViewport(raw)
	if err != nil {
		return nil, err
	}
	return s.Viewport.Execute(r.Context(), input, "cli", s.Clock().UTC())
}
