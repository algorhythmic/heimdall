package daemon

import (
	"context"
	"errors"
	"fmt"
	"heimdall/internal/checks"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"io"
	"net/http"
	"time"
)

func (s *Server) evidenceHTTP(w http.ResponseWriter, r *http.Request) {
	service := checks.Service{Store: s.Engine.Store}
	var result any
	var err error
	if r.Method == "GET" && r.URL.Path == "/evidence/state" {
		result, err = service.View(r.Context(), r.URL.Query().Get("target"))
	} else if r.Method == "POST" {
		if r.Header.Get("Content-Type") != "application/json" {
			writeError(w, 415, fmt.Errorf("application/json required"))
			return
		}
		defer r.Body.Close()
		body, readErr := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
		if readErr != nil {
			writeError(w, 400, readErr)
			return
		}
		var input checks.Request
		err = model.StrictJSON(body, &input)
		if err == nil {
			switch r.URL.Path {
			case "/evidence/configure":
				result, err = service.Accept(r.Context(), input, s.Clock().UTC())
			case "/evidence/evaluate":
				var launch bool
				var evidence model.Evidence
				evidence, launch, err = service.Start(r.Context(), input, s.Clock().UTC())
				result = evidence
				if err == nil && launch {
					parent := s.EvaluationContext
					if parent == nil {
						parent = context.Background()
					}
					s.evaluations.Add(1)
					go func() { defer s.evaluations.Done(); _ = service.Execute(parent, evidence.ID) }()
				}
			case "/evidence/refresh":
				ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
				defer cancel()
				err = service.Refresh(ctx, input.Target, s.Clock().UTC())
				if err == nil {
					result, err = service.View(ctx, input.Target)
				}
			default:
				err = fmt.Errorf("unknown evidence route")
			}
		}
	} else {
		writeError(w, 404, fmt.Errorf("unknown evidence route or method"))
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
