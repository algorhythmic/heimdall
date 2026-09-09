package main

import (
	"context"
	"flag"
	"fmt"
	"heimdall/internal/model"
	"heimdall/internal/workspace"
	"io"
	"net/url"
	"os"
	"strings"
)

func operationCLI(ctx context.Context, o options, action, target string, args []string, out io.Writer) error {
	if !model.ValidID(target) {
		return fmt.Errorf("explicit workspace task required")
	}
	f := flag.NewFlagSet("workspace "+action, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	var file, surfaces, swap, swapFile, id, reason string
	var revision int64
	var dryRun bool
	switch action {
	case "open", "focus", "close":
		f.StringVar(&file, "file", "", "complete reviewed preview JSON")
		f.StringVar(&surfaces, "surfaces", "", "explicit comma-separated surface IDs or all")
		if action == "open" {
			f.StringVar(&swap, "swap", "", "explicit outgoing resident task")
			f.StringVar(&swapFile, "swap-file", "", "outgoing resident preview JSON")
		}
		if action == "close" {
			f.BoolVar(&dryRun, "dry-run", false, "fresh read-only close review")
		}
	case "operation":
		f.StringVar(&id, "id", "", "operation ID")
	case "cancel", "reconcile":
		f.StringVar(&id, "operation", "", "operation ID")
		f.Int64Var(&revision, "expected-revision", 0, "observed operation revision")
		f.StringVar(&reason, "reason", "", "explicit reason")
	}
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected operation arguments")
	}
	method, path := "GET", "/workspace/operation/list?target="+url.QueryEscape(target)
	var input any
	requestID := o.requestID
	if requestID == "" {
		requestID = model.NewID()
	}
	switch action {
	case "open", "focus", "close":
		if dryRun {
			if file != "" || surfaces != "" {
				return fmt.Errorf("close --dry-run takes no dispatch input")
			}
			return previewCLI(ctx, o, "diff", target, nil, out)
		}
		if file == "" || surfaces == "" {
			return fmt.Errorf("--file PREVIEW.json and --surfaces all|ID,ID required")
		}
		v, err := readOperationPreview(file, target)
		if err != nil {
			return err
		}
		ids := strings.Split(surfaces, ",")
		if surfaces == "all" {
			ids = []string{}
			for _, row := range v.Surfaces {
				if row.Membership != "removed" {
					ids = append(ids, row.SurfaceID)
				}
			}
		}
		r := workspace.OperationRequest{Version: 1, ID: requestID, Kind: action, Preview: v, SurfaceIDs: ids}
		if (swap == "") != (swapFile == "") {
			return fmt.Errorf("--swap TASK and --swap-file PREVIEW.json must be supplied together")
		}
		if swap != "" {
			p, err := readOperationPreview(swapFile, swap)
			if err != nil {
				return err
			}
			r.Swap = &p
		}
		method, path, input = "POST", "/workspace/operation/queue", r
	case "operation":
		if !model.OpaqueID.MatchString(id) {
			return fmt.Errorf("--id OPERATION required")
		}
		path = "/workspace/operation/show?target=" + url.QueryEscape(target) + "&id=" + url.QueryEscape(id)
	case "cancel", "reconcile":
		if !model.OpaqueID.MatchString(id) || revision < 1 || reason == "" {
			return fmt.Errorf("--operation ID --expected-revision N --reason TEXT required")
		}
		method, path, input = "POST", "/workspace/operation/"+action, workspace.OperationControl{Version: 1, ID: requestID, Target: target, OperationID: id, ExpectedRevision: revision, Reason: reason}
	}
	raw, err := call(ctx, o, method, path, input)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, string(raw))
	return err
}
func readOperationPreview(path, target string) (workspace.Preview, error) {
	var v workspace.Preview
	f, err := os.Open(path)
	if err != nil {
		return v, err
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, workspace.PreviewMaxBytes+1))
	if err != nil {
		return v, err
	}
	if len(raw) > workspace.PreviewMaxBytes {
		return v, fmt.Errorf("preview exceeds 256 KiB")
	}
	if err := model.StrictJSON(raw, &v); err != nil {
		return v, err
	}
	if v.Request.Target != target || v.Request.Validate() != nil {
		return v, fmt.Errorf("preview belongs to another task or lacks selected retained inputs")
	}
	return v, nil
}
