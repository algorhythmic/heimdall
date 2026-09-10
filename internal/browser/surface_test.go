package browser

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"heimdall/internal/surface"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestObservedBrowserSurfacesLifecycleReplayAndIsolation(t *testing.T) {
	dir := t.TempDir()
	// Start with accepted task/workspace/action history so isolation is checked
	// against substantive existing authority, not only empty maps.
	raw, err := os.ReadFile("../../testdata/external-actions/schema20.sql")
	if err != nil {
		t.Fatal(err)
	}
	seed, err := sql.Open("sqlite", filepath.Join(dir, "heimdall.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := seed.Exec(string(raw)); err != nil {
		t.Fatal(err)
	}
	seed.Close()
	db, err := store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { db.Close() }()
	s := NewService(db)
	ctx := context.Background()
	now := time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)
	profile, epoch, connection := model.NewID(), model.NewID(), model.NewID()
	message := func(kind string) Message {
		return Message{V: 1, Type: kind, ID: model.NewID(), Profile: profile, Epoch: epoch, Connection: connection}
	}
	send := func(m Message) []store.Event {
		t.Helper()
		prior, _ := db.Events(ctx)
		if _, err := s.Handle(ctx, m, now); err != nil {
			t.Fatal(err)
		}
		all, _ := db.Events(ctx)
		out := []store.Event{}
		for _, e := range all[len(prior):] {
			if e.Subject == "surface" {
				out = append(out, e)
			}
		}
		return out
	}
	wantVerbs := func(events []store.Event, want ...string) {
		t.Helper()
		got := []string{}
		for _, e := range events {
			got = append(got, e.Verb)
		}
		if !reflect.DeepEqual(got, want) && !(len(got) == 0 && len(want) == 0) {
			t.Fatalf("surface verbs %v, want %v", got, want)
		}
	}
	hello := message("hello")
	hello.ExtensionVersion = "0.6.0"
	send(hello)
	if _, err := s.Control(ctx, Control{ID: model.NewID(), Action: "pair", Profile: profile}, now); err != nil {
		t.Fatal(err)
	}
	baseline, _ := db.State(ctx)
	complete, focus := true, 1
	seq := int64(0)
	inv := func(tabs []model.BrowserTab, full bool) Message {
		seq++
		m := message("inventory")
		m.Sequence, m.ObservedAt, m.Complete, m.FocusedWindow = seq, now.Format(time.RFC3339Nano), &complete, &focus
		m.Tabs = tabs
		if !full {
			m.Delta, m.BaseSequence = true, seq-1
		}
		return m
	}
	a := model.BrowserTab{ID: 1, WindowID: 1, URL: "https://claude.ai/chat/one/?utm_source=x#last", Title: "First"}
	b := a
	b.ID = 2
	b.URL = "https://claude.ai/chat/one"
	wantVerbs(send(inv([]model.BrowserTab{b, a}, true)), "observed", "observed")
	st, _ := db.State(ctx)
	if len(st.ObservedSurfaces) != 1 || len(st.SurfaceContainers) != 2 {
		t.Fatal("content was not shared across containers")
	}
	key := (model.BrowserSurfaceObservation{Profile: profile, Epoch: epoch, TabID: 1}).ContainerKey()
	first := st.SurfaceContainers[key].Observation.SurfaceID
	// Equivalent URL changes preserve content identity; raw locator/title changes
	// remain visible without closing and reopening the content.
	a.URL = "https://claude.ai/chat/one/?fbclid=x"
	a.Title = "Updated"
	m := inv([]model.BrowserTab{a}, false)
	wantVerbs(send(m), "changed")
	wantVerbs(send(m)) // exact delivery retry
	st, _ = db.State(ctx)
	if st.SurfaceContainers[key].Observation.SurfaceID != first {
		t.Fatal("tracking-only navigation changed identity")
	}
	// Actual navigation closes/opens within one epoch-scoped tab.
	a.URL = "https://chatgpt.com/c/two"
	events := send(inv([]model.BrowserTab{a}, false))
	wantVerbs(events, "closed", "opened")
	if events[0].EntityID != first || events[0].CommandID != events[1].CommandID {
		t.Fatal("navigation was not one atomic source command")
	}
	st, _ = db.State(ctx)
	if len(st.ObservedSurfaces) != 2 || st.SurfaceContainers[key].Observation.SurfaceID == first {
		t.Fatal("navigation content missing")
	}
	// Focus and load state still belong to the browser projection. They do not
	// manufacture content changes (attention spans are a later S1 increment).
	a.Active = true
	a.LoadStatus = "complete"
	wantVerbs(send(inv([]model.BrowserTab{a}, false)))
	// An incomplete full inventory does not close unlisted containers.
	complete = false
	wantVerbs(send(inv([]model.BrowserTab{a}, true)))
	st, _ = db.State(ctx)
	if len(st.Browsers[profile].Tabs) != 2 {
		t.Fatal("partial inventory lost a tab")
	}
	complete = true
	m = inv(nil, false)
	m.Removed = []int{2}
	wantVerbs(send(m), "closed")
	// A browser-valid URL with malformed query escapes is still retained by the
	// existing inventory path, with an explicit surface identity coverage gap.
	a.URL = "https://example.test/?q=%zz"
	wantVerbs(send(inv([]model.BrowserTab{a}, false)), "observed")
	st, _ = db.State(ctx)
	if st.SurfaceContainers[key].Observation.Gap != "invalid_pointer" || st.SurfaceContainers[key].Observation.SurfaceID != "" {
		t.Fatal("missing normalization gap")
	}
	a.URL = "https://chatgpt.com/c/two"
	wantVerbs(send(inv([]model.BrowserTab{a}, false)), "observed")
	// Reconnect and epoch loss do not assert that the old content closed.
	connection = model.NewID()
	hello = message("hello")
	hello.ExtensionVersion = "0.6.0"
	wantVerbs(send(hello))
	wantVerbs(send(inv([]model.BrowserTab{a}, true)))
	oldKey := key
	epoch, connection, seq = model.NewID(), model.NewID(), 0
	hello = message("hello")
	hello.ExtensionVersion = "0.6.0"
	wantVerbs(send(hello))
	wantVerbs(send(inv([]model.BrowserTab{a}, true)), "observed")
	st, _ = db.State(ctx)
	if !st.SurfaceContainers[oldKey].Present || len(st.SurfaceContainers) != 3 {
		t.Fatal("epoch loss fabricated closure or reused a container")
	}
	// All non-sensor state is unchanged, including ownership and task authority.
	isolated := model.Clone(st)
	isolated.Browsers, isolated.ObservedSurfaces, isolated.SurfaceContainers, isolated.LastEventID = baseline.Browsers, baseline.ObservedSurfaces, baseline.SurfaceContainers, baseline.LastEventID
	if !reflect.DeepEqual(isolated, baseline) {
		t.Fatal("surface ingestion changed non-sensor state")
	}
	beforeJSON, _ := json.Marshal(st)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = store.Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Replay uses only stored events: no live browser/service is needed.
	replayed, err := db.Replay(ctx)
	if err != nil {
		t.Fatal(err)
	}
	afterJSON, _ := json.Marshal(replayed)
	if !bytes.Equal(beforeJSON, afterJSON) {
		t.Fatal("restart/replay changed authoritative state")
	}
	allBefore, _ := db.Events(ctx)
	s = NewService(db)
	if _, err := s.Handle(ctx, m, now); err == nil {
		t.Fatal("old epoch retry bypassed authority")
	}
	allAfter, _ := db.Events(ctx)
	if !reflect.DeepEqual(allBefore, allAfter) {
		t.Fatal("refused delivery wrote events")
	}
}

func TestSurfaceEventsDoNotPromoteInheritedPartialTabs(t *testing.T) {
	st := model.Empty()
	p := model.BrowserProfile{ID: model.NewID(), Epoch: model.NewID(), LastSequence: 5, LastObservedAt: time.Now().UTC()}
	i, _ := surface.Identify("https://example.test/")
	o := model.BrowserSurfaceObservation{Version: 1, Profile: p.ID, Epoch: p.Epoch, Sequence: 1, TabID: 1, WindowID: 1, Pointer: i.Pointer, SurfaceID: i.ID}
	st.SurfaceContainers[o.ContainerKey()] = model.ObservedSurfaceContainer{Observation: o, Present: true}
	complete := false
	if got := surfaceEvents(st, p, Message{Complete: &complete}); len(got) != 0 {
		t.Fatal("partial absence became closure")
	}
	complete = true
	if got := surfaceEvents(st, p, Message{Complete: &complete, Delta: true}); len(got) != 0 {
		t.Fatal("unreported delta absence became closure")
	}
	p.Tabs = []model.BrowserTab{{ID: 2, WindowID: 1, URL: "https://example.test/"}}
	complete = false
	if got := surfaceEvents(st, p, Message{Complete: &complete}); len(got) != 0 {
		t.Fatal("inherited tab became a fresh observation")
	}
}
