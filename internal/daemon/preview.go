package daemon

import (
	"fmt"
	"heimdall/internal/model"
	"heimdall/internal/workspace"
	"io"
	"net/http"
)

func (s *Server) previewHTTP(w http.ResponseWriter, r *http.Request) (any, error) {
	if s.Previews == nil {
		return nil, fmt.Errorf("preview service unavailable")
	}
	if r.Method == "GET" {
		switch r.URL.Path {
		case "/workspace/list":
			return s.Previews.List(r.Context(), r.URL.Query().Get("target"), s.Clock().UTC())
		case "/workspace/diff":
			return s.Previews.Diff(r.Context(), r.URL.Query().Get("target"), s.Clock().UTC())
		}
	}
	if r.Method != "POST" || r.Header.Get("Content-Type") != "application/json" {
		return nil, fmt.Errorf("JSON POST required")
	}
	defer r.Body.Close()
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, workspace.PreviewMaxBytes))
	if err != nil {
		return nil, err
	}
	switch r.URL.Path {
	case "/workspace/verify":
		var input workspace.RecoveryRequest
		if err := model.StrictJSON(raw, &input); err != nil {
			return nil, err
		}
		return s.Previews.Verify(r.Context(), input, s.Clock().UTC())
	case "/workspace/preview":
		input, err := workspace.DecodePreview(raw)
		if err != nil {
			return nil, err
		}
		return s.Previews.Build(r.Context(), input, s.Clock().UTC())
	case "/workspace/validate":
		var input workspace.Preview
		if err := model.StrictJSON(raw, &input); err != nil {
			return nil, err
		}
		return s.Previews.Validate(r.Context(), input, s.Clock().UTC())
	}
	return nil, fmt.Errorf("unknown preview route or method")
}
