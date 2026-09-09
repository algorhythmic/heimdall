package daemon

import (
	"fmt"
	"heimdall/internal/model"
	"heimdall/internal/workspace"
	"io"
	"net/http"
)

func (s *Server) operationHTTP(w http.ResponseWriter, r *http.Request) (any, error) {
	service := s.Operations
	if service == nil {
		service = &workspace.OperationService{Store: s.Engine.Store}
	}
	q := r.URL.Query()
	if r.Method == "GET" {
		switch r.URL.Path {
		case "/workspace/operation/show":
			return service.Show(r.Context(), q.Get("target"), q.Get("id"))
		case "/workspace/operation/list":
			return service.List(r.Context(), q.Get("target"))
		case "/workspace/operation/residents":
			st, err := s.Engine.Store.State(r.Context())
			return st.WorkspaceSlots, err
		}
	}
	if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
		return nil, fmt.Errorf("JSON POST required")
	}
	defer r.Body.Close()
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, workspace.OperationMaxBytes))
	if err != nil {
		return nil, err
	}
	switch r.URL.Path {
	case "/workspace/operation/queue":
		var input workspace.OperationRequest
		if err := model.StrictJSON(raw, &input); err != nil {
			return nil, err
		}
		return service.Queue(r.Context(), input, "cli")
	case "/workspace/operation/cancel", "/workspace/operation/reconcile":
		var input workspace.OperationControl
		if err := model.StrictJSON(raw, &input); err != nil {
			return nil, err
		}
		kind := "cancel"
		if r.URL.Path == "/workspace/operation/reconcile" {
			kind = "reconcile"
		}
		return service.Control(r.Context(), input, kind, "cli")
	}
	return nil, fmt.Errorf("unknown workspace operation route")
}
