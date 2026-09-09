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

func (s *Server) preservationHTTP(w http.ResponseWriter, r *http.Request) {
	service := continuity.Service{Store: s.Engine.Store}
	q := r.URL.Query()
	var result any
	var err error
	switch {
	case r.Method == "GET" && r.URL.Path == "/preservation/show":
		result, err = service.PreservationShow(r.Context(), q.Get("target"), q.Get("id"))
	case r.Method == "GET" && r.URL.Path == "/preservation/list":
		limit := 25
		if q.Has("limit") {
			limit, err = strconv.Atoi(q.Get("limit"))
		}
		if err == nil {
			result, err = service.PreservationList(r.Context(), q.Get("target"), q.Get("after"), limit)
		}
	case r.Method == "GET" && r.URL.Path == "/preservation/export":
		result, err = service.PreservationExport(r.Context(), q.Get("target"), q.Get("checkpoint"))
	case r.Method == "POST" && (r.URL.Path == "/preservation/preview" || r.URL.Path == "/preservation/command"):
		if r.Header.Get("Content-Type") != "application/json" {
			writeError(w, 415, fmt.Errorf("application/json required"))
			return
		}
		defer r.Body.Close()
		var raw []byte
		raw, err = io.ReadAll(http.MaxBytesReader(w, r.Body, continuity.MaxRequest))
		if err == nil {
			var input continuity.PreservationRequest
			input, err = continuity.DecodePreservation(raw)
			if err == nil {
				if r.URL.Path == "/preservation/preview" {
					result, err = service.PreservationPreview(r.Context(), input)
				} else {
					result, err = service.Preservation(r.Context(), input, "cli", s.Clock().UTC())
				}
			}
		}
	default:
		writeError(w, 404, fmt.Errorf("unknown preservation route or method"))
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
