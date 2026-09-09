package continuity

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type artifactGitBuffer struct{ bytes.Buffer }

func (b *artifactGitBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > 16384 {
		return 0, fmt.Errorf("Git metadata exceeds 16 KiB")
	}
	return b.Buffer.Write(p)
}

func observeArtifactGit(ctx context.Context, file string, input model.ArtifactObservation, blob1, blob256 string) (*model.ArtifactGit, error) {
	cwd := filepath.Dir(file)
	run := func(args ...string) (string, error) {
		call, cancel := context.WithTimeout(ctx, time.Second)
		defer cancel()
		prefix := []string{"--no-optional-locks", "--literal-pathspecs", "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false", "-C", cwd}
		cmd := exec.CommandContext(call, "git", append(prefix, args...)...)
		cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "LANG=C", "LC_ALL=C", "GIT_CONFIG_GLOBAL=" + os.DevNull, "GIT_CONFIG_SYSTEM=" + os.DevNull, "GIT_TERMINAL_PROMPT=0", "GIT_OPTIONAL_LOCKS=0"}
		var out artifactGitBuffer
		cmd.Stdout = &out
		cmd.Stderr = io.Discard
		cmd.WaitDelay = time.Second
		err := cmd.Run()
		return out.String(), err
	}
	paths, err := run("rev-parse", "--path-format=absolute", "--show-toplevel", "--git-common-dir")
	if err != nil {
		return nil, fmt.Errorf("requested Git identity unavailable")
	}
	lines := strings.Split(strings.TrimSuffix(paths, "\n"), "\n")
	if len(lines) != 2 {
		return nil, fmt.Errorf("invalid Git path response")
	}
	g := &model.ArtifactGit{}
	g.Worktree, err = filepath.EvalSymlinks(lines[0])
	if err != nil {
		return nil, err
	}
	g.Repository, err = filepath.EvalSymlinks(lines[1])
	if err != nil {
		return nil, err
	}
	cwd = g.Worktree
	rel, err := filepath.Rel(g.Worktree, file)
	if err != nil || !filepath.IsLocal(rel) {
		return nil, fmt.Errorf("artifact outside observed worktree")
	}
	rel = filepath.ToSlash(rel)
	ref, refErr := run("symbolic-ref", "--quiet", "HEAD")
	if refErr == nil {
		g.Ref = strings.TrimSuffix(ref, "\n")
	} else {
		if e, ok := refErr.(*exec.ExitError); !ok || e.ExitCode() != 1 {
			return nil, fmt.Errorf("Git reference unavailable")
		}
	}
	commit, headErr := run("rev-parse", "--verify", "HEAD")
	if headErr == nil {
		g.Commit = strings.TrimSuffix(commit, "\n")
	} else {
		if g.Ref == "" {
			return nil, fmt.Errorf("Git HEAD unavailable")
		}
		_, err = run("show-ref", "--verify", "--quiet", g.Ref)
		if e, ok := err.(*exec.ExitError); !ok || e.ExitCode() != 1 {
			return nil, fmt.Errorf("Git HEAD is not a confirmed unborn branch")
		}
	}
	index, err := run("ls-files", "--stage", "--full-name", "-z", "--", rel)
	if err != nil {
		return nil, fmt.Errorf("Git index unavailable")
	}
	parse := func(raw string, index bool) (string, string, error) {
		if raw == "" {
			return "", "", nil
		}
		items := strings.Split(strings.TrimSuffix(raw, "\x00"), "\x00")
		if len(items) != 1 {
			return "", "", fmt.Errorf("ambiguous/unmerged Git input")
		}
		meta, name, ok := strings.Cut(items[0], "\t")
		fields := strings.Fields(meta)
		if !ok || name != rel || len(fields) != 3 {
			return "", "", fmt.Errorf("invalid Git input response")
		}
		if index {
			if fields[2] != "0" {
				return "", "", fmt.Errorf("unmerged Git input")
			}
			return fields[1], fields[0], nil
		}
		if fields[1] != "blob" {
			return "", "", fmt.Errorf("Git input is not a blob")
		}
		return fields[2], fields[0], nil
	}
	g.IndexOID, g.IndexMode, err = parse(index, true)
	if err != nil {
		return nil, err
	}
	if g.Commit != "" {
		tree, e := run("ls-tree", "--full-tree", "-z", g.Commit, "--", rel)
		if e != nil {
			return nil, fmt.Errorf("Git committed input unavailable")
		}
		g.HeadOID, g.HeadMode, err = parse(tree, false)
		if err != nil {
			return nil, err
		}
	}
	mode := "100644"
	if input.Mode&0111 != 0 {
		mode = "100755"
	}
	blob := blob1
	if len(g.IndexOID) == 64 {
		blob = blob256
	}
	g.Dirty = g.IndexOID == "" || g.IndexOID != blob || g.IndexMode != mode || g.IndexOID != g.HeadOID || g.IndexMode != g.HeadMode
	// Raw bytes and index/HEAD entries identify only this selected input. Avoid
	// status/diff/filter commands: repository-configured helpers must not execute.
	raw, _ := json.Marshal([]any{input.Digest, input.Bytes, input.Mode, g.IndexOID, g.IndexMode, g.HeadOID, g.HeadMode})
	digest := sha256.Sum256(raw)
	g.InputDigest = hex.EncodeToString(digest[:])
	return g, nil
}
