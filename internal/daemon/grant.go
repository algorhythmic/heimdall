package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"heimdall/internal/actions"
	"heimdall/internal/authz"
	"heimdall/internal/continuity"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"io"
	"net/http"
	"strconv"
	"time"
)

func (s *Server) grantHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" && r.URL.Path == "/grants" {
		st, err := s.Engine.Store.State(r.Context())
		if err != nil {
			writeError(w, 500, err)
			return
		}
		writeJSON(w, authz.List(st))
		return
	}
	if r.Method != "POST" || r.URL.Path != "/grants/command" {
		writeError(w, 404, fmt.Errorf("unknown grant route"))
		return
	}
	if r.Header.Get("Content-Type") != "application/json" {
		writeError(w, 415, fmt.Errorf("application/json required"))
		return
	}
	defer r.Body.Close()
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 8192))
	var input authz.Request
	if err == nil {
		err = model.StrictJSON(raw, &input)
	}
	var result json.RawMessage
	if err == nil {
		result, err = (authz.Service{Store: s.Engine.Store}).Execute(r.Context(), input, s.Clock().UTC())
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

// Only /client/* accepts client credentials. The store lock includes building the
// response; revocation cannot race authorization or a filesystem observation.
func (s *Server) clientHTTP(w http.ResponseWriter, r *http.Request, token string) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	if r.Method == "POST" && (r.URL.Path == "/client/intent" || r.URL.Path == "/client/report") {
		if r.Header.Get("Content-Type") != "application/json" {
			writeError(w, 415, fmt.Errorf("application/json required"))
			return
		}
		defer r.Body.Close()
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, actions.MaxRequest))
		var result json.RawMessage
		service := s.External
		if service == nil {
			service = &actions.ExternalService{Store: s.Engine.Store, Clock: s.Clock}
		}
		if err == nil {
			if r.URL.Path == "/client/intent" {
				var input actions.IntentRequest
				if err = model.StrictJSON(raw, &input); err == nil {
					result, err = service.Register(ctx, input, token)
				}
			} else {
				var input actions.ReportRequest
				if err = model.StrictJSON(raw, &input); err == nil {
					result, err = service.Report(ctx, input, token)
				}
			}
		}
		if err != nil {
			clientError(w, err)
			return
		}
		writeJSON(w, result)
		return
	}
	if r.Method == "POST" && r.URL.Path == "/client/checkpoint" {
		if r.Header.Get("Content-Type") != "application/json" {
			writeError(w, 415, fmt.Errorf("application/json required"))
			return
		}
		defer r.Body.Close()
		raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, continuity.MaxRequest))
		var input continuity.Request
		if err == nil {
			input, err = continuity.Decode(raw)
		}
		var result json.RawMessage
		if err == nil {
			result, err = (continuity.Service{Store: s.Engine.Store}).ExecuteClient(ctx, input, token, s.Clock)
		}
		if err != nil {
			clientError(w, err)
			return
		}
		writeJSON(w, result)
		return
	}
	var body []byte
	err := s.Engine.Store.Inspect(ctx, func(st model.State) error {
		g, err := authz.Authenticate(st, token, s.Clock().UTC())
		if err != nil {
			return err
		}
		if r.Method != "GET" {
			return authz.ErrDenied
		}
		target := r.URL.Query().Get("target")
		if !g.Contains(st, target) {
			return authz.ErrDenied
		}
		var result any
		switch r.URL.Path {
		case "/client/summary":
			var options continuity.SummaryOptions
			options, err = summaryOptions(r.URL.Query())
			if err == nil {
				result, err = continuity.ScopedProgressOverview(st, g, options)
			}
		case "/client/dependencies":
			if !model.ValidID(target) {
				return fmt.Errorf("dependencies require a task target")
			}
			result = continuity.DependencyViews(st, target, func(on string) bool { return g.Contains(st, on) })
		case "/client/task":
			result, _, err = model.ResolveTarget(st, target)
		case "/client/context":
			budget := 16000
			if raw := r.URL.Query().Get("budget"); raw != "" {
				budget, err = strconv.Atoi(raw)
			}
			if err == nil {
				var bundle continuity.Bundle
				bundle, err = continuity.ScopedContext(ctx, st, g, target, budget)
				if err == nil && s.Viewport != nil {
					err = bundle.ObserveOwnedWindows(ctx, s.Viewport.Observer, budget, st)
				}
				result = bundle
			}
		case "/client/history":
			limit := 20
			if raw := r.URL.Query().Get("limit"); raw != "" {
				limit, err = strconv.Atoi(raw)
			}
			if err == nil {
				result, err = continuity.History(st, g, target, r.URL.Query().Get("kind"), r.URL.Query().Get("cursor"), limit)
			}
		default:
			return authz.ErrDenied
		}
		if err != nil {
			return err
		}
		body, err = json.Marshal(result)
		if err != nil {
			return err
		}
		if len(body) > continuity.MaxReadBytes {
			return fmt.Errorf("response exceeds 512 KiB")
		}
		// A slow filesystem observation cannot use a grant past its expiry.
		if _, err = authz.Authenticate(st, token, s.Clock().UTC()); err != nil {
			return err
		}
		return ctx.Err()
	})
	if err != nil {
		clientError(w, err)
		return
	}
	w.Write(body)
}

func clientError(w http.ResponseWriter, err error) {
	var refusal *actions.Refusal
	if errors.As(err, &refusal) {
		w.WriteHeader(403)
		json.NewEncoder(w).Encode(map[string]any{"code": "access_denied", "error": refusal.Error(), "receipt": refusal.Receipt})
		return
	}

	status := 400
	if errors.Is(err, authz.ErrDenied) {
		status = 403
	}
	if errors.Is(err, store.ErrConflict) {
		status = 409
	}
	var budget *continuity.BudgetError
	if errors.As(err, &budget) {
		w.WriteHeader(422)
		json.NewEncoder(w).Encode(map[string]any{"code": "budget_too_small", "required_estimate": budget.Required, "budget": budget.Budget})
		return
	}
	code := "invalid_request"
	if status == 403 {
		code = "access_denied"
	}
	if status == 409 {
		code = "conflict"
	}
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{"code": code, "error": err.Error()})
}
