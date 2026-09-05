package continuity

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
)

const MaxResourceFiles = 4096
const MaxResourceBytes int64 = 64 << 20

func normalizeResource(r *model.Resource) error {
	if r.Kind != "file" && r.Kind != "tree" {
		return fmt.Errorf("resource kind must be file or tree")
	}
	if !filepath.IsAbs(r.Root) {
		return fmt.Errorf("resource root must be absolute")
	}
	root, err := filepath.EvalSymlinks(r.Root)
	if err != nil {
		return fmt.Errorf("canonicalize resource root: %w", err)
	}
	r.Root = filepath.Clean(root)
	if r.Path == "" {
		r.Path = "."
	}
	r.Path = filepath.Clean(r.Path)
	if !filepath.IsLocal(r.Path) {
		return fmt.Errorf("resource path must stay within root")
	}
	for _, v := range r.Exclude {
		if v == "" || v == "." || v == ".." || strings.ContainsAny(v, `/\`) {
			return fmt.Errorf("exclusions must be single directory/file names")
		}
	}
	if r.Kind == "file" && len(r.Exclude) > 0 {
		return fmt.Errorf("file resources do not accept exclusions")
	}
	if r.Kind == "tree" && !model.Contains(r.Exclude, ".git") {
		r.Exclude = append(r.Exclude, ".git")
	}
	sort.Strings(r.Exclude)
	return nil
}

// Observe makes two bounded passes, never emits file contents, and confines opens
// to an os.Root. It is an observation, not a lock on the user's working files.
func Observe(ctx context.Context, r model.Resource) (model.Snapshot, error) {
	a, err := observeOnce(ctx, r)
	if err != nil {
		return a, err
	}
	b, err := observeOnce(ctx, r)
	if err != nil {
		return b, err
	}
	if !reflect.DeepEqual(a, b) {
		return b, fmt.Errorf("resource changed while observing")
	}
	return b, nil
}
func observeOnce(ctx context.Context, r model.Resource) (model.Snapshot, error) {
	result := model.Snapshot{}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	beforeRoot, err := os.Lstat(r.Root)
	if err != nil {
		return result, err
	}
	if !beforeRoot.IsDir() || beforeRoot.Mode()&os.ModeSymlink != 0 {
		return result, fmt.Errorf("resource root changed or is a symlink")
	}
	root, err := os.OpenRoot(r.Root)
	if err != nil {
		return result, err
	}
	defer root.Close()
	openedRoot, err := root.Stat(".")
	if err != nil {
		return result, fmt.Errorf("stat opened resource root: %w", err)
	}
	if !os.SameFile(beforeRoot, openedRoot) {
		return result, fmt.Errorf("resource root changed during open")
	}
	// Reject visible symlink components as well as out-of-root symlink races.
	prefix := ""
	for _, part := range strings.Split(filepath.ToSlash(r.Path), "/") {
		prefix = filepath.Join(prefix, part)
		info, err := root.Lstat(prefix)
		if err != nil {
			return result, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return result, fmt.Errorf("symlink resources are unsupported")
		}
	}
	info, err := root.Lstat(r.Path)
	if err != nil {
		return result, err
	}
	if (r.Kind == "file" && !info.Mode().IsRegular()) || (r.Kind == "tree" && !info.IsDir()) {
		return result, fmt.Errorf("resource kind changed or unsupported file type")
	}
	h := sha256.New()
	enc := json.NewEncoder(h)
	entries := 0
	visit := func(name string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if name != filepath.ToSlash(r.Path) && model.Contains(r.Exclude, d.Name()) {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		entries++
		if entries > MaxResourceFiles*2 {
			return fmt.Errorf("resource entry limit exceeded")
		}
		stat, err := root.Lstat(filepath.FromSlash(name))
		if err != nil {
			return err
		}
		if stat.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink in resource: %s", name)
		}
		if stat.IsDir() {
			return enc.Encode([]any{"directory", name})
		}
		if !stat.Mode().IsRegular() {
			return fmt.Errorf("nonregular file in resource: %s", name)
		}
		result.Files++
		if result.Files > MaxResourceFiles || stat.Size() > MaxResourceBytes-result.Bytes {
			return fmt.Errorf("resource snapshot limit exceeded")
		}
		f, err := root.Open(filepath.FromSlash(name))
		if err != nil {
			return err
		}
		defer f.Close()
		opened, err := f.Stat()
		if err != nil {
			return err
		}
		if !opened.Mode().IsRegular() || !os.SameFile(stat, opened) {
			return fmt.Errorf("resource changed before read")
		}
		digest := sha256.New()
		n, err := io.Copy(digest, io.LimitReader(f, MaxResourceBytes-result.Bytes+1))
		if err != nil {
			return err
		}
		result.Bytes += n
		if result.Bytes > MaxResourceBytes {
			return fmt.Errorf("resource byte limit exceeded")
		}
		after, err := f.Stat()
		if err != nil {
			return err
		}
		if opened.Size() != after.Size() || !opened.ModTime().Equal(after.ModTime()) || n != opened.Size() {
			return fmt.Errorf("resource changed during read")
		}
		return enc.Encode([]any{"file", name, uint32(after.Mode().Perm()), hex.EncodeToString(digest.Sum(nil))})
	}
	if r.Kind == "tree" {
		err = fs.WalkDir(root.FS(), filepath.ToSlash(r.Path), visit)
	} else {
		err = visit(filepath.ToSlash(r.Path), fs.FileInfoToDirEntry(info), nil)
	}
	if err != nil {
		return result, err
	}
	result.Digest = hex.EncodeToString(h.Sum(nil))
	return result, nil
}
