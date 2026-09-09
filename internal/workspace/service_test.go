package workspace

import (
	"context"
	"encoding/json"
	"errors"
	"heimdall/internal/core"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

type fixture struct {
	t   *testing.T
	e   *core.Engine
	s   Service
	ctx context.Context
	now time.Time
}

func setup(t *testing.T) *fixture {
	t.Helper()
	e, err := core.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.Close() })
	f := &fixture{t, e, Service{e.Store}, context.Background(), time.Date(2026, 9, 8, 18, 0, 0, 0, time.UTC)}
	for _, id := range []string{"alpha", "beta"} {
		task := model.Task{ID: id, Title: id, Type: "project", Status: "active", Done: model.Done{Text: "Review work"}}
		if _, err := e.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "add", Task: &task}, "cli", f.now); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

func (f *fixture) request(op, target string) Request {
	st, err := f.e.Store.State(f.ctx)
	if err != nil {
		f.t.Fatal(err)
	}
	return Request{Version: 1, ID: model.NewID(), Op: op, Target: target, ExpectedTaskRevision: st.Tasks[target].Revision}
}

func (f *fixture) send(r Request) json.RawMessage {
	f.t.Helper()
	b, err := f.s.Execute(f.ctx, r, "cli", f.now)
	if err != nil {
		f.t.Fatal(r.Op, err)
	}
	return b
}

func (f *fixture) reject(r Request) error {
	f.t.Helper()
	before, _ := f.e.Store.State(f.ctx)
	events, _ := f.e.Store.Events(f.ctx)
	_, err := f.s.Execute(f.ctx, r, "cli", f.now)
	if err == nil {
		f.t.Fatal("accepted invalid request", r.Op)
	}
	after, _ := f.e.Store.State(f.ctx)
	newEvents, _ := f.e.Store.Events(f.ctx)
	if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(events, newEvents) {
		f.t.Fatal("rejected write leaked state or events")
	}
	return err
}

func (f *fixture) manifest(target string) (Request, model.WorkspaceManifest) {
	r := f.request("workspace.accept", target)
	r.Manifest = &ManifestInput{Previous: "none", Name: "Planning", Surfaces: []model.DesiredSurface{{ID: model.NewID(), Kind: "terminal", Label: "Planning shell", Required: true, RestorePolicy: "manual"}}}
	var m model.WorkspaceManifest
	if err := json.Unmarshal(f.send(r), &m); err != nil {
		f.t.Fatal(err)
	}
	return r, m
}

func (f *fixture) binding(m model.WorkspaceManifest) Request {
	r := f.request("session.bind", m.Target)
	r.Session = &SessionInput{Previous: "none", ManifestID: m.ID, SurfaceID: m.Surfaces[0].ID, Locator: &model.SessionLocator{Adapter: "generic", Environment: "local", Host: "test-host", SourceEpoch: "server-epoch-1", SessionID: "planning", WorkspaceID: "workspace-1", PaneID: "pane-1", Platform: "linux", Cwd: "/synthetic/shared-worktree", Repository: "/synthetic/repository", Worktree: "/synthetic/shared-worktree"}}
	return r
}

func TestExplicitOwnershipAndInertReplay(t *testing.T) {
	f := setup(t)
	tasksBefore, err := os.ReadFile(filepath.Join(f.e.Dir, "tasks.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	_, a := f.manifest("alpha")
	_, b := f.manifest("beta")
	ab := f.binding(a)
	accepted := f.send(ab)
	bb := f.binding(b)
	f.reject(bb)
	bb.Session.Locator.WorkspaceID = "moved-workspace"
	f.reject(bb) // moving a pane cannot double-claim it
	bb.Session.Locator.PaneID = "pane-2"
	f.send(bb) // same cwd and repository are not ownership
	for target, id := range map[string]string{"alpha": ab.ID, "beta": bb.ID} {
		v, err := f.s.View(f.ctx, target)
		if err != nil || len(v.Bindings) != 1 || v.Bindings[0].Head != id || v.Bindings[0].Status != "unverified" {
			t.Fatal(v, err)
		}
	}
	if _, err := f.s.Manifest(f.ctx, "beta", a.ID); err == nil {
		t.Fatal("cross-task manifest lookup")
	}
	if _, err := f.s.Binding(f.ctx, "beta", a.Surfaces[0].ID, ab.ID); err == nil {
		t.Fatal("cross-task binding lookup")
	}
	if _, err := f.s.Binding(f.ctx, "alpha", b.Surfaces[0].ID, ab.ID); err == nil {
		t.Fatal("cross-surface binding lookup")
	}
	if _, err := f.s.Execute(f.ctx, ab, "client:"+model.NewID(), f.now); err == nil {
		t.Fatal("client could write")
	}
	if !reflect.DeepEqual(accepted, f.send(ab)) {
		t.Fatal("retry receipt changed")
	}
	changed := model.Clone(ab)
	changed.Session.Locator.Cwd = "/another-cwd"
	if err := f.reject(changed); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	// These paths never existed. Replaying declarations must not inspect them,
	// resolve runtime identities, start applications or change task state.
	before, _ := f.e.Store.State(f.ctx)
	replayed, err := f.e.Store.Replay(f.ctx)
	if err != nil || !reflect.DeepEqual(before, replayed) {
		t.Fatal("replay differs", err)
	}
	if !reflect.DeepEqual(accepted, f.send(ab)) {
		t.Fatal("replay lost exact retry")
	}
	tasksAfter, _ := os.ReadFile(filepath.Join(f.e.Dir, "tasks.yaml"))
	if string(tasksBefore) != string(tasksAfter) {
		t.Fatal("workspace traffic changed tasks.yaml")
	}
}

func TestLifecycleRevisionAndRetiredIdentity(t *testing.T) {
	f := setup(t)
	mr, m := f.manifest("alpha")
	br := f.binding(m)
	f.send(br)
	drop := model.Clone(mr)
	drop.ID = model.NewID()
	drop.Manifest.Previous = m.ID
	drop.Manifest.Surfaces = []model.DesiredSurface{}
	f.reject(drop) // explicit unbind is required before desired membership removal
	unbound := f.request("session.unbind", "alpha")
	unbound.Session = &SessionInput{Previous: br.ID, ManifestID: m.ID, SurfaceID: br.Session.SurfaceID}
	f.send(unbound)
	f.send(drop)
	adopt := f.request("workspace.accept", "beta")
	adopt.Manifest = model.Clone(mr.Manifest)
	f.reject(adopt) // even retired IDs retain their task identity
	readd := model.Clone(mr)
	readd.ID = model.NewID()
	readd.Manifest.Previous = drop.ID
	f.send(readd)
	br.ID = model.NewID()
	br.Session.Previous = unbound.ID
	br.Session.ManifestID = readd.ID
	br.Session.Locator.SourceEpoch = "server-epoch-2"
	f.send(br)
	st, _ := f.e.Store.State(f.ctx)
	task := st.Tasks["alpha"].Task
	task.Title = "Changed task"
	if _, err := f.e.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "update", Target: "alpha", Task: &task}, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	v, err := f.s.View(f.ctx, "alpha")
	if err != nil || !model.Contains(v.Issues, "manifest_task_changed") || !model.Contains(v.Bindings[0].Issues, "binding_task_changed") {
		t.Fatal(v, err)
	}
	stale := model.Clone(br)
	stale.ID = model.NewID()
	stale.Session.Previous = br.ID
	if err := f.reject(stale); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	stale.ExpectedTaskRevision = v.TaskRevision
	if err := f.reject(stale); !errors.Is(err, store.ErrConflict) {
		t.Fatal(err)
	}
	// A user may always explicitly unbind a stale declaration using current
	// task/head preconditions; doing so does not require reviving the old task.
	unbound = f.request("session.unbind", "alpha")
	unbound.Session = &SessionInput{Previous: br.ID, ManifestID: readd.ID, SurfaceID: br.Session.SurfaceID}
	f.send(unbound)
	double := model.Clone(unbound)
	double.ID = model.NewID()
	double.Session.Previous = unbound.ID
	f.reject(double)
	br.ID = model.NewID()
	br.Session.Previous = unbound.ID
	br.ExpectedTaskRevision = v.TaskRevision
	refresh := model.Clone(readd)
	refresh.ID = model.NewID()
	refresh.Manifest.Previous = readd.ID
	refresh.ExpectedTaskRevision = v.TaskRevision
	f.send(refresh)
	br.Session.ManifestID = refresh.ID
	f.send(br)
	v, _ = f.s.View(f.ctx, "alpha")
	if len(v.Issues) != 0 || len(v.Bindings[0].Issues) != 1 || v.Bindings[0].Status != "unverified" {
		t.Fatal(v)
	}
	if _, err := f.s.Binding(f.ctx, "alpha", br.Session.SurfaceID, unbound.ID); err != nil {
		t.Fatal("unbind history lost", err)
	}
}

func TestCompetingManifestAndBindingHeads(t *testing.T) {
	f := setup(t)
	mr, m := f.manifest("alpha")
	compete := func(a, b Request) {
		t.Helper()
		var wg sync.WaitGroup
		results := make(chan error, 2)
		for _, r := range []Request{a, b} {
			wg.Go(func() { _, err := f.s.Execute(f.ctx, r, "cli", f.now); results <- err })
		}
		wg.Wait()
		close(results)
		accepted, conflicts := 0, 0
		for err := range results {
			if err == nil {
				accepted++
			} else if errors.Is(err, store.ErrConflict) {
				conflicts++
			} else {
				t.Fatal(err)
			}
		}
		if accepted != 1 || conflicts != 1 {
			t.Fatal(accepted, conflicts)
		}
	}
	a := f.binding(m)
	b := model.Clone(a)
	b.ID = model.NewID()
	compete(a, b)
	a = model.Clone(mr)
	a.ID = model.NewID()
	a.Manifest.Previous = m.ID
	b = model.Clone(a)
	b.ID = model.NewID()
	compete(a, b)
	v, _ := f.s.View(f.ctx, "alpha")
	if !model.Contains(v.Bindings[0].Issues, "binding_manifest_changed") {
		t.Fatal("manifest change silently refreshed binding", v)
	}
}

func TestReplayRejectsForgedWorkspaceEventsWithoutMutation(t *testing.T) {
	f := setup(t)
	_, m := f.manifest("alpha")
	f.send(f.binding(m))
	events, err := f.e.Store.Events(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	st := model.Empty()
	for _, e := range events {
		if e.Subject == "workspace" || e.Subject == "session" {
			var original map[string]any
			if err := json.Unmarshal(e.Payload, &original); err != nil {
				t.Fatal(err)
			}
			mutations := map[string]func(*store.Event, map[string]any){
				"version":       func(_ *store.Event, p map[string]any) { p["version"] = 99 },
				"actor":         func(_ *store.Event, p map[string]any) { p["actor"] = "scheduler" },
				"time":          func(_ *store.Event, p map[string]any) { p["at"] = f.now.Add(time.Hour) },
				"scope":         func(_ *store.Event, p map[string]any) { p["target"] = "missing-task" },
				"revision":      func(_ *store.Event, p map[string]any) { p["task_revision"] = 99 },
				"head":          func(_ *store.Event, p map[string]any) { p["previous"] = model.NewID() },
				"command":       func(e *store.Event, _ map[string]any) { e.CommandID = "not-the-record-command" },
				"unknown-field": func(_ *store.Event, p map[string]any) { p["command"] = "touch /tmp/should-never-run" },
			}
			for name, mutate := range mutations {
				t.Run(e.Subject+"/"+name, func(t *testing.T) {
					copy := model.Clone(st)
					p := model.Clone(original)
					bad := e
					mutate(&bad, p)
					bad.Payload, _ = json.Marshal(p)
					if err := store.Apply(&copy, bad); err == nil {
						t.Fatal("forged event accepted")
					}
					if !reflect.DeepEqual(copy, st) {
						t.Fatal("failed reducer partially mutated state")
					}
				})
			}
		}
		if err := store.Apply(&st, e); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRequestFixturesAndStrictNegatives(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/workspace/requests.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []json.RawMessage
	if err = json.Unmarshal(raw, &fixtures); err != nil || len(fixtures) != 3 {
		t.Fatal(err)
	}
	for _, b := range fixtures {
		if _, err := Decode(b); err != nil {
			t.Fatal(err)
		}
	}
	var base Request
	json.Unmarshal(fixtures[0], &base)
	for name, mutate := range map[string]func(*Request){
		"version":               func(r *Request) { r.Version = 2 },
		"missing-head":          func(r *Request) { r.Manifest.Previous = "" },
		"implicit-surface-list": func(r *Request) { r.Manifest.Surfaces = nil },
		"duplicate-surface":     func(r *Request) { r.Manifest.Surfaces = append(r.Manifest.Surfaces, r.Manifest.Surfaces[0]) },
		"launch-policy":         func(r *Request) { r.Manifest.Surfaces[0].RestorePolicy = "launch" },
		"step-target":           func(r *Request) { r.Target = "alpha/step" },
		"control":               func(r *Request) { r.Manifest.Name = "bad\x1b[2J" },
		"too-many-surfaces":     func(r *Request) { r.Manifest.Surfaces = make([]model.DesiredSurface, 33) },
	} {
		t.Run(name, func(t *testing.T) {
			r := model.Clone(base)
			mutate(&r)
			b, _ := json.Marshal(r)
			if _, err := Decode(b); err == nil {
				t.Fatal("invalid request accepted")
			}
		})
	}
	for _, raw := range []string{`{"version":1,"version":1}`, string(fixtures[0]) + `{}`, strings.Replace(string(fixtures[0]), `"name":`, `"launch_command":"bad","name":`, 1), strings.Repeat(" ", MaxRequest+1)} {
		if _, err := Decode([]byte(raw)); err == nil {
			t.Fatal("non-strict request accepted")
		}
	}
}
