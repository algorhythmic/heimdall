package daemon

import (
	"encoding/json"
	"errors"
	"fmt"
	"heimdall/internal/continuity"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"io"
	"net/http"
	"strconv"
)

func (s *Server) continuityHTTP(w http.ResponseWriter, r *http.Request) {
	service := continuity.Service{Store: s.Engine.Store}
	var result any
	var err error
	switch {
	case r.Method == "GET" && r.URL.Path == "/continuity/context":
		budget := 16000
		if value := r.URL.Query().Get("budget"); value != "" {
			budget, err = strconv.Atoi(value)
		}
		if err == nil {
			result, err = service.Context(r.Context(), r.URL.Query().Get("target"), budget)
		}
	case r.Method == "GET" && r.URL.Path == "/continuity/state":
		result, err = service.View(r.Context(), r.URL.Query().Get("target"))
	case r.Method == "POST" && (r.URL.Path == "/continuity/command" || r.URL.Path == "/continuity/backup"):
		if r.Header.Get("Content-Type") != "application/json" {
			writeError(w, 415, fmt.Errorf("application/json required"))
			return
		}
		defer r.Body.Close()
		body, readErr := io.ReadAll(http.MaxBytesReader(w, r.Body, continuity.MaxRequest))
		if readErr != nil {
			writeError(w, 400, readErr)
			return
		}
		if r.URL.Path == "/continuity/backup" {
			var input struct {
				Path string `json:"path"`
			}
			err = model.StrictJSON(body, &input)
			if err == nil && input.Path == "" {
				err = fmt.Errorf("backup path required")
			}
			if err == nil {
				err = s.Engine.Store.Backup(r.Context(), input.Path)
				result = map[string]any{"path": input.Path, "database_only": true, "status": "backed_up"}
			}
		} else {
			var input continuity.Request
			input, err = continuity.Decode(body)
			if err == nil {
				result, err = service.Execute(r.Context(), input, "cli", s.Clock().UTC())
			}
		}
	default:
		writeError(w, 404, fmt.Errorf("unknown continuity route or method"))
		return
	}
	if err != nil {
		var budget *continuity.BudgetError
		if errors.As(err, &budget) {
			w.WriteHeader(422)
			json.NewEncoder(w).Encode(map[string]any{"error": err.Error(), "code": "budget_too_small", "required_estimate": budget.Required, "budget": budget.Budget})
			return
		}
		status := 400
		if errors.Is(err, store.ErrConflict) {
			status = 409
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, result)
}
