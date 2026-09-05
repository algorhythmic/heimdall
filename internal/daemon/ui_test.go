package daemon

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"heimdall/internal/core"
	"heimdall/internal/model"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type uiFixture struct {
	t   *testing.T
	s   *Server
	now time.Time
}

func newUIFixture(t *testing.T) *uiFixture {
	t.Helper()
	engine, err := core.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { engine.Close() })
	f := &uiFixture{t: t, now: time.Now().UTC()}
	f.s = &Server{Engine: engine, Token: strings.Repeat("a", 64), BrowserToken: strings.Repeat("b", 64), Host: "127.0.0.1:7477", Clock: func() time.Time { return f.now }}
	for _, root := range []string{"workspace", "private"} {
		task := model.Task{ID: root, Title: root + " <img src=x onerror=alert(1)>", Type: "project", Status: "active", Done: model.Done{Text: "Complete child", Checks: []model.Check{{ID: "children", Kind: "children_done"}}}}
		f.command(core.Command{Op: "add", Task: &task})
		child := model.Task{ID: root + "-child", Parent: root, Title: "Child of " + root, Type: "project", Status: "active", Done: model.Done{Text: "Manual review"}}
		f.command(core.Command{Op: "add", Task: &child})
	}
	return f
}
func (f *uiFixture) command(c core.Command) {
	f.t.Helper()
	c.ID = model.NewID()
	if _, err := f.s.Engine.Execute(context.Background(), c, "cli", f.now); err != nil {
		f.t.Fatal(err)
	}
}
func (f *uiFixture) request(method, path string, body any, cookie *http.Cookie, csrf, origin, bearer string) *httptest.ResponseRecorder {
	f.t.Helper()
	raw, _ := json.Marshal(body)
	r := httptest.NewRequest(method, path, strings.NewReader(string(raw)))
	r.Host = f.s.Host
	if body != nil {
		r.Header.Set("Content-Type", "application/json")
	}
	if cookie != nil {
		r.AddCookie(cookie)
	}
	if csrf != "" {
		r.Header.Set("X-Heimdall-CSRF", csrf)
	}
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	if bearer != "" {
		r.Header.Set("Authorization", "Bearer "+bearer)
	}
	w := httptest.NewRecorder()
	f.s.ServeHTTP(w, r)
	return w
}
func (f *uiFixture) login() (*http.Cookie, string, string) {
	f.t.Helper()
	w := f.request("POST", "/ui-bootstrap", map[string]string{"target": "workspace"}, nil, "", "", f.s.Token)
	if w.Code != 200 {
		f.t.Fatal(w.Code, w.Body.String())
	}
	var boot map[string]any
	json.Unmarshal(w.Body.Bytes(), &boot)
	code := boot["code"].(string)
	if strings.Contains(boot["url"].(string), code) || strings.Contains(w.Body.String(), f.s.Token) {
		f.t.Fatal("credential exposed in bootstrap URL")
	}
	w = f.request("POST", "/ui/bootstrap", map[string]string{"code": code}, nil, "", "http://"+f.s.Host, "")
	if w.Code != 200 {
		f.t.Fatal(w.Code, w.Body.String())
	}
	var auth map[string]any
	json.Unmarshal(w.Body.Bytes(), &auth)
	cookies := w.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || cookies[0].SameSite != http.SameSiteStrictMode {
		f.t.Fatal("weak session cookie", cookies)
	}
	return cookies[0], auth["csrf"].(string), code
}
func TestUISessionScopeAndOrigin(t *testing.T) {
	f := newUIFixture(t)
	page := f.request("GET", "/ui/", nil, nil, "", "", "")
	if page.Code != 200 || !strings.Contains(page.Header().Get("Content-Security-Policy"), "frame-ancestors 'none'") || !strings.Contains(page.Body.String(), "One-time code") {
		t.Fatal("public login page", page.Code)
	}
	if w := f.request("GET", "/ui/session", nil, nil, "", "", ""); w.Code != 401 {
		t.Fatal("unauthenticated session", w.Code)
	}
	if w := f.request("POST", "/ui-bootstrap", map[string]string{"target": "workspace"}, nil, "", "", f.s.BrowserToken); w.Code != 401 {
		t.Fatal("browser created UI credentials")
	}
	cookie, csrf, code := f.login()
	if w := f.request("POST", "/ui/bootstrap", map[string]string{"code": code}, nil, "", "http://"+f.s.Host, ""); w.Code != 401 {
		t.Fatal("bootstrap code reused")
	}
	if w := f.request("GET", "/ui/feed", nil, cookie, "", "", ""); w.Code != 200 || strings.Contains(w.Body.String(), "private") {
		t.Fatal("feed scope leak", w.Code, w.Body.String())
	}
	if w := f.request("GET", "/ui/task?target=private", nil, cookie, "", "", ""); w.Code != 403 || strings.Contains(w.Body.String(), "Child of private") {
		t.Fatal("wrong task visible", w.Code)
	}
	if w := f.request("GET", "/ui/task?target=workspace-child", nil, cookie, "", "", ""); w.Code != 200 {
		t.Fatal("descendant context unavailable", w.Code, w.Body.String())
	}
	if w := f.request("POST", "/ui/logout", map[string]any{}, cookie, csrf, "http://evil.example", ""); w.Code != 403 {
		t.Fatal("cross-origin mutation allowed")
	}
	if w := f.request("POST", "/ui/logout", map[string]any{}, cookie, "", "http://"+f.s.Host, ""); w.Code != 403 {
		t.Fatal("missing CSRF allowed")
	}
	if w := f.request("POST", "/ui/logout", map[string]any{}, cookie, csrf, "http://"+f.s.Host, ""); w.Code != 200 {
		t.Fatal("logout failed")
	}
	if w := f.request("GET", "/ui/feed", nil, cookie, "", "", ""); w.Code != 401 {
		t.Fatal("revoked session remained valid")
	}
}
func TestUICursorBindingAndExpiry(t *testing.T) {
	f := newUIFixture(t)
	cookie, _, _ := f.login()
	w := f.request("GET", "/ui/feed", nil, cookie, "", "", "")
	var feed map[string]any
	json.Unmarshal(w.Body.Bytes(), &feed)
	cursor := feed["cursor"].(string)
	w = f.request("GET", "/ui/feed?cursor="+cursor, nil, cookie, "", "", "")
	json.Unmarshal(w.Body.Bytes(), &feed)
	if w.Code != 200 || feed["changed"] != false {
		t.Fatal("unchanged feed repeated snapshot", w.Code)
	}
	another, _, _ := f.login()
	if w = f.request("GET", "/ui/feed?cursor="+cursor, nil, another, "", "", ""); w.Code != 409 {
		t.Fatal("cross-session cursor accepted")
	}
	raw, _ := base64.RawURLEncoding.DecodeString(cursor)
	var c uiCursor
	json.Unmarshal(raw, &c)
	c.Event = 1 << 60
	raw, _ = json.Marshal(c)
	if w = f.request("GET", "/ui/feed?cursor="+base64.RawURLEncoding.EncodeToString(raw), nil, cookie, "", "", ""); w.Code != 409 {
		t.Fatal("future cursor accepted")
	}
	f.now = f.now.Add(2 * time.Hour)
	if w = f.request("GET", "/ui/feed", nil, cookie, "", "", ""); w.Code != 401 {
		t.Fatal("expired session accepted")
	}
}
func TestUIReviewAuthorityAndRetry(t *testing.T) {
	f := newUIFixture(t)
	f.command(core.Command{Op: "complete", Target: "workspace-child"})
	f.command(core.Command{Op: "complete", Target: "private-child"})
	cookie, csrf, _ := f.login()
	st, _ := f.s.Engine.Store.State(context.Background())
	proposal, other := "", ""
	for id, p := range st.Proposals {
		if p.Target == "workspace" {
			proposal = id
		}
		if p.Target == "private" {
			other = id
		}
	}
	if proposal == "" || other == "" {
		t.Fatal("missing aggregate fixture")
	}
	request := map[string]any{"id": model.NewID(), "proposal": other, "action": "accept", "revision": st.Revision}
	if w := f.request("POST", "/ui/review", request, cookie, csrf, "http://"+f.s.Host, ""); w.Code == 200 {
		t.Fatal("wrong workstream accepted")
	}
	request["proposal"] = proposal
	request["id"] = model.NewID()
	w := f.request("POST", "/ui/review", request, cookie, csrf, "http://"+f.s.Host, "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	first := w.Body.String()
	w = f.request("POST", "/ui/review", request, cookie, csrf, "http://"+f.s.Host, "")
	if w.Code != 200 || w.Body.String() != first {
		t.Fatal("review retry changed", w.Code, w.Body.String())
	}
	st, _ = f.s.Engine.Store.State(context.Background())
	if st.Tasks["workspace"].Task.Status != "done" || st.Tasks["private"].Task.Status != "active" {
		t.Fatal("review changed another task")
	}
	f.request("POST", "/ui/logout", map[string]any{}, cookie, csrf, "http://"+f.s.Host, "")
	if w = f.request("POST", "/ui/review", request, cookie, csrf, "http://"+f.s.Host, ""); w.Code != 401 {
		t.Fatal("revoked review retry accepted")
	}
}

func TestUIFeedDetectsContinuityWithoutTaskRevisionChange(t *testing.T) {
	f := newUIFixture(t)
	st, _ := f.s.Engine.Store.State(context.Background())
	g := model.Grant{Target: "workspace", Subtree: true}
	before, _ := uiSnapshot(st, g)
	st.CheckpointHeads["workspace"] = "first"
	withFirst, _ := uiSnapshot(st, g)
	st.CheckpointHeads["workspace"] = "second"
	withSecond, _ := uiSnapshot(st, g)
	if model.ContentDigest(before) == model.ContentDigest(withFirst) || model.ContentDigest(withFirst) == model.ContentDigest(withSecond) {
		t.Fatal("checkpoint advancement did not refresh detail")
	}
	st.Evidence["evaluation"] = model.Evidence{ID: "evaluation", Target: "workspace", Status: "started"}
	started, _ := uiSnapshot(st, g)
	st.Evidence["evaluation"] = model.Evidence{ID: "evaluation", Target: "workspace", Status: "finished", Outcome: "matched"}
	finished, _ := uiSnapshot(st, g)
	if model.ContentDigest(started) == model.ContentDigest(finished) {
		t.Fatal("evaluation completion did not refresh detail")
	}
}
