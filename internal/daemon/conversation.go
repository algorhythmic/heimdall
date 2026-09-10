package daemon

import (
	"fmt"
	"net/http"
	"time"
)

// This handler is reached only after the ordinary CLI bearer/host/origin guard.
func (s *Server) conversationsHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, fmt.Errorf("conversations is read-only"))
		return
	}
	query := r.URL.Query()
	for key, values := range query {
		if key != "target" || len(values) != 1 {
			writeError(w, 400, fmt.Errorf("conversations accepts one optional target"))
			return
		}
	}
	now := time.Now().UTC()
	if s.Clock != nil {
		now = s.Clock().UTC()
	}
	view, err := s.Engine.Store.Conversations(r.Context(), query.Get("target"), now)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	writeJSON(w, view)
}
