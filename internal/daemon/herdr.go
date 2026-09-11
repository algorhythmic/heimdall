package daemon

import (
	"fmt"
	"heimdall/internal/adapters/herdr"
	"heimdall/internal/model"
	"heimdall/internal/workspace"
	"io"
	"net/http"
)

func (s *Server) herdrHTTP(w http.ResponseWriter, r *http.Request) (any, error) {
	service := workspace.HerdrService{Store: s.Engine.Store, Adapter: herdr.Adapter{}}
	if r.Method == "GET" && r.URL.Path == "/workspace/herdr/refresh" {
		q := r.URL.Query()
		return service.Refresh(r.Context(), q.Get("target"), q.Get("surface"), q.Get("binding"), s.Clock().UTC())
	}
	if r.Method != "POST" || (r.URL.Path != "/workspace/herdr/bind" && r.URL.Path != "/workspace/herdr/publish" && r.URL.Path != "/workspace/herdr/jump") {
		return nil, fmt.Errorf("unknown Herdr route or method")
	}
	if r.Header.Get("Content-Type") != "application/json" {
		return nil, fmt.Errorf("application/json required")
	}
	defer r.Body.Close()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, workspace.MaxRequest))
	if err != nil {
		return nil, err
	}
	if r.URL.Path == "/workspace/herdr/bind" {
		var input workspace.HerdrBindRequest
		if err := model.StrictJSON(body, &input); err != nil {
			return nil, err
		}
		return service.Bind(r.Context(), input, "cli", s.Clock().UTC())
	}
	if r.URL.Path == "/workspace/herdr/jump" {
		var input workspace.HerdrJumpRequest
		if err := model.StrictJSON(body, &input); err != nil {
			return nil, err
		}
		return service.Jump(r.Context(), input, "cli", s.Clock().UTC())
	}
	var input workspace.HerdrPublishRequest
	if err := model.StrictJSON(body, &input); err != nil {
		return nil, err
	}
	return service.Publish(r.Context(), input, "cli", s.Clock().UTC())
}
