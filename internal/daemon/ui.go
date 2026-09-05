package daemon

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"heimdall/internal/continuity"
	"heimdall/internal/core"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"heimdall/internal/webui"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

type uiSession struct {
	ID, Secret, CSRF, Target string
	Resources                []string
	Expires                  time.Time
}
type uiBootstrap struct {
	Target    string
	Resources []string
	Expires   time.Time
}
type uiCursor struct {
	Session string `json:"session"`
	Target  string `json:"target"`
	Event   int64  `json:"event"`
	Digest  string `json:"digest"`
}

func uiHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func (s *Server) pruneUI() {
	now := s.Clock()
	for key, v := range s.uiCodes {
		if !now.Before(v.Expires) {
			delete(s.uiCodes, key)
		}
	}
	for key, v := range s.uiSessions {
		if !now.Before(v.Expires) {
			delete(s.uiSessions, key)
		}
	}
}
func (s *Server) uiBootstrapHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		writeError(w, 405, fmt.Errorf("POST required"))
		return
	}
	var input struct {
		Target string `json:"target"`
	}
	if err := uiDecode(w, r, &input); err != nil {
		writeError(w, 400, err)
		return
	}
	st, err := s.Engine.Store.State(r.Context())
	if err != nil {
		writeError(w, 500, err)
		return
	}
	task, step, err := model.ResolveTarget(st, input.Target)
	if err != nil || step != nil || task.Task.Parent != "" {
		writeError(w, 400, fmt.Errorf("UI scope must be a root task; open its workstream"))
		return
	}
	g := model.Grant{Target: input.Target, Subtree: true}
	resources := []string{}
	for id, resource := range st.Resources {
		if resource.Active && g.Contains(st, resource.Target) {
			resources = append(resources, id)
		}
	}
	sort.Strings(resources)
	if len(resources) > 256 {
		writeError(w, 422, fmt.Errorf("UI resource scope exceeds 256 bindings"))
		return
	}
	code := model.NewID()
	expires := s.Clock().Add(5 * time.Minute)
	s.uiMu.Lock()
	defer s.uiMu.Unlock()
	s.pruneUI()
	if s.uiCodes == nil {
		s.uiCodes = map[string]uiBootstrap{}
	}
	if len(s.uiCodes) >= 32 {
		writeError(w, 429, fmt.Errorf("too many pending sign-in codes"))
		return
	}
	s.uiCodes[uiHash(code)] = uiBootstrap{Target: input.Target, Resources: resources, Expires: expires}
	writeJSON(w, map[string]any{"url": "http://" + s.Host + "/ui/", "code": code, "expires_at": expires, "target": input.Target, "resource_ids": resources})
}
func uiDecode(w http.ResponseWriter, r *http.Request, out any) error {
	if r.Header.Get("Content-Type") != "application/json" {
		return fmt.Errorf("application/json required")
	}
	defer r.Body.Close()
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
	if err != nil {
		return err
	}
	return model.StrictJSON(raw, out)
}
func (s *Server) authenticatedUI(r *http.Request) (uiSession, bool) {
	cookie, err := r.Cookie("heimdall_ui")
	if err != nil || len(cookie.Value) != 64 {
		return uiSession{}, false
	}
	s.uiMu.Lock()
	defer s.uiMu.Unlock()
	s.pruneUI()
	session, ok := s.uiSessions[uiHash(cookie.Value)]
	return session, ok
}
func (s *Server) uiHTTP(w http.ResponseWriter, r *http.Request) {
	origin := "http://" + s.Host
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'")
	if r.Host != s.Host || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != origin) || r.Header.Get("Sec-Fetch-Site") == "cross-site" {
		writeError(w, 403, fmt.Errorf("same-origin UI required"))
		return
	}
	if r.Method == "GET" {
		asset := ""
		mime := ""
		switch r.URL.Path {
		case "/ui/":
			asset = "index.html"
			mime = "text/html; charset=utf-8"
		case "/ui/app.js":
			asset = "app.js"
			mime = "text/javascript; charset=utf-8"
		case "/ui/style.css":
			asset = "style.css"
			mime = "text/css; charset=utf-8"
		}
		if asset != "" {
			body, err := webui.Assets.ReadFile("assets/" + asset)
			if err != nil {
				writeError(w, 404, err)
				return
			}
			w.Header().Set("Content-Type", mime)
			w.Write(body)
			return
		}
	}
	if r.Method == "POST" && r.Header.Get("Origin") != origin {
		writeError(w, 403, fmt.Errorf("UI mutation requires exact Origin"))
		return
	}
	if r.URL.Path == "/ui/bootstrap" && r.Method == "POST" {
		var input struct {
			Code string `json:"code"`
		}
		if err := uiDecode(w, r, &input); err != nil {
			writeError(w, 400, err)
			return
		}
		s.uiMu.Lock()
		defer s.uiMu.Unlock()
		s.pruneUI()
		boot, ok := s.uiCodes[uiHash(input.Code)]
		if !ok || !model.OpaqueID.MatchString(input.Code) {
			writeError(w, 401, fmt.Errorf("code expired or invalid"))
			return
		}
		if s.uiSessions == nil {
			s.uiSessions = map[string]uiSession{}
		}
		if len(s.uiSessions) >= 32 {
			writeError(w, 429, fmt.Errorf("session limit reached"))
			return
		}
		secret := model.NewID() + model.NewID()
		session := uiSession{ID: model.NewID(), Secret: uiHash(secret), CSRF: model.NewID(), Target: boot.Target, Resources: boot.Resources, Expires: s.Clock().Add(time.Hour)}
		delete(s.uiCodes, uiHash(input.Code))
		s.uiSessions[session.Secret] = session
		http.SetCookie(w, &http.Cookie{Name: "heimdall_ui", Value: secret, Path: "/ui", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: 3600})
		writeJSON(w, map[string]any{"csrf": session.CSRF, "target": session.Target, "expires_at": session.Expires})
		return
	}
	session, ok := s.authenticatedUI(r)
	if !ok {
		writeError(w, 401, fmt.Errorf("UI session expired; request a new sign-in code"))
		return
	}
	if r.Method == "POST" && r.Header.Get("X-Heimdall-CSRF") != session.CSRF {
		writeError(w, 403, fmt.Errorf("CSRF token required"))
		return
	}
	if r.URL.Path == "/ui/session" && r.Method == "GET" {
		writeJSON(w, map[string]any{"csrf": session.CSRF, "target": session.Target, "expires_at": session.Expires})
		return
	}
	if r.URL.Path == "/ui/logout" && r.Method == "POST" {
		s.uiMu.Lock()
		delete(s.uiSessions, session.Secret)
		s.uiMu.Unlock()
		http.SetCookie(w, &http.Cookie{Name: "heimdall_ui", Value: "", Path: "/ui", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: -1})
		writeJSON(w, map[string]bool{"signed_out": true})
		return
	}
	if r.Method == "POST" && r.URL.Path == "/ui/review" {
		s.uiReview(w, r, session)
		return
	}
	if r.Method != "GET" {
		writeError(w, 405, fmt.Errorf("unsupported UI method"))
		return
	}
	var result any
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	err := s.Engine.Store.Inspect(ctx, func(st model.State) error {
		if !s.uiCurrent(session) {
			return fmt.Errorf("session expired")
		}
		g := model.Grant{ID: session.ID, Target: session.Target, Subtree: true, ResourceIDs: session.Resources}
		switch r.URL.Path {
		case "/ui/feed":
			snapshot, err := uiSnapshot(st, g)
			if err != nil {
				return err
			}
			digest := model.ContentDigest(snapshot)
			cursor := uiCursor{Session: session.ID, Target: session.Target, Event: st.LastEventID, Digest: digest}
			encoded, _ := json.Marshal(cursor)
			changed := true
			if raw := r.URL.Query().Get("cursor"); raw != "" {
				if len(raw) > 1024 {
					return store.ErrConflict
				}
				b, err := base64.RawURLEncoding.DecodeString(raw)
				var old uiCursor
				if err != nil || model.StrictJSON(b, &old) != nil || old.Session != session.ID || old.Target != session.Target || old.Event > st.LastEventID || old.Event < st.LastEventID-10000 {
					return store.ErrConflict
				}
				changed = old.Digest != digest
			}
			result = map[string]any{"changed": changed, "cursor": base64.RawURLEncoding.EncodeToString(encoded)}
			if changed {
				result.(map[string]any)["snapshot"] = snapshot
			}
		case "/ui/task":
			target := r.URL.Query().Get("target")
			if !g.Contains(st, target) {
				return fmt.Errorf("target outside UI scope")
			}
			bundle, err := continuity.ScopedContext(ctx, st, g, target, 100000)
			if err != nil {
				return err
			}
			evidence := []model.Evidence{}
			for _, v := range st.Evidence {
				if v.Target == target {
					evidence = append(evidence, v)
				}
			}
			sort.Slice(evidence, func(i, j int) bool { return evidence[i].SourceEvent > evidence[j].SourceEvent })
			truncated := len(evidence) > 50
			if truncated {
				evidence = evidence[:50]
			}
			invalid := map[string]model.EvidenceInvalidation{}
			for _, v := range evidence {
				if reason, ok := st.EvidenceInvalidations[v.ID]; ok {
					invalid[v.ID] = reason
				}
			}
			proposals := []model.Proposal{}
			for _, p := range st.Proposals {
				if p.Target == target {
					proposals = append(proposals, p)
				}
			}
			sort.Slice(proposals, func(i, j int) bool { return proposals[i].CreatedAt.After(proposals[j].CreatedAt) })
			if len(proposals) > 50 {
				proposals = proposals[:50]
				truncated = true
			}
			result = map[string]any{"context": bundle, "evidence": evidence, "invalidations": invalid, "proposals": proposals, "checks": core.Evaluate(st, target), "revision": st.Revision, "truncated": truncated}
		default:
			return fmt.Errorf("unknown UI route")
		}
		raw, _ := json.Marshal(result)
		if len(raw) > 512<<10 {
			return fmt.Errorf("UI response exceeds limit")
		}
		return nil
	})
	if err != nil {
		status := 403
		if errors.Is(err, store.ErrConflict) {
			status = 409
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, result)
}
func (s *Server) uiCurrent(session uiSession) bool {
	s.uiMu.Lock()
	defer s.uiMu.Unlock()
	current, ok := s.uiSessions[session.Secret]
	return ok && current.ID == session.ID && s.Clock().Before(current.Expires)
}
func uiSnapshot(st model.State, g model.Grant) (any, error) {
	tasks := []model.TaskRecord{}
	for _, r := range st.Tasks {
		if g.Contains(st, r.Task.ID) {
			tasks = append(tasks, r)
		}
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].Task.ID < tasks[j].Task.ID })
	count := len(tasks)
	truncated := count > 200
	if truncated {
		tasks = tasks[:200]
	}
	pending := 0
	for _, p := range st.Proposals {
		if p.Status == "pending" && g.Contains(st, p.Target) {
			pending++
		}
	}
	checkpoints := 0
	content := map[string]any{}
	for target := range st.CheckpointHeads {
		if g.Contains(st, target) {
			checkpoints++
			content["checkpoint:"+target] = st.CheckpointHeads[target]
		}
	}
	// Continuity/evidence events do not advance the task document revision. Their
	// permitted content must still invalidate the selected task's displayed detail.
	for target, id := range st.ContractHeads {
		if g.Contains(st, target) {
			content["contract:"+target] = id
		}
	}
	for id, d := range st.Decisions {
		if g.Contains(st, d.Target) {
			content["decision:"+id] = id
		}
	}
	for id, d := range st.Evaluators {
		if g.Contains(st, d.Target) {
			content["evaluator:"+id] = id
		}
	}
	for id, e := range st.Evidence {
		if g.Contains(st, e.Target) {
			content["evidence:"+id] = model.ContentDigest(e)
			if invalid, ok := st.EvidenceInvalidations[id]; ok {
				content["invalid:"+id] = invalid
			}
		}
	}
	for id, r := range st.Resources {
		if g.Contains(st, r.Target) {
			content["resource:"+id] = r.Active
		}
	}
	for id, p := range st.Proposals {
		if g.Contains(st, p.Target) {
			content["proposal:"+id] = p.Status
		}
	}
	return map[string]any{"tasks": tasks, "task_count": count, "pending_reviews": pending, "checkpoint_count": checkpoints, "truncated": truncated, "revision": st.Revision, "target": g.Target, "content_digest": model.ContentDigest(content)}, nil
}
func (s *Server) uiReview(w http.ResponseWriter, r *http.Request, session uiSession) {
	var input struct {
		ID       string `json:"id"`
		Proposal string `json:"proposal"`
		Action   string `json:"action"`
		Revision int64  `json:"revision"`
	}
	if err := uiDecode(w, r, &input); err != nil {
		writeError(w, 400, err)
		return
	}
	if !model.OpaqueID.MatchString(input.ID) || !model.Contains([]string{"accept", "reject"}, input.Action) || input.Revision < 1 {
		writeError(w, 400, fmt.Errorf("review identity, revision and action required"))
		return
	}
	guard := func(st model.State) error {
		g := model.Grant{Target: session.Target, Subtree: true}
		p, ok := st.Proposals[input.Proposal]
		if !s.uiCurrent(session) || !ok || !g.Contains(st, p.Target) {
			return fmt.Errorf("review outside current session scope")
		}
		ids, err := model.ResourceScope(st, p.Target)
		if err != nil {
			return err
		}
		for _, id := range ids {
			if !model.Contains(session.Resources, id) {
				return fmt.Errorf("resource scope changed; sign in again")
			}
		}
		return nil
	}
	raw, err := s.Engine.ExecuteChecked(r.Context(), core.Command{ID: "ui:" + session.ID + ":" + input.ID, Op: "ratify", Target: input.Proposal, Action: input.Action, ExpectedRevision: &input.Revision}, "ui:"+session.ID, s.Clock().UTC(), guard)
	if err != nil {
		status := 400
		if errors.Is(err, store.ErrConflict) {
			status = 409
		}
		writeError(w, status, err)
		return
	}
	writeJSON(w, raw)
}

// Keep routes deliberately separate from the CLI bearer credential boundary.
func isUIPath(path string) bool { return strings.HasPrefix(path, "/ui/") }
