package daemon

import (
	"errors"
	"fmt"
	"heimdall/internal/continuity"
	"heimdall/internal/store"
	"io"
	"net/http"
	"strconv"
)

func (s *Server) progressHTTP(w http.ResponseWriter, r *http.Request) {
	service := continuity.Service{Store: s.Engine.Store}
	q := r.URL.Query()
	var result any
	var err error
	switch {
	case r.Method == "GET" && r.URL.Path == "/progress/summary":
		var options continuity.SummaryOptions
		options, err = summaryOptions(q)
		if err == nil {
			result, err = service.ProgressOverview(r.Context(), options)
		}
	case r.Method == "GET" && r.URL.Path == "/progress/list":
		limit := 25
		if q.Has("limit") {
			limit, err = strconv.Atoi(q.Get("limit"))
		}
		if err == nil {
			result, err = service.ProgressList(r.Context(), q.Get("target"), q.Get("after"), limit)
		}
	case r.Method == "GET" && r.URL.Path == "/progress/show":
		result, err = service.ProgressShow(r.Context(), q.Get("target"), q.Get("id"))
	case r.Method == "POST" && r.URL.Path == "/progress/command":
		if r.Header.Get("Content-Type") != "application/json" {
			writeError(w, 415, fmt.Errorf("application/json required"))
			return
		}
		defer r.Body.Close()
		var body []byte
		body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, continuity.MaxRequest))
		if err == nil {
			var input continuity.ProgressRequest
			input, err = continuity.DecodeProgress(body)
			if err == nil {
				result, err = service.Progress(r.Context(), input, "cli", s.Clock().UTC())
			}
		}
	default:
		writeError(w, 404, fmt.Errorf("unknown progress route or method"))
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
