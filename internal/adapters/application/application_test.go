package application

import (
	"heimdall/internal/model"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestReviewedFilesAndEditorBoundaries(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("Linux recipe paths")
	}
	dir := t.TempDir()
	exe := filepath.Join(dir, "exe")
	saved := filepath.Join(dir, "saved.txt")
	if err := os.WriteFile(exe, []byte("reviewed executable bytes"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(saved, []byte("saved editor content"), 0600); err != nil {
		t.Fatal(err)
	}
	digest, err := Digest(exe)
	if err != nil {
		t.Fatal(err)
	}
	s := model.ApplicationSpec{Adapter: "foot-nvim", Executable: exe, ExecutableDigest: digest, Command: exe, CommandDigest: digest, Argv: []string{}, Cwd: dir, ClosePolicy: "leave_open", Editor: &model.EditorRestore{Files: []string{saved}, Active: 0, Line: 1, Column: 1}}
	if err := CheckFiles(s); err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*model.ApplicationSpec){
		func(s *model.ApplicationSpec) { s.Executable = filepath.Join(dir, "missing") },
		func(s *model.ApplicationSpec) { s.Cwd = filepath.Join(dir, "missing") },
		func(s *model.ApplicationSpec) { s.Editor.Files = []string{filepath.Join(dir, "missing")} },
		func(s *model.ApplicationSpec) { s.Editor.Active = 1 },
		func(s *model.ApplicationSpec) { s.ClosePolicy = "graceful_session_end" },
		func(s *model.ApplicationSpec) { s.Argv = []string{"-S", "unreviewed.vim"} },
	} {
		bad := model.Clone(s)
		change(&bad)
		if CheckFiles(bad) == nil {
			t.Fatal("unsafe or unavailable editor input accepted", bad)
		}
	}
	if err := os.WriteFile(exe, []byte("different executable bytes"), 0700); err != nil {
		t.Fatal(err)
	}
	if CheckFiles(s) == nil {
		t.Fatal("changed executable retained review authority")
	}
}
