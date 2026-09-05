package daemon

import (
	"heimdall/internal/core"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBrowserCredentialCannotControlOrReadTasks(t *testing.T) {
	e, err := core.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	s := &Server{Engine: e, Token: strings.Repeat("a", 64), BrowserToken: strings.Repeat("b", 64), Host: "127.0.0.1:7477", Clock: time.Now}
	for _, path := range []string{"/commands", "/state", "/browser/control", "/browser/state"} {
		r := httptest.NewRequest("POST", path, strings.NewReader(`{}`))
		r.Host = s.Host
		r.Header.Set("Authorization", "Bearer "+s.BrowserToken)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		if w.Code != 401 {
			t.Fatal(path, w.Code)
		}
	}
	r := httptest.NewRequest("POST", "/browser/message", strings.NewReader(`{}`))
	r.Host = s.Host
	r.Header.Set("Authorization", "Bearer "+s.Token)
	w := httptest.NewRecorder()
	s.ServeHTTP(w, r)
	if w.Code != 401 {
		t.Fatal("CLI secret accepted in browser role")
	}
}
