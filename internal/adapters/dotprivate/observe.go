// Package dotprivate inspects explicitly selected mirrors. It never invokes
// dotprivate, runs hooks/filters, changes Git state or infers a push from an exit code.
package dotprivate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"heimdall/internal/model"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

type ReadFile func(context.Context, string, string) model.PreservedFile

var oidRE = regexp.MustCompile(`^([a-f0-9]{40}|[a-f0-9]{64})$`)
var refusedRE = regexp.MustCompile(`(?i)(^|/)\.env(\..*)?$|\.(pem|key|p12|pfx|jks|keystore|tfstate|db|db-wal|db-shm)$|(^|/)id_(rsa|ed25519|ecdsa|dsa)(\.pub)?$|credential|secret|(^|/)(endpoint|browser-endpoint|client-endpoint|host-config)\.json$|(^|/)\.(netrc|npmrc|pypirc)$|kubeconfig|(^|/)\.aws/`)

func Refused(path string) bool { return refusedRE.MatchString(path) }

type bounded struct {
	bytes.Buffer
	limit int
}

func (b *bounded) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		return 0, fmt.Errorf("observation output limit")
	}
	return b.Buffer.Write(p)
}
func run(ctx context.Context, root string, limit int, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	prefix := []string{"--no-optional-locks", "--literal-pathspecs", "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false", "-c", "core.hooksPath=" + os.DevNull, "-C", root}
	c := exec.CommandContext(ctx, "git", append(prefix, args...)...)
	c.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + os.Getenv("HOME"), "LANG=C", "LC_ALL=C", "GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_SYSTEM=" + os.DevNull, "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0", "GIT_NO_REPLACE_OBJECTS=1", "GIT_ALLOW_PROTOCOL=file:https:ssh", "GIT_SSH_COMMAND=ssh -oBatchMode=yes -oConnectTimeout=5", "SSH_AUTH_SOCK=" + os.Getenv("SSH_AUTH_SOCK")}
	b := &bounded{limit: limit}
	c.Stdout = b
	c.Stderr = io.Discard
	c.WaitDelay = time.Second
	err := c.Run()
	return b.String(), err
}

type repository struct {
	head, ref, working, url, fingerprint, status string
	dirty                                        []string
}

func inspect(ctx context.Context, in model.PreservationInput) (repository, error) {
	r := repository{status: "available"}
	canonical, err := filepath.EvalSymlinks(in.PrivateRoot)
	if err != nil || canonical != in.PrivateRoot {
		return r, fmt.Errorf("private root unavailable or symlink")
	}
	dot, err := os.Lstat(filepath.Join(in.PrivateRoot, ".git"))
	if err != nil || !dot.IsDir() || dot.Mode()&os.ModeSymlink != 0 {
		return r, fmt.Errorf("explicit standalone private clone required")
	}
	top, err := run(ctx, in.PrivateRoot, 4096, "rev-parse", "--show-toplevel")
	if err != nil || strings.TrimSpace(top) != in.PrivateRoot {
		return r, fmt.Errorf("private clone identity mismatch")
	}
	r.head, _ = run(ctx, in.PrivateRoot, 256, "rev-parse", "--verify", "HEAD")
	r.head = strings.TrimSpace(r.head)
	if !oidRE.MatchString(r.head) {
		r.head = ""
		return r, fmt.Errorf("private clone requires an initial commit")
	}
	r.ref, _ = run(ctx, in.PrivateRoot, 1024, "symbolic-ref", "--quiet", "HEAD")
	r.ref = strings.TrimSpace(r.ref)
	for _, name := range []string{"rebase-merge", "rebase-apply", "MERGE_HEAD"} {
		if _, err := os.Lstat(filepath.Join(in.PrivateRoot, ".git", name)); err == nil {
			r.status = "rebase_conflict"
		}
	}
	dirty := map[string]bool{}
	for _, args := range [][]string{{"diff-index", "--cached", "--no-ext-diff", "--no-textconv", "--name-only", "-z", "HEAD", "--"}, {"ls-files", "--modified", "--deleted", "--others", "--exclude-standard", "-z"}, {"ls-files", "--unmerged", "-z"}} {
		raw, err := run(ctx, in.PrivateRoot, 256<<10, args...)
		if err != nil {
			return r, fmt.Errorf("private status unavailable")
		}
		for _, p := range strings.Split(raw, "\x00") {
			if p == "" {
				continue
			}
			if args[1] == "--unmerged" {
				_, p, _ = strings.Cut(p, "\t")
				r.status = "rebase_conflict"
			}
			dirty[p] = true
		}
	}
	for p := range dirty {
		r.dirty = append(r.dirty, p)
	}
	sort.Strings(r.dirty)
	// Include index identity; stat-only modified paths are conservative refusals.
	index, err := run(ctx, in.PrivateRoot, 1<<20, "ls-files", "--stage", "-z")
	if err != nil {
		return r, fmt.Errorf("private index unavailable")
	}
	r.working = model.PreservationDigest([]any{r.head, r.ref, r.dirty, model.PreservationDigest(index)})
	remote, _ := run(ctx, in.PrivateRoot, 16384, "config", "--get-all", "remote."+in.Remote+".pushurl")
	if strings.TrimSpace(remote) == "" {
		remote, _ = run(ctx, in.PrivateRoot, 16384, "config", "--get-all", "remote."+in.Remote+".url")
	}
	urls := strings.Split(strings.TrimSpace(remote), "\n")
	if len(urls) == 1 && urls[0] != "" {
		r.url = urls[0]
		r.fingerprint = model.PreservationDigest([]string{in.Remote, in.Ref, r.url})
	}
	return r, nil
}
func remoteURL(s string) bool {
	if s == "" || strings.ContainsAny(s, "\x00\r\n\t") {
		return false
	}
	if filepath.IsAbs(s) {
		return true
	}
	u, err := url.Parse(s)
	if err == nil && (u.Scheme == "https" || u.Scheme == "ssh" || u.Scheme == "file") {
		return u.RawQuery == "" && u.Fragment == ""
	}
	return regexp.MustCompile(`^[A-Za-z0-9_.-]+@[A-Za-z0-9.-]+:[A-Za-z0-9_./~-]+$`).MatchString(s)
}
func remote(ctx context.Context, url, ref string) (string, string) {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if !remoteURL(url) {
		return "unavailable", ""
	}
	// Query outside the clone: local uploadpack/SSH/helpers/URL rewrites cannot
	// turn a read-only observation into a repository-configured command.
	dir, err := os.MkdirTemp("", "heimdall-preservation-remote-")
	if err != nil {
		return "unavailable", ""
	}
	defer os.RemoveAll(dir)
	raw, err := run(ctx, dir, 16384, "ls-remote", "--quiet", "--exit-code", "--refs", url, ref)
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok && e.ExitCode() == 2 {
			return "missing", ""
		}
		return "unavailable", ""
	}
	fields := strings.Fields(raw)
	if len(fields) != 2 || fields[1] != ref || !oidRE.MatchString(fields[0]) {
		return "unavailable", ""
	}
	return "observed", fields[0]
}
func committed(ctx context.Context, root, head, path, expected string) model.PreservedFile {
	f := model.PreservedFile{Status: "unavailable"}
	if Refused(path) {
		f.Status = "refused"
		return f
	}
	raw, err := run(ctx, root, 8192, "ls-tree", "-z", head, "--", path)
	if err != nil {
		return f
	}
	if raw == "" {
		f.Status = "missing"
		return f
	}
	meta, name, ok := strings.Cut(strings.TrimSuffix(raw, "\x00"), "\t")
	parts := strings.Fields(meta)
	if !ok || name != path || len(parts) != 3 || parts[1] != "blob" || !model.Contains([]string{"100644", "100755"}, parts[0]) || !oidRE.MatchString(parts[2]) {
		f.Status = "refused"
		return f
	}
	data, err := run(ctx, root, 64<<20, "cat-file", "blob", parts[2])
	if err != nil {
		return f
	}
	h := sha256.Sum256([]byte(data))
	f.Digest = hex.EncodeToString(h[:])
	f.Bytes = int64(len(data))
	f.Status = "content_changed"
	if f.Digest == expected {
		f.Status = "matched"
	}
	return f
}

// Observe reads each selected file and bounds the result with repository state.
// No checkout lock claims protection against dotprivate or other Git writers.
func Observe(ctx context.Context, in model.PreservationInput, items []model.PreservationItem, read ReadFile, checkRemote bool, pinnedRemote string) model.PreservationSnapshot {
	s := model.PreservationSnapshot{Items: items, RepositoryStatus: "unavailable", RemoteStatus: "not_checked"}
	r, err := inspect(ctx, in)
	for i, sel := range in.Selections {
		s.Items[i].Mirror = read(ctx, in.PrivateRoot, sel.MirrorPath)
		s.Items[i].Committed = model.PreservedFile{Status: "unavailable"}
	}
	if err != nil {
		s.Stage = s.DeriveStage()
		return s
	}
	s.RepositoryStatus = r.status
	s.Head = r.head
	s.Ref = r.ref
	s.WorkingDigest = r.working
	s.RemoteFingerprint = r.fingerprint
	s.DirtyCount = len(r.dirty)
	allowed := map[string]bool{}
	for _, sel := range in.Selections {
		allowed[sel.MirrorPath] = true
	}
	for _, p := range r.dirty {
		if !allowed[p] {
			s.UnrelatedCount++
		}
	}
	for i, sel := range in.Selections {
		s.Items[i].Committed = committed(ctx, in.PrivateRoot, r.head, sel.MirrorPath, s.Items[i].ExpectedDigest)
	}
	if checkRemote {
		if pinnedRemote == "" || pinnedRemote != r.fingerprint {
			s.RemoteStatus = "config_changed"
		} else {
			s.RemoteStatus, s.RemoteCommit = remote(ctx, r.url, in.Ref)
			if s.RemoteStatus == "observed" {
				s.RemoteStatus = "different"
				if s.RemoteCommit == r.head {
					s.RemoteStatus = "matched"
				}
			}
		}
	}
	after, err := inspect(ctx, in)
	if err != nil || r.working != after.working || r.fingerprint != after.fingerprint || r.status != after.status {
		s.RepositoryStatus = "changed_during_observation"
	}
	s.Stage = s.DeriveStage()
	return s
}
