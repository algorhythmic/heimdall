package main

import (
	"context"
	"flag"
	"fmt"
	"heimdall/internal/checks"
	"heimdall/internal/model"
	"io"
	"net/url"
	"os"
)

func evidenceCLI(ctx context.Context, o options, args []string, out io.Writer) error {
	if len(args) < 2 {
		return fmt.Errorf("evidence configure|evaluate|list|refresh TARGET [--file FILE] [--evaluator ID] --expected-task-revision N")
	}
	action, target := args[0], args[1]
	f := flag.NewFlagSet("evidence", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	file := f.String("file", "", "evaluator definition request JSON")
	evaluator := f.String("evaluator", "", "accepted evaluator ID")
	revision := f.Int64("expected-task-revision", 0, "current task revision")
	if err := f.Parse(args[2:]); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected evidence arguments")
	}
	req := checks.Request{}
	method, path := "POST", "/evidence/"+action
	switch action {
	case "list":
		if len(args) != 2 {
			return fmt.Errorf("list takes TARGET only")
		}
		method = "GET"
		path = "/evidence/state?target=" + url.QueryEscape(target)
	case "configure":
		if *file == "" || *evaluator != "" {
			return fmt.Errorf("configure requires --file")
		}
		input, err := os.Open(*file)
		if err != nil {
			return err
		}
		defer input.Close()
		body, err := io.ReadAll(io.LimitReader(input, 64<<10+1))
		if err != nil {
			return err
		}
		if len(body) > 64<<10 {
			return fmt.Errorf("definition too large")
		}
		if err = model.StrictJSON(body, &req); err != nil {
			return err
		}
	case "evaluate":
		if *evaluator == "" || *file != "" {
			return fmt.Errorf("evaluate requires --evaluator")
		}
		req.EvaluatorID = *evaluator
	case "refresh":
		if len(args) != 2 {
			return fmt.Errorf("refresh takes TARGET only")
		}
	default:
		return fmt.Errorf("unknown evidence action")
	}
	req.Version = 1
	req.Target = target
	req.ExpectedTaskRevision = *revision
	req.ID = o.requestID
	if req.ID == "" {
		req.ID = model.NewID()
	}
	var body any = req
	if method == "GET" {
		body = nil
	}
	raw, err := call(ctx, o, method, path, body)
	if err == nil {
		_, err = fmt.Fprintln(out, string(raw))
	}
	return err
}
