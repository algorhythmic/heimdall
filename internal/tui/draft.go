package tui

import (
	"encoding/json"
	"fmt"
	"heimdall/internal/continuity"
	"heimdall/internal/model"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

type draft struct {
	Path    string
	Request continuity.Request
	Locked  bool
}
type savedRequest struct {
	Version int             `json:"version"`
	Target  string          `json:"target"`
	Path    string          `json:"path"`
	Body    json.RawMessage `json:"body"`
}

func writePrivate(path string, value any, exclusive bool) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	// Refuse symlinks/nonregular destinations; replace with a private synced file.
	if info, e := os.Lstat(path); e == nil {
		if exclusive {
			return fmt.Errorf("file already exists: %s", path)
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("refusing nonregular draft: %s", path)
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	if exclusive {
		f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return e
		}
		_, e = f.Write(raw)
		if e == nil {
			e = f.Sync()
		}
		closeErr := f.Close()
		if e == nil {
			e = closeErr
		}
		return e
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".tui-draft-*")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(raw); err == nil {
		err = f.Sync()
	}
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(name, path)
}
func readBounded(path string, out any) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Size() > continuity.MaxRequest {
		return fmt.Errorf("draft must be a regular file smaller than 64 KiB")
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, continuity.MaxRequest+1))
	if err != nil {
		return err
	}
	if len(raw) > continuity.MaxRequest {
		return fmt.Errorf("draft too large")
	}
	return model.StrictJSON(raw, out)
}
func draftPath(dir, target string) string {
	return filepath.Join(dir, "tui-drafts", strings.ReplaceAll(target, "#", "_")+".json")
}
func prepareDraft(dir string, v continuity.ResumeView) (*draft, error) {
	path := draftPath(dir, v.Target)
	var request continuity.Request
	if err := readBounded(path, &request); err == nil {
		if request.Target != v.Target || request.Op != "checkpoint.record" || request.Checkpoint == nil || !model.OpaqueID.MatchString(request.ID) {
			return nil, fmt.Errorf("retained draft does not match the selected task")
		}
		_, err = os.Stat(path + ".attempt")
		return &draft{path, request, err == nil}, nil
	} else if !os.IsNotExist(err) {
		return nil, err
	}
	contract := ""
	for _, c := range v.Contracts {
		if c.Target == v.Target {
			contract = c.ID
		}
	}
	for _, issue := range v.Issues {
		if issue.Target == v.Target && model.Contains([]string{"missing_contract", "stale_contract", "contract_scope_changed"}, issue.Code) {
			return nil, fmt.Errorf("%s", issue.Detail)
		}
	}
	if contract == "" {
		return nil, fmt.Errorf("Accept a contract for this task before saving progress (heimdall contract accept).")
	}
	input := &continuity.CheckpointInput{Previous: "none", ContractID: contract, NextAction: v.Task.Task.NextAction, Blockers: []string{}}
	version := 1
	if cp := v.Checkpoint; cp != nil {
		input.Previous = cp.ID
		input.NextAction = cp.NextAction
		input.CurrentStep = cp.CurrentStep
		input.Blockers = append(input.Blockers, cp.Blockers...)
		if len(cp.Artifacts) > 0 {
			version = 2
			input.Artifacts = append([]model.ArtifactRef{}, cp.Artifacts...)
		}
	}
	revision := v.Task.Revision
	request = continuity.Request{Version: version, ID: model.NewID(), Op: "checkpoint.record", Target: v.Target, ExpectedTaskRevision: &revision, Checkpoint: input}
	if err := writePrivate(path, request, true); err != nil {
		return nil, err
	}
	return &draft{Path: path, Request: request}, nil
}
func (d *draft) archive(outcome string) error {
	dest := filepath.Join(filepath.Dir(d.Path), d.Request.ID+"."+outcome+".json")
	if err := writePrivate(dest, d.Request, true); err != nil {
		return err
	}
	if err := os.Remove(d.Path); err != nil {
		return err
	}
	_ = os.Remove(d.Path + ".attempt")
	return nil
}
func (d *draft) reload() error {
	var changed continuity.Request
	if err := readBounded(d.Path, &changed); err != nil {
		return err
	}
	if changed.Checkpoint == nil {
		return fmt.Errorf("checkpoint payload required")
	}
	original := model.Clone(d.Request)
	edited := model.Clone(changed)
	edited.Checkpoint.Summary = original.Checkpoint.Summary
	edited.Checkpoint.NextAction = original.Checkpoint.NextAction
	edited.Checkpoint.CurrentStep = original.Checkpoint.CurrentStep
	edited.Checkpoint.Blockers = original.Checkpoint.Blockers
	if !reflect.DeepEqual(original, edited) {
		return fmt.Errorf("editor changed the draft identity, pins or preconditions; original retained in this dialog")
	}
	d.Request = changed
	return nil
}
