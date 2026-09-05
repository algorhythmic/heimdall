package daemon

import (
	"heimdall/internal/core"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEvidenceRoutesDoNotDelegateExecution(t *testing.T) {
	e, err := core.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	s := &Server{Engine: e, Token: strings.Repeat("a", 64), BrowserToken: strings.Repeat("b", 64), Host: "127.0.0.1:7477", Clock: time.Now}
	for _, path := range []string{"/evidence/configure", "/evidence/evaluate", "/evidence/refresh", "/evidence/state"} {
		for _, token := range []string{s.BrowserToken, strings.Repeat("c", 64)} {
			r := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
			r.Host = s.Host
			r.Header.Set("Authorization", "Bearer "+token)
			r.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			s.ServeHTTP(w, r)
			if w.Code != 401 {
				t.Fatal(path, w.Code)
			}
		}
	}
	for _, body := range []string{`{"actor":"cli"}`, `{"version":1,"version":1}`, `{"outcome":"matched"}`} {
		r := httptest.NewRequest("POST", "/evidence/evaluate", strings.NewReader(body))
		r.Host = s.Host
		r.Header.Set("Authorization", "Bearer "+s.Token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 400 {
			t.Fatal("forged or ambiguous request accepted", body, w.Code)
		}
	}
}
