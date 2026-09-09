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
	"strconv"
)

func snapshotCLI(ctx context.Context, o options, args []string, out io.Writer) error {
	if len(args) < 2 || !model.ValidID(args[1]) {
		return fmt.Errorf("snapshot capture|policy|pin|unpin|prune|status|list|show TASK")
	}
	action, target := args[0], args[1]
	f := flag.NewFlagSet("snapshot "+action, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	file := f.String("file", "", "saved complete request envelope")
	id := f.String("id", "", "historical snapshot ID")
	before := f.Int64("before", 0, "older than event ID from prior page")
	limit := f.Int("limit", 25, "list size 1..256")
	if err := f.Parse(args[2:]); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected snapshot arguments")
	}
	method, path := "GET", "/workspace/snapshot/"+action
	var body any
	switch action {
	case "capture", "policy", "pin", "unpin", "prune":
		if *file == "" || *id != "" || *before != 0 || *limit != 25 {
			return fmt.Errorf("snapshot mutation requires only --file REQUEST.json")
		}
		in, err := os.Open(*file)
		if err != nil {
			return err
		}
		defer in.Close()
		raw, err := io.ReadAll(io.LimitReader(in, workspace.MaxRequest+1))
		if err != nil {
			return err
		}
		r, err := workspace.DecodeSnapshot(raw)
		if err != nil {
			return err
		}
		if r.Op != action || r.Target != target || (o.requestID != "" && o.requestID != r.ID) {
			return fmt.Errorf("command differs from saved snapshot envelope")
		}
		method, path, body = "POST", "/workspace/snapshot/command", r
	case "status", "list", "show":
		if *file != "" || o.requestID != "" || (*id != "") != (action == "show") || (*id != "" && !model.OpaqueID.MatchString(*id)) || (*before != 0 && action != "list") || (*limit != 25 && action != "list") || *before < 0 || *limit < 1 || *limit > 256 {
			return fmt.Errorf("invalid snapshot read arguments; show requires --id")
		}
		path += "?" + url.Values{"target": {target}, "id": {*id}, "before": {strconv.FormatInt(*before, 10)}, "limit": {strconv.Itoa(*limit)}}.Encode()
	default:
		return fmt.Errorf("unknown snapshot action")
	}
	raw, err := call(ctx, o, method, path, body)
	if err == nil {
		_, err = fmt.Fprintln(out, string(raw))
	}
	return err
}
