package main

import (
	"context"
	"flag"
	"fmt"
	"heimdall/internal/model"
	"heimdall/internal/workspace"
	"io"
	"net/url"
	"path/filepath"
)

func herdrCLI(ctx context.Context, o options, action, target string, args []string, out io.Writer) error {
	f := flag.NewFlagSet("session "+action, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	surface := f.String("surface", "", "explicit logical surface ID")
	var input any
	path, method := "", "POST"
	id := o.requestID
	if id == "" {
		id = model.NewID()
	}
	if action == "bind-herdr" {
		r := workspace.HerdrBindRequest{Version: 1, ID: id, Target: target}
		f.StringVar(&r.ManifestID, "manifest", "", "current manifest ID")
		f.StringVar(&r.Previous, "previous", "", "previous binding ID or none")
		f.Int64Var(&r.ExpectedTaskRevision, "expected-task-revision", 0, "current task revision")
		f.StringVar(&r.Socket, "socket", "", "explicit local Herdr API socket")
		f.StringVar(&r.PaneID, "pane", "", "explicit actual Herdr pane ID")
		if err := f.Parse(args); err != nil {
			return err
		}
		if f.NArg() != 0 {
			return fmt.Errorf("unexpected arguments")
		}
		r.SurfaceID = *surface
		if r.Socket != "" {
			abs, err := filepath.Abs(r.Socket)
			if err != nil {
				return err
			}
			r.Socket = abs
		}
		if err := r.Validate(); err != nil {
			return err
		}
		input, path = r, "/workspace/herdr/bind"
	} else {
		binding := f.String("binding", "", "explicit binding ID (required for publish)")
		var summary bool
		if action == "publish" {
			f.BoolVar(&summary, "workspace-summary", false, "publish a summary naming only the selected pane")
		}
		if err := f.Parse(args); err != nil {
			return err
		}
		if f.NArg() != 0 || !model.OpaqueID.MatchString(*surface) || (*binding != "" && !model.OpaqueID.MatchString(*binding)) {
			return fmt.Errorf("explicit surface ID and valid binding ID required")
		}
		if action == "refresh" {
			q := url.Values{"target": {target}, "surface": {*surface}, "binding": {*binding}}
			method, path = "GET", "/workspace/herdr/refresh?"+q.Encode()
		} else {
			r := workspace.HerdrPublishRequest{Version: 1, ID: id, Target: target, SurfaceID: *surface, BindingID: *binding, WorkspaceSummary: summary}
			if err := r.Validate(); err != nil {
				return err
			}
			input, path = r, "/workspace/herdr/publish"
		}
	}
	b, err := call(ctx, o, method, path, input)
	if err == nil {
		_, err = fmt.Fprintln(out, string(b))
	}
	return err
}
