package daemon

import (
	"context"
	"encoding/json"
	"heimdall/internal/authz"
	"heimdall/internal/continuity"
	"heimdall/internal/core"
	"heimdall/internal/model"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestScopedClientsAndRevocation(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	e, err := core.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { e.Close() }()
	now := time.Date(2026, 9, 4, 20, 0, 0, 0, time.UTC)
	for _, task := range []model.Task{
		{ID: "project-one", Title: "Project one", Type: "project", Status: "active"},
		{ID: "child-one", Title: "Child one", Type: "project", Status: "active", Parent: "project-one"},
		{ID: "child-two", Title: "Child two", Type: "project", Status: "active", Parent: "project-one"},
		{ID: "project-two", Title: "Private other project", Type: "project", Status: "active"},
	} {
		if _, err = e.Execute(ctx, core.Command{ID: model.NewID(), Op: "add", Task: &task}, "cli", now); err != nil {
			t.Fatal(err)
		}
	}
	s := &Server{Engine: e, Token: strings.Repeat("a", 64), BrowserToken: strings.Repeat("b", 64), Host: "127.0.0.1:7411", Clock: func() time.Time { return now }}
	call := func(token, method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Host = s.Host
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	issue := func(token, target string, subtree bool) string {
		id := model.NewID()
		g := authz.IssueInput{Name: "Reader", Target: target, Subtree: subtree, TokenHash: authz.HashToken(token), ResourceIDs: []string{}, ExpiresAt: now.Add(time.Hour)}
		req := authz.Request{Version: 1, ID: id, Op: "grant.issue", Grant: &g}
		raw, _ := json.Marshal(req)
		w := call(s.Token, "POST", "/grants/command", string(raw))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		return id
	}
	token := strings.Repeat("c", 64)
	for _, raw := range []string{
		`{"version":1,"id":"99999999999999999999999999999999","op":"grant.issue","actor":"cli"}`,
		`{"version":1,"id":"99999999999999999999999999999999","op":"grant.issue","grant":{"actor":"cli"}}`,
		`{"version":1,"version":1,"id":"99999999999999999999999999999999","op":"grant.issue"}`,
	} {
		if w := call(s.Token, "POST", "/grants/command", raw); w.Code != 400 {
			t.Fatal("forged/ambiguous grant accepted", w.Code)
		}
	}
	id := issue(token, "project-one", true)
	childToken := strings.Repeat("d", 64)
	issue(childToken, "child-one", false)
	for _, target := range []string{"project-one", "child-one", "child-two"} {
		w := call(token, "GET", "/client/task?target="+target, "")
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		if strings.Contains(w.Body.String(), "Private other project") {
			t.Fatal("other project leaked")
		}
	}
	for _, path := range []string{"/client/task?target=project-two", "/client/task?target=missing-task", "/client/history?target=project-two&kind=checkpoint", "/client/context?target=project-two"} {
		if w := call(token, "GET", path, ""); w.Code != 403 {
			t.Fatal(path, w.Code)
		}
	}
	for _, target := range []string{"project-one", "child-two"} {
		if w := call(childToken, "GET", "/client/task?target="+target, ""); w.Code != 403 {
			t.Fatal("child widened scope", target, w.Code)
		}
	}
	if w := call(childToken, "GET", "/client/context?target=child-one", ""); w.Code != 403 {
		t.Fatal("mandatory ancestor exposed", w.Code, w.Body.String())
	}
	if w := call(token, "GET", "/client/context?target=child-one", ""); w.Code != 200 {
		t.Fatal("in-scope ancestor failed", w.Code, w.Body.String())
	}
	for _, path := range []string{"/state", "/events", "/grants", "/continuity/state?target=project-one", "/continuity/command", "/continuity/backup", "/grants/command", "/browser/message"} {
		if w := call(token, "POST", path, `{"actor":"cli"}`); w.Code != 401 {
			t.Fatal("client reached privileged route", path, w.Code)
		}
	}
	if w := call(token, "POST", "/client/task?target=project-one", `{}`); w.Code != 403 {
		t.Fatal("client mutation allowed", w.Code)
	}
	for _, bad := range []string{s.Token, s.BrowserToken, strings.Repeat("e", 64)} {
		if w := call(bad, "GET", "/client/task?target=project-one", ""); w.Code != 403 {
			t.Fatal("wrong credential class accepted", w.Code)
		}
	}
	// Populate several immutable records and verify bounded, scope-bound cursors.
	st, _ := e.Store.State(ctx)
	rev := st.Tasks["project-one"].Revision
	for i := 0; i < 3; i++ {
		_, err := (continuity.Service{Store: e.Store}).Execute(ctx, continuity.Request{Version: 1, ID: model.NewID(), Op: "decision.accept", Target: "project-one", ExpectedTaskRevision: &rev, Decision: &continuity.DecisionInput{Text: "Accepted fixture decision"}}, "cli", now)
		if err != nil {
			t.Fatal(err)
		}
	}
	w := call(token, "GET", "/client/history?target=project-one&kind=decision&limit=1", "")
	var page continuity.Page
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Items) != 1 || page.NextCursor == "" {
		t.Fatal(w.Code, w.Body.String())
	}
	cursor := page.NextCursor
	if w = call(token, "GET", "/client/history?target=project-one&kind=decision&limit=1&cursor="+cursor, ""); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = call(token, "GET", "/client/history?target=child-one&kind=decision&cursor="+cursor, ""); w.Code != 400 {
		t.Fatal("cursor scope", w.Code)
	}
	otherToken := strings.Repeat("f", 64)
	issue(otherToken, "project-one", true)
	if w = call(otherToken, "GET", "/client/history?target=project-one&kind=decision&cursor="+cursor, ""); w.Code != 400 {
		t.Fatal("cursor principal", w.Code)
	}
	if w = call(token, "GET", "/client/history?target=project-one&kind=decision&cursor="+cursor, ""); w.Code != 409 {
		t.Fatal("mixed snapshot cursor", w.Code)
	}
	for _, limit := range []string{"0", "51"} {
		if w = call(token, "GET", "/client/history?target=project-one&kind=decision&limit="+limit, ""); w.Code != 400 {
			t.Fatal("page bound", w.Code)
		}
	}
	// Expiry is checked by the daemon clock, not caller-supplied metadata.
	now = now.Add(2 * time.Hour)
	if w = call(token, "GET", "/client/task?target=project-one", ""); w.Code != 403 {
		t.Fatal("expired grant", w.Code)
	}
	now = now.Add(-2 * time.Hour)
	revoke := authz.Request{Version: 1, ID: model.NewID(), Op: "grant.revoke", GrantID: id}
	raw, _ := json.Marshal(revoke)
	if w = call(s.Token, "POST", "/grants/command", string(raw)); w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	if w = call(s.Token, "POST", "/grants/command", string(raw)); w.Code != 200 {
		t.Fatal("revoke retry", w.Code)
	}
	if w = call(token, "GET", "/client/task?target=project-one", ""); w.Code != 403 {
		t.Fatal("revocation ineffective", w.Code)
	}
	before, _ := e.Store.State(ctx)
	if _, err = e.Replay(ctx); err != nil {
		t.Fatal(err)
	}
	after, _ := e.Store.State(ctx)
	a, _ := json.Marshal(before)
	b, _ := json.Marshal(after)
	if string(a) != string(b) {
		t.Fatal("grant replay differs")
	}
	events, _ := e.Store.Events(ctx)
	serialized, _ := json.Marshal(events)
	if strings.Contains(string(serialized), token) || strings.Contains(string(serialized), childToken) {
		t.Fatal("credential leaked into events")
	}
	if err = e.Close(); err != nil {
		t.Fatal(err)
	}
	e, err = core.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	s.Engine = e
	if w = call(token, "GET", "/client/task?target=project-one", ""); w.Code != 403 {
		t.Fatal("revoked grant revived after restart", w.Code)
	}
	if w = call(childToken, "GET", "/client/task?target=child-one", ""); w.Code != 200 {
		t.Fatal("live grant lost after restart", w.Code)
	}
}
