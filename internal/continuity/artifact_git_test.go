//go:build linux

package continuity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"heimdall/internal/model"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"golang.org/x/sys/unix"
)

func TestArtifactGitRawInputAndWorktreeIdentity(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	linked := filepath.Join(root, "linked")
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_SYSTEM=" + os.DevNull}
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatal(string(out), err)
		}
	}
	git("init", "-b", "main", repo)
	os.Mkdir(filepath.Join(repo, "plans"), 0700)
	file := filepath.Join(repo, "plans", "notes.md")
	os.WriteFile(file, []byte("first"), 0600)
	r := model.Resource{Active: true, Kind: "tree", Root: repo, Path: "."}
	observe := func() model.ArtifactObservation {
		t.Helper()
		v, err := ObserveArtifact(context.Background(), r, "plans/notes.md", true)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	unborn := observe()
	if unborn.Git.Commit != "" || unborn.Git.Ref != "refs/heads/main" || !unborn.Git.Dirty {
		t.Fatal(unborn.Git)
	}
	git("-C", repo, "add", "plans/notes.md")
	git("-C", repo, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-m", "Initial")
	clean := observe()
	if clean.Git.Dirty || clean.Git.IndexOID != clean.Git.HeadOID {
		t.Fatal(clean.Git)
	}
	git("-C", repo, "worktree", "add", "-b", "linked", linked)
	linkedResource := r
	linkedResource.Root = linked
	live, err := ObserveArtifact(context.Background(), linkedResource, "plans/notes.md", true)
	if err != nil || live.Git.Repository != filepath.Join(repo, ".git") || live.Git.Worktree != linked || live.Git.Ref != "refs/heads/linked" {
		t.Fatal(live, err)
	}
	os.WriteFile(file, []byte("second"), 0600)
	changed := observe()
	if !changed.Git.Dirty || changed.Git.InputDigest == clean.Git.InputDigest {
		t.Fatal(changed.Git)
	}
	h := sha256.Sum256([]byte("second"))
	if changed.Digest != hex.EncodeToString(h[:]) {
		t.Fatal("digest is not exact content SHA-256")
	}
	git("-C", repo, "add", "plans/notes.md")
	staged := observe()
	if !staged.Git.Dirty || staged.Git.IndexOID == staged.Git.HeadOID {
		t.Fatal("staged change lost", staged.Git)
	}
	// Metadata observation must not invoke a repository's clean filter or fsmonitor.
	sentinel := filepath.Join(root, "UNEXPECTED_HELPER")
	git("-C", repo, "config", "filter.trap.clean", "touch "+sentinel)
	git("-C", repo, "config", "core.fsmonitor", "touch "+sentinel)
	os.WriteFile(filepath.Join(repo, ".gitattributes"), []byte("*.md filter=trap\n"), 0600)
	os.WriteFile(file, []byte("third"), 0600)
	observe()
	if _, err = os.Stat(sentinel); !os.IsNotExist(err) {
		t.Fatal("Git observation executed configured helper")
	}
}

func TestArtifactObservationRefusesExcludedSymlinkAndNonregularFiles(t *testing.T) {
	root := t.TempDir()
	os.WriteFile(filepath.Join(root, "file"), []byte("notes"), 0600)
	os.Mkdir(filepath.Join(root, "private"), 0700)
	os.WriteFile(filepath.Join(root, "private", "secret"), []byte("excluded"), 0600)
	r := model.Resource{Active: true, Kind: "tree", Root: root, Path: ".", Exclude: []string{"private"}}
	os.Symlink(filepath.Join(root, "file"), filepath.Join(root, "alias"))
	os.Symlink(filepath.Join(root, "private"), filepath.Join(root, "linked"))
	unix.Mkfifo(filepath.Join(root, "fifo"), 0600)
	for _, path := range []string{"../file", "private/secret", ".git/config", "alias", "linked/secret", "fifo", "."} {
		if _, err := ObserveArtifact(context.Background(), r, path, false); err == nil {
			t.Fatal("accepted", path)
		}
	}
	f, err := os.Create(filepath.Join(root, "large"))
	if err != nil {
		t.Fatal(err)
	}
	f.Truncate(MaxResourceBytes + 1)
	f.Close()
	if _, err = ObserveArtifact(context.Background(), r, "large", false); err == nil {
		t.Fatal("unbounded artifact read")
	}
}
