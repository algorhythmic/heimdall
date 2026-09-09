package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"heimdall/internal/continuity"
	"heimdall/internal/model"
	"io"
	"net/url"
	"os"
	"path/filepath"
)

func checkpointDraftCLI(ctx context.Context, o options, action, target string, args []string, out io.Writer) error {
	f := flag.NewFlagSet("checkpoint "+action, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	var path string
	if action == "draft" {
		f.StringVar(&path, "output", "", "new JSON draft; never overwrite")
	} else {
		f.StringVar(&path, "file", "", "edited checkpoint draft")
	}
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 || path == "" {
		return fmt.Errorf("checkpoint %s requires TARGET and a draft file path", action)
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if action == "submit" {
		return submitCheckpointDraft(ctx, o, target, abs, out)
	}
	raw, err := call(ctx, o, "GET", "/continuity/context?target="+url.QueryEscape(target)+"&budget=16000", nil)
	if err != nil {
		return err
	}
	var bundle continuity.Bundle
	if err = json.Unmarshal(raw, &bundle); err != nil {
		return err
	}
	for _, issue := range bundle.Issues {
		if issue.Target == target && model.Contains([]string{"missing_contract", "stale_contract", "contract_scope_changed"}, issue.Code) {
			return fmt.Errorf("review context before drafting a checkpoint: %s", terminalText(issue.Detail))
		}
	}
	var contractID string
	for _, c := range bundle.Contracts {
		if c.Target == target {
			contractID = c.ID
		}
	}
	if contractID == "" {
		return fmt.Errorf("accept a contract for this target before drafting a checkpoint")
	}
	input := &continuity.CheckpointInput{Previous: "none", ContractID: contractID, NextAction: bundle.Task.Task.NextAction, Blockers: []string{}}
	if cp := bundle.Checkpoint; cp != nil {
		input.Previous = cp.ID
		input.NextAction = cp.NextAction
		input.CurrentStep = cp.CurrentStep
		input.Blockers = append(input.Blockers, cp.Blockers...)
	}
	requestID := o.requestID
	if requestID == "" {
		requestID = model.NewID()
	}
	if !model.OpaqueID.MatchString(requestID) {
		return fmt.Errorf("invalid request ID")
	}
	request := continuity.Request{Version: 1, ID: requestID, Op: "checkpoint.record", Target: target, ExpectedTaskRevision: &bundle.Task.Revision, Checkpoint: input}
	if bundle.Checkpoint != nil && len(bundle.Checkpoint.Artifacts) > 0 {
		request.Version = 2
		input.Artifacts = append([]model.ArtifactRef{}, bundle.Checkpoint.Artifacts...)
	}
	// Summary intentionally starts empty: preparing a draft is not new progress.
	content, err := json.MarshalIndent(request, "", "  ")
	if err != nil {
		return err
	}
	file, err := os.OpenFile(abs, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create new draft: %w", err)
	}
	_, err = file.Write(append(content, '\n'))
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("draft write failed at %q: %w", abs, err)
	}
	return json.NewEncoder(out).Encode(map[string]any{"status": "draft_saved", "draft_file": abs, "target": target, "request_id": request.ID})
}

func submitCheckpointDraft(ctx context.Context, o options, target, path string, out io.Writer) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, continuity.MaxRequest+1))
	if err != nil {
		return err
	}
	request, err := continuity.Decode(raw)
	if err != nil {
		return fmt.Errorf("invalid checkpoint draft (file retained): %w", err)
	}
	if request.Op != "checkpoint.record" || request.Target != target || (o.requestID != "" && o.requestID != request.ID) {
		return fmt.Errorf("draft must be a checkpoint for the selected target with its original request ID")
	}
	result, err := call(ctx, o, "POST", "/continuity/command", request)
	if err != nil {
		return fmt.Errorf("checkpoint draft retained at %q; retry the unchanged file after transport failure, or prepare a new draft after a conflict: %w", path, err)
	}
	_, err = fmt.Fprintln(out, string(result))
	return err
}
