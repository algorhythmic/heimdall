package main

import (
	"context"
	"flag"
	"fmt"
	"heimdall/internal/continuity"
	"heimdall/internal/model"
	"io"
	"net/url"
	"os"
)

func artifactCLI(ctx context.Context, o options, args []string, out io.Writer) error {
	if len(args) < 2 {
		return fmt.Errorf("artifact record|list|show|check TARGET requires explicit target")
	}
	action, target := args[0], args[1]
	f := flag.NewFlagSet("artifact "+action, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	file := f.String("file", "", "JSON artifact selectors")
	revision := f.Int64("expected-task-revision", 0, "observed task revision")
	id := f.String("id", "", "explicit artifact ID")
	version := f.String("version", "", "explicit historic version; default current")
	if err := f.Parse(args[2:]); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected artifact arguments")
	}
	method, path := "GET", "/artifact/"+action
	var body any
	if action == "record" {
		if *file == "" || *revision < 1 || *id != "" || *version != "" {
			return fmt.Errorf("record requires --file and --expected-task-revision")
		}
		input, err := os.Open(*file)
		if err != nil {
			return err
		}
		defer input.Close()
		raw, err := io.ReadAll(io.LimitReader(input, continuity.MaxRequest+1))
		if err != nil {
			return err
		}
		if len(raw) > continuity.MaxRequest {
			return fmt.Errorf("artifact input exceeds 64 KiB")
		}
		r := continuity.ArtifactRequest{Version: 1, ID: o.requestID, Target: target, ExpectedTaskRevision: *revision}
		if r.ID == "" {
			r.ID = model.NewID()
		}
		if err = model.StrictJSON(raw, &r.Artifact); err != nil {
			return err
		}
		if err = r.Validate(); err != nil {
			return err
		}
		method, body = "POST", r
	} else {
		if *file != "" || *revision != 0 {
			return fmt.Errorf("read operations do not take write preconditions")
		}
		if action == "list" {
			if *id != "" || *version != "" {
				return fmt.Errorf("list takes only target")
			}
		} else if action == "show" || action == "check" {
			if !model.OpaqueID.MatchString(*id) || (*version != "" && !model.OpaqueID.MatchString(*version)) {
				return fmt.Errorf("explicit artifact ID and valid version required")
			}
		} else {
			return fmt.Errorf("unknown artifact action")
		}
		path += "?" + url.Values{"target": {target}, "id": {*id}, "version": {*version}}.Encode()
	}
	result, err := call(ctx, o, method, path, body)
	if err == nil {
		_, err = fmt.Fprintln(out, string(result))
	}
	return err
}
