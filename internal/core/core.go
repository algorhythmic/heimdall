// Package core implements the local command, evidence, and task-editing boundary.
package core

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
	"heimdall/internal/capture"
	"heimdall/internal/model"
	"heimdall/internal/store"
)

//go:embed defaults/types.yaml
var defaultTypes []byte

type Command struct {
	ID               string          `json:"id"`
	Op               string          `json:"op"`
	ExpectedRevision *int64          `json:"expected_revision,omitempty"`
	Document         *model.Document `json:"document,omitempty"`
	Task             *model.Task     `json:"task,omitempty"`
	Target           string          `json:"target,omitempty"`
	Action           string          `json:"action,omitempty"`
	Line             string          `json:"line,omitempty"`
	Pointer          string          `json:"pointer,omitempty"`
	Title            string          `json:"title,omitempty"`
	Client           string          `json:"client,omitempty"`
	Origin           string          `json:"origin_id,omitempty"`
	Targets          []string        `json:"targets,omitempty"`
}
type Result struct {
	ID       string `json:"id"`
	Revision int64  `json:"revision"`
	Data     any    `json:"data"`
}
type Engine struct {
	ValidateEvidence func(context.Context, model.State, string) error
	mu               sync.Mutex
	Store            *store.Store
	Dir              string
	Catalog          model.Catalog
	viewHash         string
	viewError        string
}

func Open(dir string) (*Engine, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	s, err := store.Open(abs)
	if err != nil {
		return nil, err
	}
	fail := func(err error) (*Engine, error) { s.Close(); return nil, err }
	path := filepath.Join(abs, "types.yaml")
	if err = writeNew(path, defaultTypes); err != nil && !errors.Is(err, os.ErrExist) {
		return fail(err)
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return fail(err)
	}
	catalog, err := model.ParseCatalog(b)
	if err != nil {
		return fail(err)
	}
	e := &Engine{Store: s, Dir: abs, Catalog: catalog}
	st, err := s.State(context.Background())
	if err != nil {
		return fail(err)
	}
	b, err = yaml.Marshal(st.Document())
	if err != nil {
		return fail(err)
	}
	path = filepath.Join(abs, "tasks.yaml")
	err = writeNew(path, b)
	if err == nil {
		e.viewHash = digest(b)
	} else if !errors.Is(err, os.ErrExist) {
		return fail(err)
	} else {
		current, err := os.ReadFile(path)
		if err != nil {
			return fail(err)
		}
		d, err := model.ParseDocument(current)
		if err == nil && reflect.DeepEqual(d, st.Document()) {
			e.viewHash = digest(current)
		}
	}
	return e, nil
}
func (e *Engine) Close() error { return e.Store.Close() }
func digest(b []byte) string   { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func writeNew(path string, b []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err != nil {
		return err
	}
	return closeErr
}
func atomicWrite(path string, b []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".heimdall-view-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(b)
	}
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err != nil {
		return err
	}
	if ce != nil {
		return ce
	}
	return os.Rename(name, path)
}

// replaceView never replaces a concurrently recreated destination. The detached
// original remains in history, including writes through an editor's old handle.
// Hard-link support is required for atomic no-clobber publication; failure leaves
// recoverable files and a pending view instead of using a lossy rename fallback.
func replaceView(path string, out []byte, expected string, afterDetach func()) error {
	history := filepath.Join(filepath.Dir(path), "task-file-history")
	if err := os.MkdirAll(history, 0700); err != nil {
		return err
	}
	backup := filepath.Join(history, model.NewID()+".yaml")
	if err := os.Rename(path, backup); err != nil {
		return err
	}
	restore := func() { _ = os.Link(backup, path) }
	if afterDetach != nil {
		afterDetach()
	}
	actual, err := os.ReadFile(backup)
	if err != nil {
		restore()
		return err
	}
	if digest(actual) != expected {
		restore()
		return fmt.Errorf("concurrent file edit preserved at %s", backup)
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".heimdall-view-*")
	if err != nil {
		restore()
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if err = f.Chmod(0600); err == nil {
		_, err = f.Write(out)
	}
	if err == nil {
		err = f.Sync()
	}
	ce := f.Close()
	if err == nil {
		err = ce
	}
	if err == nil {
		err = os.Link(tmp, path)
	}
	if err != nil {
		restore()
		return fmt.Errorf("view publication failed; original retained at %s: %w", backup, err)
	}
	return nil
}
func (e *Engine) ViewError() string { e.mu.Lock(); defer e.mu.Unlock(); return e.viewError }
func (e *Engine) Execute(ctx context.Context, c Command, actor string, now time.Time) (json.RawMessage, error) {
	return e.ExecuteChecked(ctx, c, actor, now, nil)
}

// ExecuteChecked evaluates caller authority inside the writer transaction,
// before cached receipts are exposed and again before fresh events commit.
func (e *Engine) ExecuteChecked(ctx context.Context, c Command, actor string, now time.Time, authorize func(model.State) error) (json.RawMessage, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	r, err := e.executeChecked(ctx, c, actor, now, authorize)
	if err == nil {
		e.publish(ctx)
	}
	return r, err
}
func (e *Engine) execute(ctx context.Context, c Command, actor string, now time.Time) (json.RawMessage, error) {
	return e.executeChecked(ctx, c, actor, now, nil)
}
func (e *Engine) executeChecked(ctx context.Context, c Command, actor string, now time.Time, authorize func(model.State) error) (json.RawMessage, error) {
	if c.ID == "" {
		return nil, fmt.Errorf("command id required")
	}
	intent := c
	intent.ExpectedRevision = nil
	request, _ := json.Marshal(intent)
	return e.Store.TransactChecked(ctx, c.ID, actor, request, now, authorize, func(st model.State) (store.Change, error) {
		if c.ExpectedRevision != nil && *c.ExpectedRevision != st.Revision {
			return store.Change{}, store.ErrConflict
		}
		b := builder{state: model.Clone(st), now: now.UTC(), cmdID: c.ID, catalog: e.Catalog, events: []store.Pending{}, ctx: ctx, validateEvidence: e.ValidateEvidence}
		data, err := b.run(c)
		if err != nil {
			return store.Change{}, err
		}
		if err = b.reconcile(); err != nil {
			return store.Change{}, err
		}
		return store.Change{Revision: b.state.Revision, Events: b.events, Result: Result{c.ID, b.state.Revision, data}}, nil
	})
}
func (e *Engine) ReconcileFile(ctx context.Context, now time.Time) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	b, err := os.ReadFile(filepath.Join(e.Dir, "tasks.yaml"))
	if err != nil {
		e.viewError = err.Error()
		return err
	}
	h := digest(b)
	if h == e.viewHash {
		return nil
	}
	d, err := model.ParseDocument(b)
	if err != nil {
		e.viewError = err.Error()
		return err
	}
	_, err = e.execute(ctx, Command{ID: "file-" + model.NewID(), Op: "replace", Document: &d}, "file", now)
	if err != nil {
		e.viewError = err.Error()
		return err
	}
	e.viewHash = h
	e.publish(ctx)
	return nil
}
func (e *Engine) publish(ctx context.Context) {
	st, err := e.Store.State(ctx)
	if err != nil {
		e.viewError = err.Error()
		return
	}
	out, err := yaml.Marshal(st.Document())
	if err != nil {
		e.viewError = err.Error()
		return
	}
	path := filepath.Join(e.Dir, "tasks.yaml")
	current, err := os.ReadFile(path)
	if err != nil || digest(current) != e.viewHash {
		e.viewError = "task file changed: preserved edits; current view is tasks.pending.yaml"
		_ = atomicWrite(filepath.Join(e.Dir, "tasks.pending.yaml"), out)
		return
	}
	// Preserve comments/formatting when this is already the accepted semantic view.
	d, parseErr := model.ParseDocument(current)
	if parseErr == nil && reflect.DeepEqual(d, st.Document()) {
		e.viewError = ""
		return
	}
	if err = replaceView(path, out, e.viewHash, nil); err != nil {
		e.viewError = err.Error()
		_ = atomicWrite(filepath.Join(e.Dir, "tasks.pending.yaml"), out)
		return
	}
	e.viewHash = digest(out)
	e.viewError = ""
	_ = os.Remove(filepath.Join(e.Dir, "tasks.pending.yaml"))
}
func (e *Engine) Format(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	st, err := e.Store.State(ctx)
	if err != nil {
		return err
	}
	p := filepath.Join(e.Dir, "tasks.yaml")
	b, err := os.ReadFile(p)
	if err != nil {
		return err
	}
	d, err := model.ParseDocument(b)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(d, st.Document()) {
		return fmt.Errorf("fmt requires the current accepted task view")
	}
	out, err := yaml.Marshal(d)
	if err != nil {
		return err
	}
	if err = replaceView(p, out, digest(b), nil); err != nil {
		return err
	}
	e.viewHash = digest(out)
	return nil
}
func (e *Engine) Replay(ctx context.Context) (model.State, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.Store.Replay(ctx)
}

type builder struct {
	ctx              context.Context
	validateEvidence func(context.Context, model.State, string) error
	state            model.State
	now              time.Time
	cmdID            string
	catalog          model.Catalog
	events           []store.Pending
}

func (b *builder) emit(subject, verb, id string, payload any) error {
	p, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err = store.Apply(&b.state, store.Event{Version: 1, Subject: subject, Verb: verb, EntityID: id, Payload: p}); err != nil {
		return err
	}
	b.events = append(b.events, store.Pending{Subject: subject, Verb: verb, EntityID: id, Payload: payload})
	return nil
}
func (b *builder) run(c Command) (any, error) {
	switch c.Op {
	case "replace":
		if c.Document == nil {
			return nil, fmt.Errorf("document required")
		}
		return nil, b.replace(model.Clone(*c.Document))
	case "add":
		if c.Task == nil {
			return nil, fmt.Errorf("task required")
		}
		d := b.state.Document()
		t := model.Clone(*c.Task)
		if t.Type == "" {
			t.Type = "project"
		}
		w, ok := b.catalog.Types[t.Type]
		if !ok {
			return nil, fmt.Errorf("unknown type")
		}
		if t.Subtasks == nil {
			t.Subtasks = model.Clone(w.Subtasks)
		}
		d.Tasks = append(d.Tasks, t)
		if err := b.replace(d); err != nil {
			return nil, err
		}
		return map[string]string{"task_id": b.lastTaskID()}, nil
	case "update":
		if c.Task == nil {
			return nil, fmt.Errorf("task required")
		}
		if _, ok := b.state.Tasks[c.Task.ID]; !ok {
			return nil, fmt.Errorf("unknown task")
		}
		d := b.state.Document()
		for i := range d.Tasks {
			if d.Tasks[i].ID == c.Task.ID {
				d.Tasks[i] = *c.Task
			}
		}
		return nil, b.replace(d)
	case "complete", "reopen", "drop":
		return nil, b.transition(c.Target, c.Op)
	case "capture":
		return b.capture(c)
	case "assign":
		return nil, b.assign(c)
	case "ratify":
		return nil, b.ratify(c)
	case "tick":
		return nil, b.tick()
	default:
		return nil, fmt.Errorf("unsupported command %q", c.Op)
	}
}
func (b *builder) lastTaskID() string {
	for i := len(b.events) - 1; i >= 0; i-- {
		if b.events[i].Subject == "task" && b.events[i].Verb == "created" {
			return b.events[i].EntityID
		}
	}
	return ""
}
func (b *builder) replace(d model.Document) error {
	if d.Revision != b.state.Revision {
		return store.ErrConflict
	}
	for i := range d.Tasks {
		t := &d.Tasks[i]
		if t.ID == "" {
			t.ID = "task-" + model.NewID()[:12]
		}
		w, ok := b.catalog.Types[t.Type]
		if !ok {
			return fmt.Errorf("unknown workflow %q", t.Type)
		}
		model.Defaults(t, w)
	}
	if err := model.Validate(d, b.catalog); err != nil {
		return err
	}
	sort.Slice(d.Tasks, func(i, j int) bool { return d.Tasks[i].ID < d.Tasks[j].ID })
	incoming := map[string]bool{}
	for _, t := range d.Tasks {
		incoming[t.ID] = true
	}
	// Omitted terminal tasks remain archived in the edit view and event history.
	for id, r := range b.state.Tasks {
		if !incoming[id] {
			if !terminal(r) {
				return fmt.Errorf("cannot omit active task %s; drop it explicitly", id)
			}
			d.Tasks = append(d.Tasks, r.Task)
		}
	}
	if err := model.Validate(d, b.catalog); err != nil {
		return err
	}
	changed := false
	for _, t := range d.Tasks {
		old, exists := b.state.Tasks[t.ID]
		if exists && reflect.DeepEqual(old.Task, t) {
			continue
		}
		if exists && old.Task.Type != t.Type {
			return fmt.Errorf("task type migration is not supported; create a new task")
		}
		r := model.TaskRecord{Task: t, Revision: 1, Workflow: model.Clone(b.catalog.Types[t.Type]), CreatedAt: b.now, UpdatedAt: b.now, Completed: map[string]model.Stamp{}}
		verb := "created"
		fields := []string{"*"}
		if exists {
			r.Revision = old.Revision + 1
			r.Workflow = old.Workflow
			r.CreatedAt = old.CreatedAt
			r.Completed = model.Clone(old.Completed)
			verb = "updated"
			fields = model.ChangedFields(old.Task, t)
			if r.Completed == nil {
				r.Completed = map[string]model.Stamp{}
			}
		}
		live := map[string]bool{}
		for _, s := range t.Subtasks {
			if s.Status == "done" {
				live[s.ID] = true
				if _, ok := r.Completed[s.ID]; !ok {
					r.Completed[s.ID] = model.Stamp{At: b.now, Token: b.cmdID + "#" + s.ID}
				}
			}
		}
		for id := range r.Completed {
			if !live[id] {
				delete(r.Completed, id)
			}
		}
		if exists && old.Task.Status != t.Status {
			if model.Contains(r.Workflow.Success, t.Status) {
				verb = "completed"
			} else if model.Contains(r.Workflow.Dropped, t.Status) {
				verb = "dropped"
			} else if terminal(old) {
				verb = "reopened"
			}
		}
		if err := b.emit("task", verb, t.ID, store.TaskChange{Record: r, Fields: fields}); err != nil {
			return err
		}
		changed = true
	}
	if changed {
		b.state.Revision++
	}
	return nil
}
func terminal(r model.TaskRecord) bool {
	return model.Contains(r.Workflow.Success, r.Task.Status) || model.Contains(r.Workflow.Dropped, r.Task.Status)
}
func (b *builder) transition(target, op string) error {
	parts := strings.Split(target, "#")
	if len(parts) > 2 {
		return fmt.Errorf("invalid target")
	}
	r, ok := b.state.Tasks[parts[0]]
	if !ok {
		return fmt.Errorf("unknown task")
	}
	t := model.Clone(r.Task)
	if len(parts) == 1 {
		switch op {
		case "complete":
			t.Status = r.Workflow.Success[0]
		case "drop":
			t.Status = r.Workflow.Dropped[0]
		case "reopen":
			t.Status = r.Workflow.Initial
		}
	}
	if len(parts) == 2 {
		if terminal(r) && op != "reopen" {
			return fmt.Errorf("task is terminal")
		}
		found := false
		for i := range t.Subtasks {
			if t.Subtasks[i].ID != parts[1] {
				continue
			}
			found = true
			if op == "complete" {
				for _, dep := range t.Subtasks[i].After {
					for _, s := range t.Subtasks {
						if s.ID == dep && s.Status != "done" {
							return fmt.Errorf("prerequisite %s is not done", dep)
						}
					}
				}
				t.Subtasks[i].Status = "done"
			} else if op == "drop" {
				t.Subtasks[i].Status = "dropped"
			} else {
				t.Subtasks[i].Status = "open"
				if terminal(r) {
					t.Status = r.Workflow.Initial
				}
			}
		}
		if !found {
			return fmt.Errorf("unknown subtask")
		}
	}
	d := b.state.Document()
	for i := range d.Tasks {
		if d.Tasks[i].ID == t.ID {
			d.Tasks[i] = t
		}
	}
	return b.replace(d)
}
func (b *builder) capture(c Command) (any, error) {
	line, err := capture.Parse(c.Line)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(c.Pointer) == "" || c.Client == "" {
		return nil, fmt.Errorf("pointer and client required")
	}
	if err = b.validateTargets(line.Targets); err != nil {
		return nil, err
	}
	origin := c.Origin
	if line.Origin {
		if origin != "" {
			return nil, fmt.Errorf("use either ^ or origin_id")
		}
		var latest time.Time
		ambiguous := false
		for _, v := range b.state.Captures {
			if v.Client != c.Client || v.CreatedAt.After(b.now) || b.now.Sub(v.CreatedAt) > 30*time.Minute {
				continue
			}
			if v.CreatedAt.After(latest) {
				origin = v.ID
				latest = v.CreatedAt
				ambiguous = false
			} else if v.CreatedAt.Equal(latest) {
				ambiguous = true
			}
		}
		if origin == "" || ambiguous {
			return nil, fmt.Errorf("origin: no unambiguous capture in this client's last 30 minutes")
		}
	}
	if origin != "" {
		v, ok := b.state.Captures[origin]
		if !ok || v.Client != c.Client {
			return nil, fmt.Errorf("origin is not in this client scope")
		}
	}
	v := model.Capture{ID: model.NewID(), Client: c.Client, Pointer: c.Pointer, Title: c.Title, Targets: line.Targets, Kind: line.Kind, Why: line.Why, Origin: origin, CreatedAt: b.now}
	v.ExpiresAt = b.captureDeadline(v)
	if err = b.emit("capture", "created", v.ID, v); err != nil {
		return nil, err
	}
	return map[string]string{"capture_id": v.ID}, nil
}
func (b *builder) validateTargets(targets []string) error {
	if len(targets) == 0 {
		return fmt.Errorf("targets required")
	}
	seen := map[string]bool{}
	for _, id := range targets {
		if seen[id] {
			return fmt.Errorf("duplicate target")
		}
		seen[id] = true
		if id == "unassigned" {
			if len(targets) != 1 {
				return fmt.Errorf("unassigned must be alone")
			}
		} else if r, ok := b.state.Tasks[id]; !ok || terminal(r) {
			return fmt.Errorf("unknown or terminal stream %s", id)
		}
	}
	return nil
}
func (b *builder) captureDeadline(v model.Capture) *time.Time {
	var due *time.Time
	add := func(t time.Time) {
		if due == nil || t.Before(*due) {
			x := t
			due = &x
		}
	}
	if model.Contains(v.Targets, "unassigned") {
		add(v.CreatedAt.Add(72 * time.Hour))
	}
	if v.Kind == "candidate" {
		add(v.CreatedAt.Add(14 * 24 * time.Hour))
	}
	if v.Kind == "study" {
		for _, id := range v.Targets {
			if r, ok := b.state.Tasks[id]; ok && r.Task.ResumeBy != "" {
				d, _ := time.Parse("2006-01-02", r.Task.ResumeBy)
				add(d.Add(24*time.Hour - time.Second))
			}
		}
	}
	return due
}
func (b *builder) assign(c Command) error {
	v, ok := b.state.Captures[c.Target]
	if !ok || v.Expired {
		return fmt.Errorf("unknown or expired capture")
	}
	if err := b.validateTargets(c.Targets); err != nil {
		return err
	}
	v.Targets = c.Targets
	v.ExpiresAt = b.captureDeadline(v)
	return b.emit("capture", "assigned", v.ID, v)
}
func sortedKeys[T any](m map[string]T) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}
