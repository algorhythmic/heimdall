package daemon

import (
	"context"
	"encoding/json"
	"heimdall/internal/authz"
	"heimdall/internal/continuity"
	"heimdall/internal/core"
	"heimdall/internal/model"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestSummaryScopeMovementRevocationAndAuthority(t *testing.T) {
	e, err := core.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	ctx := context.Background()
	now := time.Now().UTC()
	for _, task := range []model.Task{{ID: "root", Title: "Visible root", Type: "project", Status: "active"}, {ID: "alpha", Title: "Visible task", Parent: "root", Type: "project", Status: "active"}, {ID: "beta", Title: "Secret project title", Parent: "root", Type: "project", Status: "active"}} {
		if _, err = e.Execute(ctx, core.Command{ID: model.NewID(), Op: "add", Task: &task}, "cli", now); err != nil {
			t.Fatal(err)
		}
	}
	service := continuity.Service{Store: e.Store}
	d := continuity.DependencyRequest{Version: 1, ID: model.NewID(), Target: "alpha", DependsOn: "beta", ExpectedTaskRevision: 1, ExpectedPrerequisiteRevision: 1, Previous: "none", Op: "add", Reason: "Secret project path /private/source"}
	if _, err = service.Dependency(ctx, d, "cli", now); err != nil {
		t.Fatal(err)
	}
	s := &Server{Engine: e, Token: strings.Repeat("a", 64), BrowserToken: strings.Repeat("b", 64), Host: "127.0.0.1:7477", Clock: func() time.Time { return now }}
	call := func(token, method, path, body string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		r.Host = s.Host
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		s.ServeHTTP(w, r)
		return w
	}
	token := strings.Repeat("c", 64)
	grantID := model.NewID()
	_, err = (authz.Service{Store: e.Store}).Execute(ctx, authz.Request{Version: 1, ID: grantID, Op: "grant.issue", Grant: &authz.IssueInput{Name: "Reader", Target: "root", Subtree: true, TokenHash: authz.HashToken(token), ExpiresAt: now.Add(time.Hour)}}, now)
	if err != nil {
		t.Fatal(err)
	}
	w := call(token, "GET", "/client/summary?target=root&subtree=true&sort=id&limit=1", "")
	if w.Code != 200 {
		t.Fatal(w.Code, w.Body.String())
	}
	var page continuity.ProgressOverview
	json.Unmarshal(w.Body.Bytes(), &page)
	if page.NextCursor == "" {
		t.Fatal("no cursor")
	}
	if w := call(token, "GET", "/client/dependencies?target=alpha", ""); !strings.Contains(w.Body.String(), "Secret project title") {
		t.Fatal("granted prerequisite unavailable", w.Body.String())
	}
	st, _ := e.Store.State(ctx)
	task := st.Tasks["beta"].Task
	task.Parent = ""
	if _, err = e.Execute(ctx, core.Command{ID: model.NewID(), Op: "update", Task: &task}, "cli", now); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/client/summary?target=root&subtree=true&sort=id", "/client/dependencies?target=alpha"} {
		w := call(token, "GET", path, "")
		if w.Code != 200 || !strings.Contains(w.Body.String(), "restricted") {
			t.Fatal(w.Code, w.Body.String())
		}
		for _, secret := range []string{"beta", "Secret project", "/private/source", d.ID} {
			if strings.Contains(w.Body.String(), secret) {
				t.Fatal("moved scope leaked", secret)
			}
		}
	}
	w = call(token, "GET", "/client/summary?target=root&subtree=true&sort=id&limit=1&cursor="+url.QueryEscape(page.NextCursor), "")
	if w.Code != 409 {
		t.Fatal("old scope cursor accepted", w.Code)
	}
	for _, path := range []string{"/dependency/command", "/dependency/list?target=alpha", "/dependency/show?target=alpha&id=" + d.ID, "/progress/summary?target=all"} {
		for _, credential := range []string{token, s.BrowserToken} {
			w := call(credential, "GET", path, "")
			if w.Code != 401 {
				t.Fatal("non-CLI reached privileged route", path, w.Code)
			}
		}
	}
	_, err = (authz.Service{Store: e.Store}).Execute(ctx, authz.Request{Version: 1, ID: model.NewID(), Op: "grant.revoke", GrantID: grantID}, now)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{"/client/summary?target=root&subtree=true", "/client/dependencies?target=alpha"} {
		w := call(token, "GET", path, "")
		if w.Code != 403 || strings.Contains(w.Body.String(), "Secret") {
			t.Fatal("revoked summary exposed data", w.Code, w.Body.String())
		}
	}
}
