package daemon

import (
	"bytes"
	"encoding/json"
	"fmt"
	"heimdall/internal/browser"
	"io"
	"net/http"
)

func (s *Server) browserHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" && r.URL.Path == "/browser/state" {
		st, err := s.Engine.Store.State(r.Context())
		if err != nil {
			writeError(w, 500, err)
			return
		}
		writeJSON(w, map[string]any{"profiles": st.Browsers, "operations": st.BrowserOperations})
		return
	}
	if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
		writeError(w, 400, fmt.Errorf("POST application/json required"))
		return
	}
	defer r.Body.Close()
	r.Body = http.MaxBytesReader(w, r.Body, browser.MaxFrame)
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, 400, err)
		return
	}
	service := browser.Service{Store: s.Engine.Store}
	var result any
	switch r.URL.Path {
	case "/browser/message":
		var m browser.Message
		m, err = browser.Decode(body)
		if err == nil {
			result, err = service.Handle(r.Context(), m, s.Clock().UTC())
		}
	case "/browser/control":
		var c browser.Control
		d := json.NewDecoder(bytes.NewReader(body))
		d.DisallowUnknownFields()
		err = d.Decode(&c)
		if err == nil {
			if extra := d.Decode(new(any)); extra != io.EOF {
				err = fmt.Errorf("expected one command")
			}
		}
		if err == nil {
			result, err = service.Control(r.Context(), c, s.Clock().UTC())
		}
	default:
		writeError(w, 404, fmt.Errorf("unknown browser route"))
		return
	}
	if err != nil {
		writeError(w, 400, err)
		return
	}
	writeJSON(w, result)
}
