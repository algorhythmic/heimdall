//go:build linux

package continuity

import (
	"context"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"heimdall/internal/model"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"golang.org/x/sys/unix"
)

func artifactHost() (string, error) {
	b, err := os.ReadFile("/etc/machine-id")
	if err != nil || strings.TrimSpace(string(b)) == "" {
		return "", fmt.Errorf("local machine identity unavailable")
	}
	h := sha256.Sum256([]byte(strings.TrimSpace(string(b))))
	return hex.EncodeToString(h[:]), nil
}

// ObserveArtifact never retains file contents. It compares two complete byte /
// metadata observations. Git probing is opt-in and uses non-filtering plumbing.
func ObserveArtifact(ctx context.Context, r model.Resource, relative string, git bool) (model.ArtifactObservation, error) {
	file, err := model.ArtifactResource(r, relative)
	if err != nil {
		return model.ArtifactObservation{}, err
	}
	first, err := artifactOnce(ctx, file, git)
	if err != nil {
		return first, err
	}
	second, err := artifactOnce(ctx, file, git)
	if err != nil {
		return second, err
	}
	if !reflect.DeepEqual(first, second) {
		return second, fmt.Errorf("artifact changed while observing")
	}
	return second, nil
}

func artifactOnce(ctx context.Context, r model.Resource, git bool) (model.ArtifactObservation, error) {
	out := model.ArtifactObservation{}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	canonical, err := filepath.EvalSymlinks(r.Root)
	if err != nil {
		return out, err
	}
	if canonical != r.Root {
		return out, fmt.Errorf("artifact root changed or is a symlink")
	}
	before, err := os.Lstat(r.Root)
	if err != nil {
		return out, err
	}
	root, err := os.OpenRoot(r.Root)
	if err != nil {
		return out, err
	}
	defer root.Close()
	opened, err := root.Stat(".")
	if err != nil || !os.SameFile(before, opened) {
		return out, fmt.Errorf("artifact root changed during open")
	}
	if !filepath.IsLocal(r.Path) {
		return out, fmt.Errorf("artifact escapes its resource root")
	}
	// Descend with independent handles and inode checks. A raced symlink cannot
	// redirect a later file open into an excluded sibling directory.
	parts := strings.Split(filepath.ToSlash(r.Path), "/")
	parent := root
	for _, part := range parts[:len(parts)-1] {
		stat, err := parent.Lstat(part)
		if err != nil {
			return out, err
		}
		if !stat.IsDir() || stat.Mode()&os.ModeSymlink != 0 {
			return out, fmt.Errorf("artifact symlink/non-directory refused")
		}
		next, err := parent.OpenRoot(part)
		if err != nil {
			return out, err
		}
		defer next.Close()
		actual, err := next.Stat(".")
		if err != nil || !os.SameFile(stat, actual) {
			return out, fmt.Errorf("artifact directory changed during open")
		}
		parent = next
	}
	name := parts[len(parts)-1]
	stat, err := parent.Lstat(name)
	if err != nil {
		return out, err
	}
	if !stat.Mode().IsRegular() || stat.Size() > MaxResourceBytes {
		return out, fmt.Errorf("artifact must be a regular file of at most 64 MiB")
	}
	f, err := parent.OpenFile(name, os.O_RDONLY|unix.O_NOFOLLOW|unix.O_NONBLOCK, 0)
	if err != nil {
		return out, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || !os.SameFile(stat, info) {
		return out, fmt.Errorf("artifact file changed before read")
	}
	h, blob1, blob256 := sha256.New(), sha1.New(), sha256.New()
	fmt.Fprintf(blob1, "blob %d\x00", info.Size())
	fmt.Fprintf(blob256, "blob %d\x00", info.Size())
	n, err := io.Copy(io.MultiWriter(h, blob1, blob256), io.LimitReader(f, MaxResourceBytes+1))
	if err != nil {
		return out, err
	}
	after, err := f.Stat()
	if err != nil {
		return out, err
	}
	current, err := parent.Lstat(name)
	if err != nil {
		return out, err
	}
	if n > MaxResourceBytes || n != info.Size() || n != after.Size() || !info.ModTime().Equal(after.ModTime()) || info.Mode() != after.Mode() || !os.SameFile(after, current) || current.Mode()&os.ModeSymlink != 0 {
		return out, fmt.Errorf("artifact changed during read")
	}
	currentRoot, err := os.Lstat(r.Root)
	if err != nil || !os.SameFile(before, currentRoot) {
		return out, fmt.Errorf("artifact root replaced")
	}
	if err := ctx.Err(); err != nil {
		return out, err
	}
	out.Digest = hex.EncodeToString(h.Sum(nil))
	out.Bytes = n
	out.Mode = uint32(after.Mode().Perm())
	if git {
		out.Git, err = observeArtifactGit(ctx, filepath.Join(r.Root, r.Path), out, hex.EncodeToString(blob1.Sum(nil)), hex.EncodeToString(blob256.Sum(nil)))
	}
	return out, err
}
