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

func preservationCLI(ctx context.Context, o options, args []string, out io.Writer) error {
	if len(args) < 2 {
		return fmt.Errorf("preservation preview|request|observe|show|list|export TARGET requires explicit target")
	}
	action, target := args[0], args[1]
	f := flag.NewFlagSet("preservation "+action, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	file := f.String("file", "", "versioned request envelope")
	id := f.String("id", "", "plan ID")
	checkpoint := f.String("checkpoint", "", "checkpoint ID to export")
	after := f.String("after", "", "page after plan ID")
	limit := f.Int("limit", 25, "page size 1..50")
	if err := f.Parse(args[2:]); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected preservation arguments")
	}
	method, path := "GET", "/preservation/"+action
	var body any
	if action == "preview" || action == "request" || action == "observe" {
		if *file == "" || *id != "" || *checkpoint != "" || *after != "" || *limit != 25 {
			return fmt.Errorf("preview/request/observe require only --file REQUEST.json")
		}
		in, err := os.Open(*file)
		if err != nil {
			return err
		}
		defer in.Close()
		raw, err := io.ReadAll(io.LimitReader(in, continuity.MaxRequest+1))
		if err != nil {
			return err
		}
		r, err := continuity.DecodePreservation(raw)
		if err != nil {
			return err
		}
		if r.Target != target || (o.requestID != "" && o.requestID != r.ID) {
			return fmt.Errorf("explicit target/request ID differs from saved envelope")
		}
		if (action == "observe") != (r.Observe != nil) {
			return fmt.Errorf("payload does not match preservation action")
		}
		method, body = "POST", r
		if action != "preview" {
			path = "/preservation/command"
		}
	} else {
		if *file != "" || o.requestID != "" {
			return fmt.Errorf("reads do not take request input")
		}
		switch action {
		case "show":
			if !model.OpaqueID.MatchString(*id) || *checkpoint != "" || *after != "" || *limit != 25 {
				return fmt.Errorf("show requires --id PLAN_ID")
			}
		case "list":
			if *id != "" || *checkpoint != "" || *limit < 1 || *limit > 50 || (*after != "" && !model.OpaqueID.MatchString(*after)) {
				return fmt.Errorf("list takes --limit 1..50 and optional --after ID")
			}
		case "export":
			if !model.OpaqueID.MatchString(*checkpoint) || *id != "" || *after != "" || *limit != 25 {
				return fmt.Errorf("export requires --checkpoint CHECKPOINT_ID")
			}
		default:
			return fmt.Errorf("unknown preservation action")
		}
		path += "?" + url.Values{"target": {target}, "id": {*id}, "checkpoint": {*checkpoint}, "after": {*after}, "limit": {strconv.Itoa(*limit)}}.Encode()
	}
	result, err := call(ctx, o, method, path, body)
	if err == nil {
		_, err = fmt.Fprintln(out, string(result))
	}
	return err
}
