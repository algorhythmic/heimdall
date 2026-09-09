package daemon

import (
	"errors"
	"fmt"
	"heimdall/internal/store"
	"heimdall/internal/workspace"
	"io"
	"net/http"
	"strings"
)

func (s *Server) workspaceHTTP(w http.ResponseWriter, r *http.Request) {
	service := workspace.Service{Store: s.Engine.Store}
	var result any
	var err error
	q := r.URL.Query()
	switch {
	case strings.HasPrefix(r.URL.Path, "/workspace/viewport/"):
		result, err = s.viewportHTTP(w, r)
	case strings.HasPrefix(r.URL.Path, "/workspace/herdr/"):
		result, err = s.herdrHTTP(w, r)
	case r.Method == "GET" && r.URL.Path == "/workspace/state":
		result, err = service.View(r.Context(), q.Get("target"))
	case r.Method == "GET" && r.URL.Path == "/workspace/manifest":
		result, err = service.Manifest(r.Context(), q.Get("target"), q.Get("id"))
	case r.Method == "GET" && r.URL.Path == "/workspace/session":
		result, err = service.Binding(r.Context(), q.Get("target"), q.Get("surface"), q.Get("id"))
	case r.Method == "POST" && r.URL.Path == "/workspace/command":
		if r.Header.Get("Content-Type") != "application/json" {
			writeError(w, 415, fmt.Errorf("application/json required"))
			return
		}
		defer r.Body.Close()
		var body []byte
		body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, workspace.MaxRequest))
		if err == nil {
			var input workspace.Request
			input, err = workspace.Decode(body)
			if err == nil {
				result, err = service.Execute(r.Context(), input, "cli", s.Clock().UTC())
			}
		}
	default:
		writeError(w, 404, fmt.Errorf("unknown workspace route or method"))
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
