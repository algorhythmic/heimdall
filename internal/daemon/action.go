package daemon

import (
	"errors"
	"fmt"
	"heimdall/internal/actions"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"io"
	"net/http"
	"strconv"
)

func (s *Server) actionHTTP(w http.ResponseWriter, r *http.Request) {
	service := actions.Service{Store: s.Engine.Store}
	q := r.URL.Query()
	var result any
	var err error
	if r.Method == "GET" {
		switch r.URL.Path {
		case "/action/context":
			result, err = service.Context(r.Context(), q.Get("target"))
		case "/action/show":
			result, err = service.Show(r.Context(), q.Get("target"), q.Get("id"))
		case "/action/list", "/action/history":
			before, limit := int64(0), 25
			if q.Get("before") != "" {
				before, err = strconv.ParseInt(q.Get("before"), 10, 64)
			}
			if err == nil && q.Get("limit") != "" {
				limit, err = strconv.Atoi(q.Get("limit"))
			}
			if err == nil {
				if r.URL.Path == "/action/list" {
					result, err = service.List(r.Context(), q.Get("target"), before, limit)
				} else {
					result, err = s.Engine.Store.ActionHistory(r.Context(), q.Get("target"), q.Get("id"), before, limit)
				}
			}
		default:
			err = fmt.Errorf("unknown action route")
		}
	} else if r.Method == "POST" && r.Header.Get("Content-Type") == "application/json" {
		defer r.Body.Close()
		var raw []byte
		raw, err = io.ReadAll(http.MaxBytesReader(w, r.Body, actions.MaxRequest))
		if err == nil {
			switch r.URL.Path {
			case "/action/queue":
				var input actions.Request
				input, err = actions.Decode(raw)
				if err == nil {
					result, err = service.Queue(r.Context(), input, "cli", s.Clock().UTC())
				}
			case "/action/cancel", "/action/reconcile":
				var input actions.CancelRequest
				err = model.StrictJSON(raw, &input)
				if err == nil {
					if r.URL.Path == "/action/reconcile" {
						result, err = service.Reconcile(r.Context(), input, "cli", s.Clock().UTC())
					} else {
						result, err = service.Cancel(r.Context(), input, "cli", s.Clock().UTC())
					}
				}
			default:
				err = fmt.Errorf("unknown action command")
			}
		}
	} else {
		err = fmt.Errorf("action route requires GET or JSON POST")
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
