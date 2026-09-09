package main

import (
	"encoding/json"
	"heimdall/internal/model"
	"heimdall/internal/workspace"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestWorkspaceCLIWorkflow(t *testing.T) {
	invoke, dir := resumeFixture(t)
	cli := func(args ...string) []byte {
		t.Helper()
		b, err := invoke(args...)
		if err != nil {
			t.Fatal(args, err)
		}
		return b
	}
	file := func(name string, v any) string {
		t.Helper()
		b, err := json.Marshal(v)
		if err != nil {
			t.Fatal(err)
		}
		p := filepath.Join(dir, name+".json")
		if err := os.WriteFile(p, b, 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	cli("add", "Alpha", "--id", "alpha", "--status", "active")
	cli("add", "Beta", "--id", "beta", "--status", "active")
	raw, err := os.ReadFile("../../testdata/workspace/requests.json")
	if err != nil {
		t.Fatal(err)
	}
	var requests []workspace.Request
	if err := json.Unmarshal(raw, &requests); err != nil {
		t.Fatal(err)
	}
	manifest, binding, unbind := requests[0], requests[1], requests[2]
	manifestFile := file("manifest", manifest.Manifest)
	cli("workspace", "accept", "alpha", "--expected-task-revision", "1", "--request-id", manifest.ID, "--file", manifestFile)
	bindingFile := file("binding", binding.Session)
	args := []string{"session", "bind", "alpha", "--expected-task-revision", "1", "--request-id", binding.ID, "--file", bindingFile}
	accepted := cli(args...)
	if string(accepted) != string(cli(args...)) {
		t.Fatal("CLI retry changed")
	}
	var view workspace.View
	if err := json.Unmarshal(cli("workspace", "show", "alpha"), &view); err != nil || view.Bindings[0].Status != "unverified" {
		t.Fatal(view, err)
	}
	var record model.SessionBinding
	if err := json.Unmarshal(cli("session", "show", "alpha", "--surface", binding.Session.SurfaceID), &record); err != nil || record.ID != binding.ID {
		t.Fatal(record, err)
	}
	before := cli("events")
	cli("workspace", "show", "alpha", "--id", manifest.ID)
	cli("session", "show", "alpha", "--surface", binding.Session.SurfaceID, "--id", binding.ID)
	if !reflect.DeepEqual(before, cli("events")) {
		t.Fatal("reads wrote events")
	}
	for _, args := range [][]string{
		{"workspace", "show", "beta", "--id", manifest.ID},
		{"session", "show", "beta", "--surface", binding.Session.SurfaceID, "--id", binding.ID},
		{"session", "show", "alpha"},
		{"workspace", "show", "alpha/step"},
		{"workspace", "accept", "alpha", "--file", manifestFile},
		{"workspace", "accept", "alpha", "--expected-task-revision", "1", "--file", file("unknown", map[string]string{"actor": "cli"})},
	} {
		if _, err := invoke(args...); err == nil {
			t.Fatal("invalid CLI accepted", args)
		}
	}
	loser := file("competing", binding.Session)
	if _, err := invoke("session", "bind", "alpha", "--expected-task-revision", "1", "--file", loser); err == nil || !strings.Contains(err.Error(), "409") {
		t.Fatal("lost binding precondition", err)
	}
	cli("session", "unbind", "alpha", "--expected-task-revision", "1", "--request-id", unbind.ID, "--file", file("unbind", unbind.Session))
	if err := json.Unmarshal(cli("workspace", "show", "alpha"), &view); err != nil || view.Bindings[0].Status != "unbound" {
		t.Fatal(view, err)
	}
	if err := json.Unmarshal(cli("session", "show", "alpha", "--surface", binding.Session.SurfaceID, "--id", binding.ID), &record); err != nil || !record.Active {
		t.Fatal("unbind rewrote history", err)
	}
	retained, err := os.ReadFile(loser)
	if err != nil {
		t.Fatal(err)
	}
	original, _ := json.Marshal(binding.Session)
	if string(retained) != string(original) {
		t.Fatal("failed CLI changed input file")
	}
}
