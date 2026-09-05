package daemon

import (
	"context"
	"heimdall/internal/core"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestAPIRejectsUntrustedClients(t *testing.T) {
	e, err := core.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	s := &Server{Engine: e, Token: strings.Repeat("a", 64), Host: "127.0.0.1:7477", Clock: time.Now}
	cases := []struct {
		name, method, path, token, origin, host, body string
		want                                          int
	}{
		{"missing auth", "GET", "/state", "", "", "127.0.0.1:7477", "", 401},
		{"page origin", "GET", "/state", s.Token, "https://example.test", "127.0.0.1:7477", "", 401},
		{"bad host", "GET", "/state", s.Token, "", "attacker.test", "", 401},
		{"healthy", "GET", "/health", s.Token, "", s.Host, "", 200},
		{"unknown payload field", "POST", "/commands", s.Token, "", s.Host, `{"command":{"id":"x","op":"tick","actor":"user"}}`, 400},
		{"second JSON body", "POST", "/commands", s.Token, "", s.Host, `{} {}`, 400},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body)).WithContext(context.Background())
			r.Host = tc.host
			if tc.token != "" {
				r.Header.Set("Authorization", "Bearer "+tc.token)
			}
			if tc.origin != "" {
				r.Header.Set("Origin", tc.origin)
			}
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != tc.want {
				t.Fatal(w.Code, w.Body.String())
			}
		})
	}
}
