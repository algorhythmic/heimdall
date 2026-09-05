package checks

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"heimdall/internal/continuity"
	"heimdall/internal/model"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"time"
)

const MaxOutput = 1 << 20

type output struct {
	mu      sync.Mutex
	hash    hash.Hash
	count   int64
	limited bool
	cancel  context.CancelFunc
}

func (o *output) Write(b []byte) (int, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	if int64(len(b)) > MaxOutput-o.count {
		o.limited = true
		o.cancel()
		return 0, fmt.Errorf("output limit exceeded")
	}
	o.count += int64(len(b))
	o.hash.Write(b)
	return len(b), nil
}
func minimalEnv() []string {
	result := []string{}
	// No inherited API tokens, user Git configuration variables or shell startup
	// variables. PATH permits build tools to locate their ordinary subprocesses.
	for _, key := range []string{"PATH", "SystemRoot", "WINDIR", "TEMP", "TMP", "TMPDIR"} {
		if value := os.Getenv(key); value != "" {
			result = append(result, key+"="+value)
		}
	}
	return result
}
func snapshots(ctx context.Context, st model.State, d model.Evaluator) ([]model.ResourceVersion, error) {
	refs := []model.ResourceVersion{}
	for _, id := range st.Contracts[d.ContractID].ResourceIDs {
		r := st.Resources[id]
		v, err := continuity.Observe(ctx, r)
		if err != nil {
			return refs, fmt.Errorf("resource %s unavailable or changed", id)
		}
		refs = append(refs, model.ResourceVersion{ID: id, Snapshot: v})
	}
	return refs, nil
}
func executableDigest(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() || info.Size() > 128<<20 {
		return "", fmt.Errorf("unsupported executable")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, 128<<20+1))
	if err != nil || n != info.Size() {
		return "", fmt.Errorf("executable changed or unreadable")
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
func repoIdentity(ctx context.Context, root string) (*model.RepoIdentity, error) {
	// Exact root check prevents a directory within another repository from
	// borrowing its parent's commit identity.
	run := func(args ...string) (string, error) {
		command := exec.CommandContext(ctx, "git", append([]string{"-c", "safe.directory=" + filepath.ToSlash(root), "-c", "core.fsmonitor=false", "-c", "core.untrackedCache=false", "-C", root}, args...)...)
		command.Env = append(minimalEnv(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL="+os.DevNull, "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0")
		var b bytes.Buffer
		command.Stdout = &boundedBuffer{b: &b}
		command.Stderr = io.Discard
		command.WaitDelay = time.Second
		err := command.Run()
		return b.String(), err
	}
	top, err := run("rev-parse", "--show-toplevel")
	if err != nil {
		return nil, fmt.Errorf("repository identity unavailable")
	}
	canonical, err := filepath.EvalSymlinks(strings.TrimSpace(top))
	if err != nil || filepath.Clean(canonical) != filepath.Clean(root) {
		return nil, fmt.Errorf("repository root mismatch")
	}
	head, err := run("rev-parse", "--verify", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("repository has no commit")
	}
	status, err := run("status", "--porcelain=v1", "-z", "--untracked-files=all")
	if err != nil {
		return nil, fmt.Errorf("repository coverage unavailable")
	}
	h := sha256.Sum256([]byte(status))
	return &model.RepoIdentity{Root: root, Commit: strings.TrimSpace(head), StatusDigest: hex.EncodeToString(h[:]), Clean: status == ""}, nil
}

type boundedBuffer struct{ b *bytes.Buffer }

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.b.Len()+len(p) > MaxOutput {
		return 0, fmt.Errorf("output too large")
	}
	return b.b.Write(p)
}
func evaluate(ctx context.Context, st model.State, d model.Evaluator, e model.Evidence) model.Evidence {
	fail := func(reason string) model.Evidence { e.Outcome = "unknown"; e.Reason = reason; return e }
	if !d.Current(st) {
		return fail("definition_or_task_changed")
	}
	before, err := snapshots(ctx, st, d)
	if err != nil {
		return fail(err.Error())
	}
	e.Inputs = before
	resource := st.Resources[d.Spec.ResourceID]
	var repoBefore *model.RepoIdentity
	if d.Spec.Kind == "repo.state" {
		repoBefore, err = repoIdentity(ctx, resource.Root)
		if err != nil {
			return fail(err.Error())
		}
	}
	if d.Spec.Kind == "test.exit" {
		if _, statErr := os.Lstat(filepath.Join(resource.Root, ".git")); statErr == nil {
			repoBefore, err = repoIdentity(ctx, resource.Root)
			if err != nil {
				return fail(err.Error())
			}
		}
	}
	e.Repo = repoBefore
	e.Outcome = "matched"
	switch d.Spec.Kind {
	case "artifact.digest":
		for _, ref := range before {
			if ref.ID == d.Spec.ResourceID && ref.Snapshot.Digest != d.Spec.ExpectedDigest {
				e.Outcome = "not_matched"
				e.Reason = "artifact_digest_differs"
			}
		}
	case "repo.state":
		if (d.Spec.RequireClean && !repoBefore.Clean) || (d.Spec.ExpectedCommit != "" && d.Spec.ExpectedCommit != repoBefore.Commit) {
			e.Outcome = "not_matched"
			e.Reason = "repository_predicate_not_met"
		}
	case "test.exit":
		executable, err := executableDigest(d.Spec.Argv[0])
		if err != nil {
			return fail("executable_unavailable")
		}
		e.ExecutableDigest = executable
		e.EnvironmentDigest = model.ContentDigest(minimalEnv())
		testCtx, cancel := context.WithTimeout(ctx, time.Duration(d.Spec.TimeoutSeconds)*time.Second)
		defer cancel()
		command := exec.CommandContext(testCtx, d.Spec.Argv[0], d.Spec.Argv[1:]...)
		command.Dir = resource.Root
		command.Env = minimalEnv()
		command.WaitDelay = time.Second
		sink := &output{hash: sha256.New(), cancel: cancel}
		command.Stdout = sink
		command.Stderr = sink
		err = command.Run()
		e.OutputDigest = hex.EncodeToString(sink.hash.Sum(nil))
		e.OutputBytes = sink.count
		if command.ProcessState != nil {
			code := command.ProcessState.ExitCode()
			e.ExitCode = &code
		}
		afterExecutable, digestErr := executableDigest(d.Spec.Argv[0])
		if digestErr != nil || afterExecutable != executable {
			return fail("executable_changed")
		}
		if sink.limited {
			return fail("output_limit_exceeded")
		}
		if testCtx.Err() != nil {
			return fail("test_timeout_or_cancelled_process_tree_not_verified")
		}
		if err != nil {
			var exit *exec.ExitError
			if errors.As(err, &exit) {
				e.Outcome = "not_matched"
				e.Reason = "test_exit_nonzero"
			} else {
				return fail("test_execution_unavailable")
			}
		}
	}
	after, err := snapshots(ctx, st, d)
	if err != nil || !reflect.DeepEqual(before, after) {
		return fail("inputs_changed_or_unavailable")
	}
	if repoBefore != nil {
		afterRepo, err := repoIdentity(ctx, resource.Root)
		if err != nil || !reflect.DeepEqual(repoBefore, afterRepo) {
			return fail("repository_changed_during_evaluation")
		}
	}
	return e
}
func ValidateEvidence(ctx context.Context, st model.State, e model.Evidence) error {
	if !model.EvidenceCurrent(st, e) || e.Outcome != "matched" {
		return fmt.Errorf("evidence_or_definition_stale")
	}
	d := st.Evaluators[e.EvaluatorID]
	now, err := snapshots(ctx, st, d)
	if err != nil || !reflect.DeepEqual(now, e.Inputs) {
		return fmt.Errorf("evidence_inputs_changed_or_unavailable")
	}
	if e.Repo != nil {
		repo, err := repoIdentity(ctx, st.Resources[d.Spec.ResourceID].Root)
		if err != nil || !reflect.DeepEqual(repo, e.Repo) {
			return fmt.Errorf("evidence_repository_changed")
		}
	}
	if d.Spec.Kind == "test.exit" {
		exe, err := executableDigest(d.Spec.Argv[0])
		if err != nil || exe != e.ExecutableDigest || model.ContentDigest(minimalEnv()) != e.EnvironmentDigest {
			return fmt.Errorf("evidence_execution_environment_changed")
		}
	}
	return nil
}

// ValidateTarget is called inside the task writer transaction immediately before
// acceptance. It observes current inputs, but never reruns an executable.
func ValidateTarget(ctx context.Context, st model.State, target string) error {
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	r, step, err := model.ResolveTarget(st, target)
	if err != nil {
		return err
	}
	done := r.Task.Done
	if step != nil {
		done = step.Done
	}
	for _, check := range done.Checks {
		e, current := model.LatestEvidence(st, target, check.ID)
		if current && e.Outcome == "matched" {
			if err := ValidateEvidence(ctx, st, e); err != nil {
				return err
			}
		}
	}
	return nil
}
