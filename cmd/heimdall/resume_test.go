package main

import (
	"bytes"
	"context"
	"encoding/json"
	"heimdall/internal/continuity"
	"heimdall/internal/daemon"
	"heimdall/internal/model"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func resumeFixture(t *testing.T) (func(...string) ([]byte, error), string) {
	t.Helper()
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	clock := func() time.Time { return time.Date(2026, 9, 8, 18, 0, 0, 0, time.UTC) }
	call := func(args ...string) ([]byte, error) {
		var b bytes.Buffer
		err := run(ctx, append(args, "--data-dir", dir, "--now", clock().Format(time.RFC3339)), &b)
		return b.Bytes(), err
	}
	if _, err := call("init"); err != nil {
		cancel()
		t.Fatal(err)
	}
	ready, done := make(chan daemon.Endpoint, 1), make(chan error, 1)
	go func() { done <- daemon.Serve(ctx, dir, clock, func(ep daemon.Endpoint) { ready <- ep }) }()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Error(err)
			}
		case <-time.After(5 * time.Second):
			t.Error("daemon did not stop")
		}
	})
	select {
	case <-ready:
	case <-time.After(5 * time.Second):
		t.Fatal("daemon did not start")
	}
	return call, dir
}

func TestResumeAndCheckpointDraftWorkflow(t *testing.T) {
	invoke, dir := resumeFixture(t)
	cli := func(args ...string) []byte {
		t.Helper()
		b, err := invoke(args...)
		if err != nil {
			t.Fatal(args, err)
		}
		return b
	}
	input := func(name string, v any) string {
		t.Helper()
		path := filepath.Join(dir, name+".json")
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, b, 0600); err != nil {
			t.Fatal(err)
		}
		return path
	}
	cli("add", "Resume fixture", "--id", "resume-test", "--status", "active", "--next-action", "Inspect the accepted plan")
	cli("add", "Other task", "--id", "other-task", "--status", "active")
	missing := string(cli("resume", "resume-test"))
	if !strings.Contains(missing, "No accepted contract") || !strings.Contains(missing, "No saved checkpoint") || !strings.Contains(missing, "Inspect the accepted plan") {
		t.Fatal(missing)
	}
	if _, err := invoke("checkpoint", "draft", "resume-test", "--output", filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("draft without contract accepted")
	}
	var task model.TaskRecord
	json.Unmarshal(cli("state", "resume-test"), &task)
	cli("contract", "accept", "resume-test", "--expected-task-revision", "1", "--file", input("contract", continuity.ContractInput{Previous: "none", Objective: "Return to the right work", Constraints: []string{"Keep checkpoint preconditions"}}))
	before := cli("events")
	cli("resume", "resume-test")
	if !bytes.Equal(before, cli("events")) {
		t.Fatal("resume appended events")
	}
	path := filepath.Join(dir, "draft.json")
	cli("checkpoint", "draft", "resume-test", "--output", path)
	raw, _ := os.ReadFile(path)
	var draft continuity.Request
	if err := json.Unmarshal(raw, &draft); err != nil {
		t.Fatal(err)
	}
	if draft.Checkpoint.Summary != "" || draft.Checkpoint.Previous != "none" || *draft.ExpectedTaskRevision != task.Revision {
		t.Fatal("draft invented progress or lost preconditions", draft)
	}
	if _, err := invoke("checkpoint", "draft", "resume-test", "--output", path); err == nil {
		t.Fatal("draft overwrote a saved file")
	}
	saved, _ := os.ReadFile(path)
	if !bytes.Equal(raw, saved) {
		t.Fatal("draft changed on failed overwrite")
	}
	if _, err := invoke("checkpoint", "submit", "resume-test", "--file", path); err == nil {
		t.Fatal("empty progress summary accepted")
	}
	draft.Checkpoint.Summary = "Saved planning progress\x1b]52;c;hidden\a\r\u202e"
	draft.Checkpoint.NextAction = "Review the implementation"
	path = input("draft", draft)
	if _, err := invoke("checkpoint", "submit", "other-task", "--file", path); err == nil {
		t.Fatal("cross-target draft accepted")
	}
	result := cli("checkpoint", "submit", "resume-test", "--file", path)
	if !bytes.Equal(result, cli("checkpoint", "submit", "resume-test", "--file", path)) {
		t.Fatal("retry did not return exact checkpoint")
	}
	var checkpoints []model.Checkpoint
	json.Unmarshal(cli("checkpoint", "list", "resume-test"), &checkpoints)
	if len(checkpoints) != 1 || checkpoints[0].ID != draft.ID {
		t.Fatal("duplicate checkpoint on retry", checkpoints)
	}
	text := string(cli("resume", "resume-test"))
	for _, expected := range []string{"Return to the right work", "Keep checkpoint preconditions", "Review the implementation", "less than a minute ago", "\\x1b", "\\u202e"} {
		if !strings.Contains(text, expected) {
			t.Fatalf("missing %q in %s", expected, text)
		}
	}
	if strings.ContainsAny(text, "\x1b\a\r\u202e") {
		t.Fatal("terminal escape or bidi control survived rendering")
	}
	var machine continuity.ResumeView
	if err := json.Unmarshal(cli("--json", "resume", "resume-test"), &machine); err != nil || machine.Checkpoint.ID != draft.ID {
		t.Fatal("explicit JSON resume unavailable", err)
	}
	var original map[string]any
	json.Unmarshal(cli("context", "resume-test"), &original)
	if _, ok := original["reviews"]; ok {
		t.Fatal("existing context JSON changed")
	}
	if _, err := invoke("resume", "resume-test", "--budget", "1"); err == nil || !strings.Contains(err.Error(), "budget_too_small") {
		t.Fatal("small budget was silently truncated", err)
	}
	// Two independently edited drafts share the same head. The loser stays on
	// disk with its original identity and cannot silently advance to a new head.
	loser, winner := filepath.Join(dir, "loser.json"), filepath.Join(dir, "winner.json")
	cli("checkpoint", "draft", "resume-test", "--output", loser)
	cli("checkpoint", "draft", "resume-test", "--output", winner)
	for _, file := range []string{loser, winner} {
		b, _ := os.ReadFile(file)
		var request continuity.Request
		json.Unmarshal(b, &request)
		request.Checkpoint.Summary = "New planning progress"
		b, _ = json.Marshal(request)
		if err := os.WriteFile(file, b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	cli("checkpoint", "submit", "resume-test", "--file", winner)
	losingBytes, _ := os.ReadFile(loser)
	if _, err := invoke("checkpoint", "submit", "resume-test", "--file", loser); err == nil || !strings.Contains(err.Error(), "409") {
		t.Fatal("competing draft did not conflict", err)
	}
	retained, _ := os.ReadFile(loser)
	if !bytes.Equal(losingBytes, retained) {
		t.Fatal("conflicting draft modified")
	}
	cli("update", "resume-test", "--title", "Changed direction")
	if _, err := invoke("checkpoint", "draft", "resume-test", "--output", filepath.Join(dir, "stale.json")); err == nil {
		t.Fatal("stale accepted contract allowed new draft")
	}
}

func TestResumeTerminalTextAndFutureCheckpoint(t *testing.T) {
	for _, value := range []string{"\x1b[2J", "\x1b]8;;https://example.test\a", "\b\t\n\r\x7f\u009b\u2028\u2029\u2066"} {
		if strings.ContainsAny(terminalText(value), "\x1b\a\b\t\n\r\x7f\u009b\u2028\u2029\u2066") {
			t.Fatal("unescaped terminal controls")
		}
	}
	now := time.Now()
	if !strings.Contains(checkpointAge(now.Add(time.Hour), now), "check clock") {
		t.Fatal("future checkpoint appeared fresh")
	}
}
