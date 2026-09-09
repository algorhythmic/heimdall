package daemon

import (
	"errors"
	"fmt"
	"heimdall/internal/continuity"
	"heimdall/internal/store"
	"io"
	"net/http"
)

func (s *Server) artifactHTTP(w http.ResponseWriter, r *http.Request) {
	service := continuity.Service{Store: s.Engine.Store}
	q := r.URL.Query()
	var result any
	var err error
	switch {
	case r.Method == "GET" && r.URL.Path == "/artifact/list":
		result, err = service.ArtifactList(r.Context(), q.Get("target"))
	case r.Method == "GET" && r.URL.Path == "/artifact/show":
		result, err = service.ArtifactView(r.Context(), q.Get("target"), q.Get("id"), q.Get("version"))
	case r.Method == "GET" && r.URL.Path == "/artifact/check":
		result, err = service.CheckArtifact(r.Context(), q.Get("target"), q.Get("id"), q.Get("version"))
	case r.Method == "POST" && r.URL.Path == "/artifact/record":
		if r.Header.Get("Content-Type") != "application/json" {
			writeError(w, 415, fmt.Errorf("application/json required"))
			return
		}
		defer r.Body.Close()
		var body []byte
		body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, continuity.MaxRequest))
		if err == nil {
			var input continuity.ArtifactRequest
			input, err = continuity.DecodeArtifact(body)
			if err == nil {
				result, err = service.RecordArtifact(r.Context(), input, "cli", s.Clock().UTC())
			}
		}
	default:
		writeError(w, 404, fmt.Errorf("unknown artifact route or method"))
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
