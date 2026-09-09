//go:build linux

package continuity

import (
	"encoding/json"
	"errors"
	"heimdall/internal/model"
	"heimdall/internal/store"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func preservationGit(t *testing.T, args ...string) string {
	t.Helper()
	c := exec.Command("git", args...)
	c.Env = []string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_SYSTEM=" + os.DevNull}
	out, err := c.CombinedOutput()
	if err != nil {
		t.Fatal(string(out), err)
	}
	return strings.TrimSpace(string(out))
}
func preservationFixture(t *testing.T) (*fixture, string, string, PreservationRequest) {
	t.Helper()
	f, _, file, a := artifactFixture(t)
	v := recordArtifact(t, f, a)
	cp := f.checkpoint(f.contract(), "none")
	cp.Version = 2
	cp.Checkpoint.Artifacts = []model.ArtifactRef{{ArtifactID: v.Artifact.ID, VersionID: v.Record.ID}}
	f.send(cp)
	root := filepath.Join(t.TempDir(), ".private")
	preservationGit(t, "init", "-b", "main", root)
	os.WriteFile(filepath.Join(root, "README.md"), []byte("Synthetic private clone"), 0600)
	preservationGit(t, "-C", root, "add", "README.md")
	preservationGit(t, "-C", root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-m", "Initial")
	remote := filepath.Join(t.TempDir(), "remote.git")
	preservationGit(t, "init", "--bare", remote)
	preservationGit(t, "-C", root, "remote", "add", "origin", remote)
	r := PreservationRequest{Version: 1, ID: model.NewID(), Target: f.target, ExpectedTaskRevision: f.rev, Plan: &model.PreservationInput{CheckpointID: cp.ID, PrivateRoot: root, Remote: "origin", Ref: "refs/heads/main", Selections: []model.PreservationSelection{{ArtifactID: v.Artifact.ID, VersionID: v.Record.ID, MirrorPath: "project/ingested/local/notes.md"}}}}
	return f, file, remote, r
}
func requestPreservation(t *testing.T, f *fixture, r PreservationRequest) PreservationRequest {
	t.Helper()
	p, err := f.s.PreservationPreview(f.ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	r.PreviewDigest = p.Digest
	if _, err = f.s.Preservation(f.ctx, r, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	return r
}
func TestPreservationSeparateFactsRetryReplayExport(t *testing.T) {
	f, file, remote, r := preservationFixture(t)
	before, _ := f.e.Store.State(f.ctx)
	preview, err := f.s.PreservationPreview(f.ctx, r)
	if err != nil || preview.Snapshot.Stage != "not_preserved" || preview.Snapshot.Items[0].Source.Status != "matched" {
		t.Fatal(preview, err)
	}
	if after, _ := f.e.Store.State(f.ctx); !reflect.DeepEqual(before, after) {
		t.Fatal("preview mutated state")
	}
	r = requestPreservation(t, f, r)
	saved, _ := f.s.Preservation(f.ctx, r, "cli", f.now)
	root := r.Plan.PrivateRoot
	mirror := filepath.Join(root, r.Plan.Selections[0].MirrorPath)
	os.MkdirAll(filepath.Dir(mirror), 0700)
	content, _ := os.ReadFile(file)
	os.WriteFile(mirror, content, 0600)
	previous := "none"
	observe := func(outcome, note string, check bool) (PreservationRequest, model.PreservationReceipt) {
		t.Helper()
		o := PreservationRequest{Version: 1, ID: model.NewID(), Target: r.Target, ExpectedTaskRevision: f.rev, Observe: &PreservationObserveInput{PlanID: r.ID, Previous: previous, CheckRemote: check, ReportedOutcome: outcome, Note: note}}
		b, err := f.s.Preservation(f.ctx, o, "cli", f.now)
		if err != nil {
			t.Fatal(err)
		}
		var receipt model.PreservationReceipt
		json.Unmarshal(b, &receipt)
		previous = receipt.ID
		return o, receipt
	}
	_, mirrored := observe("not_reported", "", false)
	if mirrored.Snapshot.Stage != "mirrored" {
		t.Fatal(mirrored)
	}
	preservationGit(t, "-C", root, "add", r.Plan.Selections[0].MirrorPath)
	preservationGit(t, "-C", root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-m", "Preserve selected notes")
	_, committed := observe("push_failed", "Manual push was refused", true)
	if committed.Snapshot.Stage != "committed" || committed.Snapshot.RemoteStatus != "missing" || committed.ReportedOutcome != "push_failed" {
		t.Fatal(committed)
	}
	preservationGit(t, "-C", root, "push", "origin", "main")
	_, published := observe("succeeded", "Manual push completed", true)
	if published.Snapshot.Stage != "published" || published.Snapshot.Head != published.Snapshot.RemoteCommit || published.Snapshot.Items[0].Committed.Digest != published.Snapshot.Items[0].ExpectedDigest {
		t.Fatal(published)
	}
	// Publication is an observed historical fact, not evidence of current source.
	os.WriteFile(file, []byte("New draft after copying"), 0600)
	_, changed := observe("source_changed_during_copy", "Operator noticed edits while copying", true)
	if changed.Snapshot.Items[0].Source.Status != "content_changed" || changed.Snapshot.Stage != "published" {
		t.Fatal(changed)
	}
	os.Remove(file)
	_, missing := observe("missing_original", "Original subsequently removed", false)
	if missing.Snapshot.Items[0].Source.Status != "missing" || missing.Snapshot.Stage != "committed" {
		t.Fatal(missing)
	}
	os.Rename(remote, remote+"-offline")
	offlineArgs, offline := observe("offline", "Remote unavailable during manual push", true)
	if offline.Snapshot.Stage != "committed" || offline.Snapshot.RemoteStatus != "unavailable" || offline.ReportedOutcome != "offline" {
		t.Fatal(offline)
	}
	portable, err := f.s.PreservationExport(f.ctx, r.Target, r.Plan.CheckpointID)
	if err != nil || len(portable.Artifacts) != 1 {
		t.Fatal(portable, err)
	}
	export, _ := json.Marshal(portable)
	for _, secret := range []string{root, remote, "resource_id", "host", "private_root", "remote_fingerprint"} {
		if strings.Contains(string(export), secret) {
			t.Fatal("nonportable metadata exported", secret)
		}
	}
	st, _ := f.e.Store.State(f.ctx)
	if !reflect.DeepEqual(st.Checkpoints, before.Checkpoints) || !reflect.DeepEqual(st.Tasks, before.Tasks) {
		t.Fatal("preservation altered checkpoint or task")
	}
	os.RemoveAll(root)
	retry, err := f.s.Preservation(f.ctx, r, "cli", f.now)
	if err != nil || !reflect.DeepEqual(retry, saved) {
		t.Fatal("plan retry observed missing files", err)
	}
	retry, err = f.s.Preservation(f.ctx, offlineArgs, "cli", f.now)
	var again model.PreservationReceipt
	json.Unmarshal(retry, &again)
	if err != nil || !reflect.DeepEqual(again, offline) {
		t.Fatal("receipt retry reobserved", err)
	}
	conflict := offlineArgs
	copyInput := *conflict.Observe
	copyInput.Note = "Different request under same ID"
	conflict.Observe = &copyInput
	if _, err := f.s.Preservation(f.ctx, conflict, "cli", f.now); !errors.Is(err, store.ErrConflict) {
		t.Fatal("idempotency conflict accepted", err)
	}
	replayed, err := f.e.Store.Replay(f.ctx)
	if err != nil || !reflect.DeepEqual(replayed, st) {
		t.Fatal("replay depended on vanished originals/clone", err)
	}
}
func TestPreservationDirtyRefusedStaleAndScope(t *testing.T) {
	f, file, _, r := preservationFixture(t)
	p, err := f.s.PreservationPreview(f.ctx, r)
	if err != nil {
		t.Fatal(err)
	}
	r.PreviewDigest = p.Digest
	os.WriteFile(filepath.Join(r.Plan.PrivateRoot, "unrelated.txt"), []byte("unrelated private work"), 0600)
	if _, err = f.s.Preservation(f.ctx, r, "cli", f.now); !errors.Is(err, store.ErrConflict) {
		t.Fatal("changed preview accepted", err)
	}
	p, err = f.s.PreservationPreview(f.ctx, r)
	if err != nil || p.Snapshot.UnrelatedCount != 1 {
		t.Fatal(p, err)
	}
	preservationGit(t, "-C", r.Plan.PrivateRoot, "add", "unrelated.txt")
	p, err = f.s.PreservationPreview(f.ctx, r)
	if err != nil || p.Snapshot.UnrelatedCount != 1 {
		t.Fatal(p, err)
	}
	os.Mkdir(filepath.Join(r.Plan.PrivateRoot, ".git", "rebase-merge"), 0700)
	p, err = f.s.PreservationPreview(f.ctx, r)
	if err != nil || p.Snapshot.RepositoryStatus != "rebase_conflict" {
		t.Fatal(p, err)
	}
	r = requestPreservation(t, f, r)
	wrong := r
	wrong.ID = model.NewID()
	wrong.Target = "missing-target"
	if _, err = f.s.PreservationPreview(f.ctx, wrong); err == nil {
		t.Fatal("wrong target accepted")
	}
	if _, err = f.s.Preservation(f.ctx, r, "browser", f.now); err == nil {
		t.Fatal("browser mutation accepted")
	}
	if _, err = f.s.PreservationShow(f.ctx, "missing-target", r.ID); err == nil {
		t.Fatal("cross target read")
	}
	if _, err = f.s.PreservationExport(f.ctx, "missing-target", r.Plan.CheckpointID); err == nil {
		t.Fatal("cross target export")
	}
	st, _ := f.e.Store.State(f.ctx)
	resID := st.ArtifactVersions[r.Plan.Selections[0].VersionID].ResourceID
	unbind := f.request("resource.unbind")
	unbind.ResourceID = resID
	f.send(unbind)
	o := PreservationRequest{Version: 1, ID: model.NewID(), Target: r.Target, ExpectedTaskRevision: f.rev, Observe: &PreservationObserveInput{PlanID: r.ID, Previous: "none", ReportedOutcome: "not_reported"}}
	b, err := f.s.Preservation(f.ctx, o, "cli", f.now)
	if err != nil {
		t.Fatal(err)
	}
	var receipt model.PreservationReceipt
	json.Unmarshal(b, &receipt)
	if receipt.Snapshot.Items[0].Source.Status != "scope_changed" {
		t.Fatal(receipt)
	}
	// Refused names do not get read, and symlinks cannot bypass that boundary.
	f2, _, file2, a := artifactFixture(t)
	v := recordArtifact(t, f2, a)
	st2, _ := f2.e.Store.State(f2.ctx)
	res := st2.Resources[v.Record.ResourceID]
	secret := filepath.Join(res.Root, ".env")
	os.WriteFile(secret, []byte("synthetic-secret"), 0600)
	if x := preservationRead(f2.ctx, res.Root, ".env", v.Record.Observation); x.Status != "refused" || x.Digest != "" {
		t.Fatal(x)
	}
	os.Symlink(file2, filepath.Join(res.Root, "link.md"))
	if x := preservationRead(f2.ctx, res.Root, "link.md", v.Record.Observation); x.Status != "refused" {
		t.Fatal(x)
	}
	_ = file
}
