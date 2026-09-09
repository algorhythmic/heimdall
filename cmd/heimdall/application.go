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

func applicationCLI(ctx context.Context, o options, args []string, out io.Writer) error {
	if len(args) < 2 || !model.ValidID(args[1]) {
		return fmt.Errorf("application requires review|show TASK")
	}
	action, target := args[0], args[1]
	f := flag.NewFlagSet("application "+action, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	surface := f.String("surface", "", "logical surface ID")
	file := f.String("file", "", "reviewed recipe request JSON")
	if err := f.Parse(args[2:]); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected application arguments")
	}
	method, path := "GET", "/workspace/application/show?"+url.Values{"target": {target}, "surface": {*surface}}.Encode()
	var input any
	switch action {
	case "show":
		if !model.OpaqueID.MatchString(*surface) || *file != "" {
			return fmt.Errorf("show requires --surface ID")
		}
	case "review":
		if *file == "" || *surface != "" {
			return fmt.Errorf("review requires --file FILE containing explicit recipe pins")
		}
		h, err := os.Open(*file)
		if err != nil {
			return err
		}
		defer h.Close()
		raw, err := io.ReadAll(io.LimitReader(h, workspace.MaxRequest+1))
		if err != nil {
			return err
		}
		if len(raw) > workspace.MaxRequest {
			return fmt.Errorf("application request too large")
		}
		var r workspace.ApplicationRequest
		if err := model.StrictJSON(raw, &r); err != nil {
			return err
		}
		if r.Target != target {
			return fmt.Errorf("recipe target differs from explicit task")
		}
		if o.requestID != "" {
			if r.ID != "" && r.ID != o.requestID {
				return fmt.Errorf("request ID differs from reviewed file")
			}
			r.ID = o.requestID
		}
		if r.ID == "" {
			r.ID = model.NewID()
		}
		method, path, input = "POST", "/workspace/application/review", r
	default:
		return fmt.Errorf("application requires review|show")
	}
	b, err := call(ctx, o, method, path, input)
	if err == nil {
		_, err = fmt.Fprintln(out, string(b))
	}
	return err
}
