package daemon

import (
	"fmt"
	"heimdall/internal/model"
	"heimdall/internal/workspace"
	"io"
	"net/http"
	"time"
)

func (s *Server) applicationHTTP(w http.ResponseWriter, r *http.Request) (any, error) {
	service := workspace.Service{Store: s.Engine.Store}
	if r.Method == "GET" && r.URL.Path == "/workspace/application/show" {
		return service.Application(r.Context(), r.URL.Query().Get("target"), r.URL.Query().Get("surface"))
	}
	if r.Method != "POST" || r.URL.Path != "/workspace/application/review" || r.Header.Get("Content-Type") != "application/json" {
		return nil, fmt.Errorf("application review requires JSON POST")
	}
	defer r.Body.Close()
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, workspace.MaxRequest))
	if err != nil {
		return nil, err
	}
	var input workspace.ApplicationRequest
	if err := model.StrictJSON(raw, &input); err != nil {
		return nil, err
	}
	return service.ReviewApplication(r.Context(), input, "cli", time.Now().UTC())
}
