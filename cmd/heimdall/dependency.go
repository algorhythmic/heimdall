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

func dependencyCLI(ctx context.Context, o options, args []string, out io.Writer) error {
	if len(args) < 2 {
		return fmt.Errorf("dependency add|remove|list|show TARGET requires explicit target")
	}
	action, target := args[0], args[1]
	f := flag.NewFlagSet("dependency "+action, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	file := f.String("file", "", "versioned dependency request")
	id := f.String("id", "", "historical record ID")
	if err := f.Parse(args[2:]); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected dependency arguments")
	}
	method, path := "GET", "/dependency/"+action
	var body any
	if action == "add" || action == "remove" {
		if *file == "" || *id != "" {
			return fmt.Errorf("dependency mutation requires --file REQUEST.json")
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
		r, err := continuity.DecodeDependency(raw)
		if err != nil {
			return err
		}
		if r.Target != target || r.Op != action || (o.requestID != "" && o.requestID != r.ID) {
			return fmt.Errorf("command differs from saved dependency envelope")
		}
		method, path, body = "POST", "/dependency/command", r
	} else {
		if *file != "" || o.requestID != "" || !model.ValidID(target) {
			return fmt.Errorf("dependency read requires task target and no mutation input")
		}
		if (action != "show" && action != "list") || (action == "show" && !model.OpaqueID.MatchString(*id)) || (action == "list" && *id != "") {
			return fmt.Errorf("list takes a target; show also requires --id RECORD_ID")
		}
		path += "?" + url.Values{"target": {target}, "id": {*id}}.Encode()
	}
	raw, err := call(ctx, o, method, path, body)
	if err == nil {
		_, err = fmt.Fprintln(out, string(raw))
	}
	return err
}
func summaryCLI(ctx context.Context, o options, args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("progress summary TASK|--all requires explicit scope")
	}
	target := args[0]
	if target == "--all" {
		target = "*"
	}
	f := flag.NewFlagSet("progress summary", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	subtree := f.Bool("subtree", false, "include descendants")
	sort := f.String("sort", "checkpoint", "sort by checkpoint, due or id")
	limit := f.Int("limit", 25, "page size 1..50")
	cursor := f.String("cursor", "", "page cursor")
	if err := f.Parse(args[1:]); err != nil {
		return err
	}
	if f.NArg() != 0 || o.requestID != "" {
		return fmt.Errorf("summary accepts only read options")
	}
	if err := (continuity.SummaryOptions{Target: target, Subtree: *subtree, Sort: *sort, Limit: *limit, Cursor: *cursor}).Validate(); err != nil {
		return err
	}
	q := url.Values{"target": {target}, "subtree": {strconv.FormatBool(*subtree)}, "sort": {*sort}, "limit": {strconv.Itoa(*limit)}, "cursor": {*cursor}}
	raw, err := call(ctx, o, "GET", "/progress/summary?"+q.Encode(), nil)
	if err == nil {
		_, err = fmt.Fprintln(out, string(raw))
	}
	return err
}
