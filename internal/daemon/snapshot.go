package daemon

import (
	"fmt"
	"heimdall/internal/workspace"
	"io"
	"net/http"
	"strconv"
)

func (s *Server) snapshotHTTP(w http.ResponseWriter, r *http.Request) (any, error) {
	if s.Snapshots == nil {
		return nil, fmt.Errorf("snapshot service unavailable")
	}
	q := r.URL.Query()
	if r.Method == "GET" {
		switch r.URL.Path {
		case "/workspace/snapshot/status":
			return s.Snapshots.Status(r.Context(), q.Get("target"), s.Clock().UTC())
		case "/workspace/snapshot/show":
			return s.Engine.Store.WorkspacePoint(r.Context(), q.Get("target"), q.Get("id"))
		case "/workspace/snapshot/list":
			before := int64(0)
			limit := 25
			var err error
			if q.Get("before") != "" {
				before, err = strconv.ParseInt(q.Get("before"), 10, 64)
				if err != nil {
					return nil, fmt.Errorf("invalid snapshot cursor")
				}
			}
			if q.Get("limit") != "" {
				limit, err = strconv.Atoi(q.Get("limit"))
				if err != nil {
					return nil, fmt.Errorf("invalid snapshot limit")
				}
			}
			return s.Snapshots.List(r.Context(), q.Get("target"), before, limit)
		}
	}
	if r.Method != "POST" || r.URL.Path != "/workspace/snapshot/command" {
		return nil, fmt.Errorf("unknown snapshot route or method")
	}
	if r.Header.Get("Content-Type") != "application/json" {
		return nil, fmt.Errorf("application/json required")
	}
	defer r.Body.Close()
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, workspace.MaxRequest))
	if err != nil {
		return nil, err
	}
	input, err := workspace.DecodeSnapshot(raw)
	if err != nil {
		return nil, err
	}
	return s.Snapshots.Execute(r.Context(), input, "cli", s.Clock().UTC())
}
