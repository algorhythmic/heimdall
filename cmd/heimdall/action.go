package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"heimdall/internal/actions"
	"heimdall/internal/model"
	"io"
	"net/url"
	"os"
	"strconv"
)

func actionCLI(ctx context.Context, o options, args []string, out io.Writer) error {
	if len(args) < 2 || !model.ValidID(args[1]) {
		return fmt.Errorf("action requires context|queue|cancel|reconcile|show|list|history TASK")
	}
	action, target := args[0], args[1]
	f := flag.NewFlagSet("action "+action, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	var file, id string
	var before int64
	limit := 25
	if action == "queue" || action == "cancel" || action == "reconcile" {
		f.StringVar(&file, "file", "", "retained complete JSON request")
	}
	if action == "show" || action == "history" {
		f.StringVar(&id, "id", "", "action ID")
	}
	if action == "list" || action == "history" {
		f.Int64Var(&before, "before", 0, "event cursor")
		f.IntVar(&limit, "limit", 25, "page size 1..100")
	}
	if err := f.Parse(args[2:]); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected action arguments")
	}
	if !model.Contains([]string{"context", "queue", "cancel", "reconcile", "show", "list", "history"}, action) {
		return fmt.Errorf("unknown action verb")
	}
	method := "GET"
	q := url.Values{"target": {target}}
	var input any
	if action == "show" || action == "history" {
		if !model.OpaqueID.MatchString(id) {
			return fmt.Errorf("--id ACTION_ID required")
		}
		q.Set("id", id)
	}
	if action == "list" || action == "history" {
		if before < 0 || limit < 1 || limit > 100 {
			return fmt.Errorf("invalid cursor/limit")
		}
		q.Set("before", strconv.FormatInt(before, 10))
		q.Set("limit", strconv.Itoa(limit))
	}
	path := "/action/" + action + "?" + q.Encode()
	if action == "queue" || action == "cancel" || action == "reconcile" {
		if file == "" {
			return fmt.Errorf("--file REQUEST.json required")
		}
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()
		raw, err := io.ReadAll(io.LimitReader(f, actions.MaxRequest+1))
		if err != nil {
			return err
		}
		if len(raw) > actions.MaxRequest {
			return fmt.Errorf("action input exceeds 64 KiB")
		}
		var requestID, requestTarget string
		if action == "queue" {
			r, err := actions.Decode(raw)
			if err != nil {
				return err
			}
			requestID, requestTarget, input = r.ID, r.Target, r
		} else {
			var r actions.CancelRequest
			if err := model.StrictJSON(raw, &r); err != nil {
				return err
			}
			requestID, requestTarget, input = r.ID, r.Target, r
		}
		if requestTarget != target || (o.requestID != "" && requestID != o.requestID) {
			return fmt.Errorf("saved action target/request ID differs from command")
		}
		method, path = "POST", "/action/"+action
	}
	raw, err := call(ctx, o, method, path, input)
	if err != nil {
		return err
	}
	var pretty any
	if err = json.Unmarshal(raw, &pretty); err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(pretty)
}
