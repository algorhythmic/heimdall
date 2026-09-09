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
	"strconv"
)

func progressCLI(ctx context.Context, o options, args []string, out io.Writer) error {
	if len(args) > 0 && args[0] == "summary" {
		return summaryCLI(ctx, o, args[1:], out)
	}
	if len(args) < 2 {
		return fmt.Errorf("progress propose|review|list|show TARGET requires explicit target")
	}
	action, target := args[0], args[1]
	f := flag.NewFlagSet("progress "+action, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	file := f.String("file", "", "JSON proposal or review input")
	revision := f.Int64("expected-task-revision", 0, "observed task revision")
	id := f.String("id", "", "explicit proposal ID")
	after := f.String("after", "", "list after this proposal ID")
	limit := f.Int("limit", 25, "list page size, 1..50")
	if err := f.Parse(args[2:]); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected progress arguments")
	}
	method, path := "GET", "/progress/"+action
	var body any
	if action == "propose" || action == "review" {
		if *file == "" || *revision < 1 || *id != "" || *after != "" || *limit != 25 {
			return fmt.Errorf("propose/review require --file and --expected-task-revision")
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
			return fmt.Errorf("progress input exceeds 64 KiB")
		}
		r := continuity.ProgressRequest{Version: 1, ID: o.requestID, Target: target, ExpectedTaskRevision: *revision}
		if r.ID == "" {
			r.ID = model.NewID()
		}
		if action == "propose" {
			r.Proposal = &continuity.ProgressInput{}
			err = model.StrictJSON(raw, r.Proposal)
		} else {
			r.Review = &continuity.ProgressReviewInput{}
			err = model.StrictJSON(raw, r.Review)
		}
		if err != nil {
			return err
		}
		if err = r.Validate(); err != nil {
			return err
		}
		method, path, body = "POST", "/progress/command", r
	} else {
		if *file != "" || *revision != 0 {
			return fmt.Errorf("progress reads do not take write preconditions")
		}
		if action == "list" {
			if *id != "" || *limit < 1 || *limit > 50 || (*after != "" && !model.OpaqueID.MatchString(*after)) {
				return fmt.Errorf("list requires --limit 1..50 and optional --after ID")
			}
		} else if action == "show" {
			if !model.OpaqueID.MatchString(*id) || *after != "" || *limit != 25 {
				return fmt.Errorf("show requires --id PROPOSAL_ID")
			}
		} else {
			return fmt.Errorf("unknown progress action")
		}
		path += "?" + url.Values{"target": {target}, "id": {*id}, "after": {*after}, "limit": {strconv.Itoa(*limit)}}.Encode()
	}
	result, err := call(ctx, o, method, path, body)
	if err == nil {
		_, err = fmt.Fprintln(out, string(result))
	}
	return err
}
