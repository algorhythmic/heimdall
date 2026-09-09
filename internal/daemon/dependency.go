package daemon

import (
	"errors"
	"fmt"
	"heimdall/internal/continuity"
	"heimdall/internal/store"
	"io"
	"net/http"
	"net/url"
	"strconv"
)

func summaryOptions(q url.Values) (continuity.SummaryOptions, error) {
	r := continuity.SummaryOptions{Target: q.Get("target"), Sort: q.Get("sort"), Limit: 25, Cursor: q.Get("cursor")}
	if r.Sort == "" {
		r.Sort = "checkpoint"
	}
	var err error
	if q.Has("limit") {
		r.Limit, err = strconv.Atoi(q.Get("limit"))
		if err != nil {
			return r, err
		}
	}
	if q.Has("subtree") {
		r.Subtree, err = strconv.ParseBool(q.Get("subtree"))
		if err != nil {
			return r, err
		}
	}
	return r, r.Validate()
}
func (s *Server) dependencyHTTP(w http.ResponseWriter, r *http.Request) {
	service := continuity.Service{Store: s.Engine.Store}
	q := r.URL.Query()
	var result any
	var err error
	switch {
	case r.Method == "GET" && r.URL.Path == "/dependency/list":
		result, err = service.DependencyList(r.Context(), q.Get("target"))
	case r.Method == "GET" && r.URL.Path == "/dependency/show":
		result, err = service.DependencyShow(r.Context(), q.Get("target"), q.Get("id"))
	case r.Method == "POST" && r.URL.Path == "/dependency/command":
		if r.Header.Get("Content-Type") != "application/json" {
			writeError(w, 415, fmt.Errorf("application/json required"))
			return
		}
		defer r.Body.Close()
		var raw []byte
		raw, err = io.ReadAll(http.MaxBytesReader(w, r.Body, continuity.MaxRequest))
		if err == nil {
			var input continuity.DependencyRequest
			input, err = continuity.DecodeDependency(raw)
			if err == nil {
				result, err = service.Dependency(r.Context(), input, "cli", s.Clock().UTC())
			}
		}
	default:
		writeError(w, 404, fmt.Errorf("unknown dependency route or method"))
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
