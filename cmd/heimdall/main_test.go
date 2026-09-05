package main

import (
	"bytes"
	"context"
	"encoding/json"
	"heimdall/internal/daemon"
	"heimdall/internal/model"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCLIOverLiveDaemon(t *testing.T) {
	dir := t.TempDir()
	fixed := time.Date(2026, 9, 4, 18, 0, 0, 0, time.UTC)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	invoke := func(args ...string) []byte {
		t.Helper()
		var b bytes.Buffer
		args = append(args, "--data-dir", dir, "--now", fixed.Format(time.RFC3339))
		if err := run(ctx, args, &b); err != nil {
			t.Fatal(args, err)
		}
		return bytes.TrimSpace(b.Bytes())
	}
	invoke("init")
	ready := make(chan daemon.Endpoint, 1)
	done := make(chan error, 1)
	go func() {
		done <- daemon.Serve(ctx, dir, func() time.Time { return fixed }, func(ep daemon.Endpoint) { ready <- ep })
	}()
	select {
	case <-ready:
	case err := <-done:
		t.Fatal(err)
	case <-time.After(5 * time.Second):
		t.Fatal("start timeout")
	}
	invoke("doctor")
	invoke("import-tasks", "../../testdata/tasks.yaml")
	invoke("add", "Another task", "--id", "another-task", "--status", "active")
	invoke("update", "another-task", "--title", "Edited title")
	invoke("capture", "heimdall-core/reference: source for work", "--pointer", "https://example.test", "--client", "cli")
	invoke("complete", "heimdall-core#store")
	invoke("complete", "heimdall-core#tasks")
	var ps []model.Proposal
	if err := json.Unmarshal(invoke("ratify"), &ps); err != nil || len(ps) != 1 {
		t.Fatal(string(invoke("ratify")), err)
	}
	invoke("ratify", ps[0].ID, "--accept")
	before := invoke("state")
	invoke("replay")
	after := invoke("state")
	if !bytes.Equal(before, after) {
		t.Fatal("CLI replay mismatch")
	}
	// A daemon-produced view is a no-op for the watcher, and an editor rename is ingested.
	var st model.State
	_ = json.Unmarshal(after, &st)
	doc := st.Document()
	for i := range doc.Tasks {
		if doc.Tasks[i].ID == "another-task" {
			doc.Tasks[i].Title = "Atomic editor save"
		}
	}
	file, _ := json.Marshal(doc)
	temp := filepath.Join(dir, "editor-save.tmp")
	if err := os.WriteFile(temp, file, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temp, filepath.Join(dir, "tasks.yaml")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(4 * time.Second)
	for {
		var r model.TaskRecord
		_ = json.Unmarshal(invoke("state", "another-task"), &r)
		if r.Task.Title == "Atomic editor save" {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("watcher did not ingest rename")
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("shutdown timeout")
	}
	if _, err := os.Stat(filepath.Join(dir, "endpoint.json")); !os.IsNotExist(err) {
		t.Fatal("stale endpoint after shutdown")
	}
}
func TestBadEndpointCannotLeakToken(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "endpoint.json"), []byte(`{"url":"http://example.test","token":"secret"}`), 0600)
	if _, err := call(context.Background(), options{dir: dir}, "GET", "/health", nil); err == nil {
		t.Fatal("accepted remote endpoint")
	}
}
