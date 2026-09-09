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

func viewportCLI(ctx context.Context, o options, args []string, out io.Writer) error {
	if len(args) < 1 {
		return fmt.Errorf("viewport probe|select|stop|status|inventory|bind|unbind|list")
	}
	action := args[0]
	args = args[1:]
	target := ""
	if model.Contains([]string{"bind", "unbind", "list"}, action) {
		if len(args) < 1 || !model.ValidID(args[0]) {
			return fmt.Errorf("explicit task target required")
		}
		target = args[0]
		args = args[1:]
	}
	f := flag.NewFlagSet("viewport "+action, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	file := f.String("file", "", "saved complete request envelope")
	dir := f.String("socket-dir", "", "explicit canonical compositor socket directory")
	cached := f.Bool("cached", false, "use cached scoped observation")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected viewport arguments")
	}
	method, path := "GET", "/workspace/viewport/"+action
	var body any
	switch action {
	case "select", "stop", "bind", "unbind":
		if *file == "" || *dir != "" || *cached {
			return fmt.Errorf("mutation requires --file REQUEST.json")
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
		r, err := workspace.DecodeViewport(raw)
		if err != nil {
			return err
		}
		if r.Op != action || r.Target != target || (o.requestID != "" && o.requestID != r.ID) {
			return fmt.Errorf("command differs from saved viewport request")
		}
		method, path, body = "POST", "/workspace/viewport/command", r
	case "probe", "status", "inventory", "list":
		if *file != "" || o.requestID != "" || (*dir != "") != (action == "probe") || (*cached && action != "list") {
			return fmt.Errorf("invalid viewport read options; probe requires --socket-dir")
		}
		q := url.Values{"target": {target}, "socket_dir": {*dir}}
		if *cached {
			q.Set("cached", "true")
		}
		path += "?" + q.Encode()
	default:
		return fmt.Errorf("unknown viewport action")
	}
	raw, err := call(ctx, o, method, path, body)
	if err == nil {
		_, err = fmt.Fprintln(out, string(raw))
	}
	return err
}
