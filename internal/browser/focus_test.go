package browser

import (
	"context"
	"encoding/json"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"reflect"
	"testing"
	"time"
)

func TestBrowserFocusSpansCommitReplayAndRejectOverlap(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewService(db)
	ctx := context.Background()
	now := time.Now().UTC().Truncate(time.Millisecond)
	profile, epoch, connection := model.NewID(), model.NewID(), model.NewID()
	base := func(kind string) Message {
		return Message{V: 1, ID: model.NewID(), Type: kind, Profile: profile, Epoch: epoch, Connection: connection}
	}
	hello := base("hello")
	hello.ExtensionVersion = "0.6.1"
	if _, err = s.Handle(ctx, hello, now); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Control(ctx, Control{ID: model.NewID(), Action: "pair", Profile: profile}, now); err != nil {
		t.Fatal(err)
	}
	complete, window := true, 1
	tab := model.BrowserTab{ID: 1, WindowID: 1, Active: true, URL: "https://example.test/", Title: "One"}
	m := base("inventory")
	m.Sequence = 1
	m.ObservedAt = now.Format(time.RFC3339Nano)
	m.Complete = &complete
	m.FocusedWindow = &window
	m.Tabs = []model.BrowserTab{tab}
	if _, err = s.Handle(ctx, m, now); err != nil {
		t.Fatal(err)
	}
	start := now
	now = now.Add(5 * time.Second)
	window = -1
	m.ID = model.NewID()
	m.Sequence = 2
	m.ObservedAt = now.Format(time.RFC3339Nano)
	m.FocusSpans = []model.BrowserFocusSpan{{Version: 1, TabID: 1, WindowID: 1, Pointer: tab.URL, StartedAt: start, EndedAt: now, DurationSeconds: 5}}
	if _, err = s.Handle(ctx, m, now); err != nil {
		t.Fatal(err)
	}
	before, _ := db.State(ctx)
	span := before.SurfaceFocusSpans[profile]
	if span.TabID != 1 || span.DurationSeconds != 5 || span.Epoch != epoch || span.Sequence != 2 {
		t.Fatal("focus projection", span)
	}
	if _, err = s.Handle(ctx, m, now); err != nil {
		t.Fatal(err)
	}
	after, _ := db.State(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("duplicate span delivery changed state")
	}
	replayed, err := db.Replay(ctx)
	if err != nil || !reflect.DeepEqual(before, replayed) {
		t.Fatal("focus replay drift", err)
	}
	// Overlapping reports under another request id cannot inflate attention.
	m.ID = model.NewID()
	m.Sequence = 3
	if _, err = s.Handle(ctx, m, now); err == nil {
		t.Fatal("overlap accepted")
	}
	after, _ = db.State(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("rejected span left inventory/receipt changes")
	}
	// A report that does not end focus, or changes source identity, fails too.
	m.FocusSpans[0].StartedAt = now
	m.FocusSpans[0].EndedAt = now.Add(5 * time.Second)
	m.FocusSpans[0].DurationSeconds = 5
	now = now.Add(5 * time.Second)
	m.ObservedAt = now.Format(time.RFC3339Nano)
	window = 1
	m.ID = model.NewID()
	if _, err = s.Handle(ctx, m, now); err == nil {
		t.Fatal("span without blur accepted")
	}
	for _, patch := range []func(*Message){
		func(m *Message) { m.FocusSpans[0].DurationSeconds = 1 },
		func(m *Message) { m.FocusSpans[0].DurationSeconds = 4 },
		func(m *Message) { m.FocusSpans[0].Version = 2 },
		func(m *Message) { m.Type = "poll" },
	} {
		bad := model.Clone(m)
		patch(&bad)
		if bad.Validate() == nil {
			t.Fatal("invalid focus wire accepted")
		}
	}
}

func TestActiveRequestsFreshReadbackWithoutAction(t *testing.T) {
	db, err := store.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	s := NewService(db)
	ctx := context.Background()
	profile, epoch, connection := model.NewID(), model.NewID(), model.NewID()
	base := func(kind string) Message {
		return Message{V: 1, ID: model.NewID(), Type: kind, Profile: profile, Epoch: epoch, Connection: connection}
	}
	hello := base("hello")
	hello.ExtensionVersion = "0.6.1"
	hello.VerificationProtocol = 1
	if _, err = s.Handle(ctx, hello, time.Now()); err != nil {
		t.Fatal(err)
	}
	if _, err = s.Control(ctx, Control{ID: model.NewID(), Action: "pair", Profile: profile}, time.Now()); err != nil {
		t.Fatal(err)
	}
	complete, focused := true, 1
	tab := model.BrowserTab{ID: 1, WindowID: 1, Active: true, URL: "https://example.test/"}
	m := base("inventory")
	m.Sequence = 1
	m.ObservedAt = time.Now().UTC().Format(time.RFC3339Nano)
	m.Complete = &complete
	m.FocusedWindow = &focused
	m.Tabs = []model.BrowserTab{tab}
	if _, err = s.Handle(ctx, m, time.Now()); err != nil {
		t.Fatal(err)
	}
	type response struct {
		state ActiveState
		err   error
	}
	result := make(chan response, 1)
	callCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	go func() { r, e := s.Active(callCtx); result <- response{r, e} }()
	var challenge *model.BrowserChallenge
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		raw, e := s.Handle(ctx, base("poll"), time.Now())
		if e != nil {
			t.Fatal(e)
		}
		var r Reply
		json.Unmarshal(raw, &r)
		if len(r.Commands) > 0 {
			t.Fatal("focus read dispatched an action")
		}
		if r.Challenge != nil {
			challenge = r.Challenge
			break
		}
		time.Sleep(time.Millisecond * 5)
	}
	if challenge == nil {
		t.Fatal("focus did not request readback")
	}
	m = base("readback")
	m.Sequence = 2
	m.ObservedAt = time.Now().UTC().Format(time.RFC3339Nano)
	m.Complete = &complete
	m.Stable = &complete
	m.FocusedWindow = &focused
	m.Tabs = []model.BrowserTab{tab}
	m.PresentTabs = []int{1}
	m.ChallengeID = challenge.ID
	if _, err = s.Handle(ctx, m, time.Now()); err != nil {
		t.Fatal(err)
	}
	r := <-result
	if r.err != nil || r.state.Status != "unbound" || r.state.Focus == nil || r.state.Focus.TabID != 1 {
		t.Fatal(r)
	}
	st, _ := db.State(ctx)
	if len(st.Actions) != 0 || len(st.BrowserOperations) != 0 {
		t.Fatal("focus read created execution authority")
	}
	if got := selectActive(st, nil); got.Status != "unknown" || got.Target != "" {
		t.Fatal("saved focus promoted without a lease", got)
	}
	restarted := NewService(db)
	if restarted.RecoveryFresh(st.Browsers[profile], 0, time.Now()) {
		t.Fatal("lease survived runtime restart")
	}
	before, _ := db.Events(ctx)
	if r, e := s.Active(ctx); e != nil || r.Status != "unbound" {
		t.Fatal(r, e)
	}
	after, _ := db.Events(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("fresh active read grew the event log")
	}
}

func TestActiveSelectionRequiresExactTabOwnership(t *testing.T) {
	st := model.Empty()
	profile, epoch, owner, manifest, desired := model.NewID(), model.NewID(), model.NewID(), model.NewID(), model.NewID()
	st.Tasks["alpha"] = model.TaskRecord{Task: model.Task{ID: "alpha", Status: "active"}, Revision: 1}
	st.WorkspaceHeads["alpha"] = manifest
	st.WorkspaceManifests[manifest] = model.WorkspaceManifest{ID: manifest, Target: "alpha", TaskRevision: 1, Surfaces: []model.DesiredSurface{{ID: desired, Kind: "browser"}}}
	st.Actions[owner] = model.ActionRecord{DeliveryID: model.NewID(), Intent: model.ActionIntent{ID: owner, Target: "alpha", TaskRevision: 1, ManifestID: manifest, SurfaceID: desired, Browser: &model.BrowserIntent{Action: "open", Profile: profile, Epoch: epoch}}}
	p := model.BrowserProfile{ID: profile, Epoch: epoch, Paired: true, VerificationProtocol: 1, FocusedWindow: 1, Tabs: []model.BrowserTab{{ID: 1, WindowID: 1, Active: true, URL: "https://example.test/", OwnerID: owner}}}
	st.Browsers[profile] = p
	fresh := map[string]bool{profile: true}
	if got := selectActive(st, fresh); got.Status != "active" || got.Target != "alpha" {
		t.Fatal(got)
	}
	p.Tabs[0].OwnerID = ""
	st.Browsers[profile] = p
	if got := selectActive(st, fresh); got.Status != "unbound" || got.Target != "" {
		t.Fatal("focus invented ownership", got)
	}
	p.Tabs[0].OwnerID = owner
	p.Tabs = append(p.Tabs, model.BrowserTab{ID: 2, WindowID: 1, Active: true, URL: "https://other.test/"})
	st.Browsers[profile] = p
	if got := selectActive(st, fresh); got.Status != "ambiguous" || got.Target != "" {
		t.Fatal(got)
	}
	p.Tabs = p.Tabs[:1]
	st.Browsers[profile] = p
	task := st.Tasks["alpha"]
	task.Revision++
	st.Tasks["alpha"] = task
	if got := selectActive(st, fresh); got.Status != "unbound" {
		t.Fatal("stale manifest bound focus", got)
	}
	p2 := model.Clone(p)
	p2.ID = model.NewID()
	st.Browsers[p2.ID] = p2
	fresh[p2.ID] = true
	if got := selectActive(st, fresh); got.Status != "ambiguous" || got.Focus != nil {
		t.Fatal("competing focus claims selected", got)
	}
}

func TestActiveSelectionReportsUnresolvedFocusedContent(t *testing.T) {
	st := model.Empty()
	profile, epoch := model.NewID(), model.NewID()
	st.Browsers[profile] = model.BrowserProfile{ID: profile, Epoch: epoch, Paired: true, VerificationProtocol: 1,
		FocusedWindow: 1, Tabs: []model.BrowserTab{{ID: 1, WindowID: 1, Active: true, URL: "https://example.test/?q=%zz"}}}
	got := selectActive(st, map[string]bool{profile: true})
	if got.Status != "unbound" || got.Focus == nil || got.Focus.SurfaceID != "" || !reflect.DeepEqual(got.Gaps, []ActiveGap{{Profile: profile, Reason: "focused_content_unresolved"}}) {
		t.Fatal(got)
	}
}
