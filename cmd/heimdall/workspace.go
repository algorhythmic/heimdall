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
)

func workspaceCLI(ctx context.Context, o options, verb string, args []string, out io.Writer) error {
	if len(args) < 2 {
		return fmt.Errorf("%s requires ACTION TASK", verb)
	}
	action, target := args[0], args[1]
	if verb == "workspace" && action == "verify" {
		return recoveryCLI(ctx, o, target, args[2:], out)
	}
	if verb == "workspace" && model.Contains([]string{"open", "focus", "close", "operation", "operations", "cancel", "reconcile"}, action) {
		return operationCLI(ctx, o, action, target, args[2:], out)
	}
	if verb == "workspace" && model.Contains([]string{"list", "diff", "preview", "validate"}, action) {
		return previewCLI(ctx, o, action, target, args[2:], out)
	}
	if !model.ValidID(target) {
		return fmt.Errorf("workspace/session requires an explicit task ID")
	}
	if verb == "session" && model.Contains([]string{"bind-herdr", "refresh", "publish"}, action) {
		return herdrCLI(ctx, o, action, target, args[2:], out)
	}
	f := flag.NewFlagSet(verb+" "+action, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	var path string
	var input any
	if action == "show" {
		id := f.String("id", "", "specific immutable record ID")
		var surface string
		if verb == "session" {
			f.StringVar(&surface, "surface", "", "required logical surface ID")
		}
		if err := f.Parse(args[2:]); err != nil {
			return err
		}
		if f.NArg() != 0 || (*id != "" && !model.OpaqueID.MatchString(*id)) {
			return fmt.Errorf("invalid show arguments")
		}
		q := url.Values{"target": {target}}
		path = "/workspace/state"
		if verb == "session" {
			if !model.OpaqueID.MatchString(surface) {
				return fmt.Errorf("session show requires --surface ID")
			}
			q.Set("surface", surface)
			path = "/workspace/session"
		}
		if *id != "" {
			q.Set("id", *id)
			if verb == "workspace" {
				path = "/workspace/manifest"
			}
		}
		path += "?" + q.Encode()
	} else {
		if !((verb == "workspace" && action == "accept") || (verb == "session" && (action == "bind" || action == "unbind"))) {
			return fmt.Errorf("unsupported %s action", verb)
		}
		file := f.String("file", "", "JSON operation input")
		rev := f.Int64("expected-task-revision", 0, "required observed task revision")
		if err := f.Parse(args[2:]); err != nil {
			return err
		}
		if f.NArg() != 0 || *file == "" || *rev < 1 {
			return fmt.Errorf("--file FILE and --expected-task-revision N required")
		}
		opened, err := os.Open(*file)
		if err != nil {
			return err
		}
		defer opened.Close()
		body, err := io.ReadAll(io.LimitReader(opened, workspace.MaxRequest+1))
		if err != nil {
			return err
		}
		if len(body) > workspace.MaxRequest {
			return fmt.Errorf("workspace input exceeds 64 KiB")
		}
		r := workspace.Request{Version: 1, ID: o.requestID, Op: verb + "." + action, Target: target, ExpectedTaskRevision: *rev}
		if r.ID == "" {
			r.ID = model.NewID()
		}
		if verb == "workspace" {
			r.Manifest = &workspace.ManifestInput{}
			err = model.StrictJSON(body, r.Manifest)
		} else {
			r.Session = &workspace.SessionInput{}
			err = model.StrictJSON(body, r.Session)
		}
		if err != nil {
			return err
		}
		if err = r.Validate(); err != nil {
			return err
		}
		path, input = "/workspace/command", r
	}
	method := "GET"
	if input != nil {
		method = "POST"
	}
	b, err := call(ctx, o, method, path, input)
	if err == nil {
		_, err = fmt.Fprintln(out, string(b))
	}
	return err
}
