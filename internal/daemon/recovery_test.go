package daemon

import (
	"heimdall/internal/core"
	"heimdall/internal/workspace"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestRecoveryRouteRequiresCLIAndRejectsAuthorityFields(t *testing.T) {
	e, err := core.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	s := &Server{Engine: e, Token: strings.Repeat("a", 64), BrowserToken: strings.Repeat("b", 64), Host: "127.0.0.1:7477", Clock: time.Now, Previews: &workspace.PreviewService{Store: e.Store}}
	for _, tc := range []struct {
		token, body string
		status      int
	}{
		{s.BrowserToken, `{"version":1,"target":"alpha","placement_policy":"saved"}`, 401},
		{strings.Repeat("c", 64), `{}`, 401},
		{s.Token, `{"version":1,"target":"alpha","placement_policy":"saved","full":true}`, 400},
		{s.Token, `{"version":1,"version":1,"target":"alpha","placement_policy":"saved"}`, 400},
		{s.Token, `{"version":1,"target":"alpha","placement_policy":"named-monitor-clamp"}`, 400},
	} {
		r := httptest.NewRequest("POST", "/workspace/verify", strings.NewReader(tc.body))
		r.Host = s.Host
		r.Header.Set("Authorization", "Bearer "+tc.token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatal(w.Code, w.Body.String())
		}
	}
}
