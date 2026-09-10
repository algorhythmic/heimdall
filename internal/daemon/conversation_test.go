package daemon

import (
	"heimdall/internal/core"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestConversationsCLIAuthAndReadOnly(t *testing.T) {
	e, err := core.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	s := &Server{Engine: e, Token: strings.Repeat("a", 64), BrowserToken: strings.Repeat("b", 64), Host: "127.0.0.1:7477", Clock: time.Now}
	for _, tc := range []struct {
		method, path, token, origin string
		status                      int
	}{
		{"GET", "/conversations", s.Token, "", 200},
		{"GET", "/conversations", "", "", 401},
		{"GET", "/conversations", s.BrowserToken, "", 401},
		{"GET", "/conversations", s.Token, "https://example.test", 401},
		{"POST", "/conversations", s.Token, "", 405},
		{"GET", "/conversations?target=missing", s.Token, "", 400},
		{"GET", "/conversations?target=a&target=b", s.Token, "", 400},
		{"GET", "/conversations?write=1", s.Token, "", 400},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Host = s.Host
		r.Header.Set("Authorization", "Bearer "+tc.token)
		r.Header.Set("Origin", tc.origin)
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Fatal(tc, w.Code, w.Body.String())
		}
		if tc.status == 200 && strings.TrimSpace(w.Body.String()) != "[]" {
			t.Fatal("empty conversations should be array", w.Body.String())
		}
	}
}
