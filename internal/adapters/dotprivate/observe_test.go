//go:build linux

package dotprivate

import (
	"context"
	"heimdall/internal/model"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoteProtocolsAndRefusal(t *testing.T) {
	for _, p := range []string{".env", "plan/.env.local", "private.key", "client-endpoint.json", "data/heimdall.db", "data/heimdall.db-wal", "x.credential.json"} {
		if !Refused(p) {
			t.Fatal("credential/database selector accepted", p)
		}
	}
	for _, s := range []string{"ext::sh -c touch /tmp/marker", "-oProxyCommand=bad", "http://host/repo", "https://host/repo?token=secret", "git@host:repo\nextra"} {
		if remoteURL(s) {
			t.Fatal("unsafe remote", s)
		}
	}
	for _, s := range []string{"https://host/repo.git", "ssh://git@host/repo", "git@host:repo.git", "/tmp/remote.git"} {
		if !remoteURL(s) {
			t.Fatal("supported remote", s)
		}
	}
}
func TestGitReadbackDoesNotRunConfiguredHelpers(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "private")
	marker := filepath.Join(dir, "helper-ran")
	git := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Env = []string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_SYSTEM=" + os.DevNull}
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatal(string(out), err)
		}
	}
	git("init", "-b", "main", root)
	os.WriteFile(filepath.Join(root, "notes.md"), []byte("notes"), 0600)
	git("-C", root, "add", "notes.md")
	git("-C", root, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "-m", "initial")
	helper := "!echo ran > " + marker
	for _, k := range []string{"core.fsmonitor", "diff.external", "filter.evil.clean", "filter.evil.smudge", "core.sshCommand", "remote.origin.uploadpack"} {
		git("-C", root, "config", k, helper)
	}
	git("-C", root, "config", "remote.origin.url", "ext::"+helper)
	os.WriteFile(filepath.Join(root, ".gitattributes"), []byte("*.md filter=evil diff=evil\n"), 0600)
	os.WriteFile(filepath.Join(root, "notes.md"), []byte("changed"), 0600)
	in := model.PreservationInput{PrivateRoot: root, Remote: "origin", Ref: "refs/heads/main", Selections: []model.PreservationSelection{{MirrorPath: "notes.md"}}}
	r, err := inspect(context.Background(), in)
	if err != nil {
		t.Fatal(err)
	}
	read := func(context.Context, string, string) model.PreservedFile {
		return model.PreservedFile{Status: "missing"}
	}
	items := []model.PreservationItem{{ExpectedDigest: strings.Repeat("a", 64)}}
	s := Observe(context.Background(), in, items, read, true, r.fingerprint)
	if s.RemoteStatus != "unavailable" {
		t.Fatal(s)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatal("repository helper executed")
	}
}
func TestObservationDetectsConcurrentRepositoryChange(t *testing.T) {
	dir := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		c := exec.Command("git", args...)
		c.Env = []string{"PATH=" + os.Getenv("PATH"), "GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_SYSTEM=" + os.DevNull}
		if out, err := c.CombinedOutput(); err != nil {
			t.Fatal(string(out), err)
		}
	}
	git("init", "-b", "main", dir)
	commit := func() {
		git("-C", dir, "-c", "user.name=Fixture", "-c", "user.email=fixture@example.invalid", "commit", "--allow-empty", "-m", "Concurrent change")
	}
	commit()
	in := model.PreservationInput{PrivateRoot: dir, Remote: "origin", Ref: "refs/heads/main", Selections: []model.PreservationSelection{{MirrorPath: "notes.md"}}}
	read := func(context.Context, string, string) model.PreservedFile {
		commit()
		return model.PreservedFile{Status: "missing"}
	}
	s := Observe(context.Background(), in, []model.PreservationItem{{}}, read, false, "")
	if s.RepositoryStatus != "changed_during_observation" || s.Stage != "uncertain" {
		t.Fatal(s)
	}
}
