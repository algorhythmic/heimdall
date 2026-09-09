//go:build linux

package continuity

import (
	"encoding/json"
	"errors"
	"heimdall/internal/authz"
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

func artifactFixture(t *testing.T) (*fixture, string, string, ArtifactRequest) {
	t.Helper()
	f := setup(t)
	root := t.TempDir()
	file := filepath.Join(root, "notes.md")
	if err := os.WriteFile(file, []byte("Planning without a commit"), 0600); err != nil {
		t.Fatal(err)
	}
	r := f.request("resource.bind")
	r.Resource = &ResourceInput{Kind: "tree", Root: root, Path: ".", Exclude: []string{"private"}}
	f.send(r)
	a := ArtifactRequest{Version: 1, ID: model.NewID(), Target: f.target, ExpectedTaskRevision: f.rev, Artifact: ArtifactInput{ArtifactID: "new", Previous: "none", Name: "Design notes", Environment: "local", ResourceID: r.ID, Path: "notes.md"}}
	return f, root, file, a
}
func recordArtifact(t *testing.T, f *fixture, r ArtifactRequest) ArtifactView {
	t.Helper()
	raw, err := f.s.RecordArtifact(f.ctx, r, "cli", f.now)
	if err != nil {
		t.Fatal(err)
	}
	var v ArtifactView
	if err = json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}
func TestArtifactsCheckpointMoveRetryAndReplay(t *testing.T) {
	f, root, file, r := artifactFixture(t)
	v := recordArtifact(t, f, r)
	contract := f.contract()
	check := func(version, want string) {
		t.Helper()
		c, err := f.s.CheckArtifact(f.ctx, f.target, v.Artifact.ID, version)
		if err != nil || c.Status != want {
			t.Fatal(c.Status, want, err)
		}
	}
	check("", "matched")
	cp := f.checkpoint(contract, "none")
	cp.Version = 2
	cp.Checkpoint.Artifacts = []model.ArtifactRef{{ArtifactID: v.Artifact.ID, VersionID: v.Record.ID}}
	raw := f.send(cp)
	var saved model.Checkpoint
	json.Unmarshal(raw, &saved)
	if saved.Version != 3 || len(saved.Artifacts) != 1 {
		t.Fatal(saved)
	}
	before, _ := f.e.Store.State(f.ctx)
	context, err := f.s.Context(f.ctx, f.target, 16000)
	if err != nil || len(context.Artifacts) != 1 || context.Artifacts[0].Status != "matched" {
		t.Fatal(context, err)
	}
	after, _ := f.e.Store.State(f.ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("artifact reads wrote state")
	}
	_, err = f.s.Context(f.ctx, f.target, context.EstimatedTokens-1)
	var budget *BudgetError
	if !errors.As(err, &budget) {
		t.Fatal("artifact context omitted from budget", err)
	}
	os.WriteFile(file, []byte("Changed design notes"), 0600)
	check("", "content_changed")
	stale := cp
	stale.ID = model.NewID()
	copyCP := *cp.Checkpoint
	copyCP.Previous = cp.ID
	stale.Checkpoint = &copyCP
	if _, err = f.s.Execute(f.ctx, stale, "cli", f.now); !errors.Is(err, store.ErrConflict) {
		t.Fatal("checkpoint pinned changed bytes", err)
	}
	os.Rename(file, filepath.Join(root, "moved.md"))
	check("", "missing")
	moved := r
	moved.ID = model.NewID()
	moved.Artifact.ArtifactID = v.Artifact.ID
	moved.Artifact.Previous = v.Record.ID
	moved.Artifact.Name = ""
	moved.Artifact.Path = "moved.md"
	newVersion := recordArtifact(t, f, moved)
	if newVersion.Artifact.ID != v.Artifact.ID || newVersion.Record.ID == v.Record.ID {
		t.Fatal("logical identity lost")
	}
	check(v.Record.ID, "relocated")
	check("", "matched")
	context, err = f.s.Context(f.ctx, f.target, 16000)
	if err != nil || context.Artifacts[0].Status != "relocated" {
		t.Fatal(context, err)
	}
	state, _ := f.e.Store.State(f.ctx)
	if _, status := RecordedNextAction(state, f.target); status != "checkpoint_needs_review" {
		t.Fatal(status)
	}
	os.RemoveAll(root)
	if retry := recordArtifact(t, f, moved); !reflect.DeepEqual(retry, newVersion) {
		t.Fatal("retry reobserved missing file")
	}
	if retry := f.send(cp); !reflect.DeepEqual(retry, raw) {
		t.Fatal("checkpoint retry lost exact references")
	}
	state, _ = f.e.Store.State(f.ctx)
	replay, err := f.e.Replay(f.ctx)
	if err != nil || !reflect.DeepEqual(state, replay) {
		t.Fatal("replay accessed originals or changed records", err)
	}
	if replay.Tasks[f.target].Task.Status != "active" {
		t.Fatal("artifact observation completed task")
	}
}

func TestArtifactScopeEnvironmentAndCompetingHeads(t *testing.T) {
	f, root, _, r := artifactFixture(t)
	v := recordArtifact(t, f, r)
	other := r
	other.ID = model.NewID()
	other.Artifact.Environment = "research"
	otherV := recordArtifact(t, f, other)
	if v.Artifact.ID == otherV.Artifact.ID || v.Record.Observation.Digest != otherV.Record.Observation.Digest {
		t.Fatal("environment identity collapsed")
	}
	_, err := f.e.Execute(f.ctx, core.Command{ID: model.NewID(), Op: "add", Task: &model.Task{ID: "other-project", Title: "Other", Type: "project", Status: "active"}}, "cli", f.now)
	if err != nil {
		t.Fatal(err)
	}
	bad := r
	bad.ID = model.NewID()
	bad.Target = "other-project"
	if _, err = f.s.RecordArtifact(f.ctx, bad, "cli", f.now); err == nil {
		t.Fatal("cross-target resource adopted")
	}
	resource := f.request("resource.bind")
	resource.Target = bad.Target
	resource.Resource = &ResourceInput{Kind: "tree", Root: root, Path: "."}
	f.send(resource)
	bad.Artifact.ResourceID = resource.ID
	separate := recordArtifact(t, f, bad)
	if separate.Artifact.ID == v.Artifact.ID {
		t.Fatal("same-path task identity collapsed")
	}
	if _, err = f.s.ArtifactView(f.ctx, bad.Target, v.Artifact.ID, ""); err == nil {
		t.Fatal("cross-target lookup")
	}
	next := r
	next.Artifact = ArtifactInput{ArtifactID: v.Artifact.ID, Previous: v.Record.ID, ResourceID: r.Artifact.ResourceID, Path: "notes.md", Environment: "local"}
	bad = next
	bad.ID = model.NewID()
	bad.Artifact.Environment = "research"
	if _, err = f.s.RecordArtifact(f.ctx, bad, "cli", f.now); err == nil {
		t.Fatal("existing artifact moved environments")
	}
	results := make(chan error, 4)
	var group sync.WaitGroup
	for i := 0; i < 4; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			request := next
			request.ID = model.NewID()
			_, e := f.s.RecordArtifact(f.ctx, request, "cli", f.now)
			results <- e
		}()
	}
	group.Wait()
	close(results)
	winners := 0
	for e := range results {
		if e == nil {
			winners++
		} else if !errors.Is(e, store.ErrConflict) {
			t.Fatal(e)
		}
	}
	if winners != 1 {
		t.Fatal("competing heads", winners)
	}
}

func TestArtifactCheckpointKeepsScopedAuthority(t *testing.T) {
	f, _, _, r := artifactFixture(t)
	v := recordArtifact(t, f, r)
	contract := f.contract()
	token := strings.Repeat("c", 64)
	grant := model.NewID()
	_, err := (authz.Service{Store: f.e.Store}).Execute(f.ctx, authz.Request{Version: 1, ID: grant, Op: "grant.issue", Grant: &authz.IssueInput{Name: "Existing writer", Target: f.target, ResourceIDs: []string{r.Artifact.ResourceID}, TokenHash: authz.HashToken(token), CheckpointWrite: true, ExpiresAt: f.now.Add(time.Hour)}}, f.now)
	if err != nil {
		t.Fatal(err)
	}
	cp := f.checkpoint(contract, "none")
	cp.Version = 2
	cp.Checkpoint.Artifacts = []model.ArtifactRef{{ArtifactID: v.Artifact.ID, VersionID: v.Record.ID}}
	if _, err = f.s.ExecuteClient(f.ctx, cp, token, func() time.Time { return f.now }); !errors.Is(err, authz.ErrDenied) {
		t.Fatal("old writer gained artifact authority", err)
	}
	if _, err = f.s.RecordArtifact(f.ctx, r, "client:"+grant, f.now); err == nil {
		t.Fatal("client recorded artifact")
	}
	f.send(cp)
	st, _ := f.e.Store.State(f.ctx)
	bundle, err := ScopedContext(f.ctx, st, st.Grants[grant], f.target, 16000)
	if err != nil || len(bundle.Artifacts) != 0 || len(bundle.Checkpoint.Artifacts) != 1 || !hasIssue(bundle, "artifact_checks_require_cli") {
		t.Fatal("scope gained artifact observations or lost references", err, bundle)
	}
	legacy := cp
	legacy.Version = 1
	legacy.ID = model.NewID()
	if _, err = f.s.Execute(f.ctx, legacy, "cli", f.now); err == nil {
		t.Fatal("legacy request smuggled references")
	}
}
