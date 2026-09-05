package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"heimdall/internal/continuity"
	"heimdall/internal/model"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
)

func continuityCLI(ctx context.Context, o options, verb string, args []string, out io.Writer) error {
	print := func(method, path string, input any) error {
		b, err := call(ctx, o, method, path, input)
		if err == nil {
			_, err = fmt.Fprintln(out, string(b))
		}
		return err
	}
	if verb == "backup" {
		f := flag.NewFlagSet("backup", flag.ContinueOnError)
		f.SetOutput(io.Discard)
		dest := f.String("output", "", "new database snapshot file")
		if err := f.Parse(args); err != nil {
			return err
		}
		if *dest == "" || f.NArg() != 0 {
			return fmt.Errorf("backup requires --output FILE")
		}
		abs, err := filepath.Abs(*dest)
		if err != nil {
			return err
		}
		return print("POST", "/continuity/backup", map[string]string{"path": abs})
	}
	if verb == "context" {
		if len(args) == 0 {
			return fmt.Errorf("context requires TARGET [--budget N]")
		}
		target := args[0]
		f := flag.NewFlagSet("context", flag.ContinueOnError)
		f.SetOutput(io.Discard)
		budget := f.Int("budget", 16000, "mandatory context estimate budget")
		if err := f.Parse(args[1:]); err != nil {
			return err
		}
		if f.NArg() != 0 {
			return fmt.Errorf("unexpected arguments")
		}
		return print("GET", "/continuity/context?target="+url.QueryEscape(target)+"&budget="+strconv.Itoa(*budget), nil)
	}
	if len(args) < 2 {
		return fmt.Errorf("%s requires ACTION TARGET", verb)
	}
	action, target := args[0], args[1]
	rest := args[2:]
	if action == "list" || action == "show" {
		selectedID := ""
		if verb == "checkpoint" && action == "show" {
			f := flag.NewFlagSet("checkpoint show", flag.ContinueOnError)
			f.SetOutput(io.Discard)
			f.StringVar(&selectedID, "id", "", "specific checkpoint ID; default is current head")
			if err := f.Parse(rest); err != nil {
				return err
			}
			if f.NArg() != 0 || (selectedID != "" && !model.OpaqueID.MatchString(selectedID)) {
				return fmt.Errorf("invalid checkpoint show arguments")
			}
		} else if len(rest) != 0 {
			return fmt.Errorf("unexpected arguments")
		}
		raw, err := call(ctx, o, "GET", "/continuity/state?target="+url.QueryEscape(target), nil)
		if err != nil {
			return err
		}
		var v continuity.View
		if err = json.Unmarshal(raw, &v); err != nil {
			return err
		}
		var result any = v
		switch verb {
		case "checkpoint":
			if action == "show" {
				if selectedID == "" {
					selectedID = v.CheckpointHead
				}
				found := false
				for _, c := range v.Checkpoints {
					if c.ID == selectedID {
						result = c
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("checkpoint not found for target")
				}
			} else {
				result = v.Checkpoints
			}
		case "contract":
			if action == "list" {
				result = v.Contracts
			} else {
				if v.ContractHead == "" {
					return fmt.Errorf("no accepted contract")
				}
				for _, c := range v.Contracts {
					if c.ID == v.ContractHead {
						result = c
						break
					}
				}
			}
		case "resource":
			result = v.Resources
		case "decision":
			result = v.Decisions
		}
		return json.NewEncoder(out).Encode(result)
	}
	f := flag.NewFlagSet(verb+" "+action, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	file := f.String("file", "", "JSON operation input")
	revision := f.Int64("expected-task-revision", 0, "required observed task revision")
	resourceID := f.String("id", "", "resource ID to unbind")
	if err := f.Parse(rest); err != nil {
		return err
	}
	if f.NArg() != 0 || *revision < 1 {
		return fmt.Errorf("--expected-task-revision N is required; inspect state TARGET")
	}
	req := continuity.Request{Version: 1, ID: o.requestID, Target: target, ExpectedTaskRevision: revision}
	if req.ID == "" {
		req.ID = model.NewID()
	}
	if verb == "resource" && action == "unbind" {
		if *file != "" {
			return fmt.Errorf("unbind does not take a file")
		}
		req.Op = "resource.unbind"
		req.ResourceID = *resourceID
	} else {
		if *file == "" || *resourceID != "" {
			return fmt.Errorf("--file FILE required")
		}
		input, err := os.Open(*file)
		if err != nil {
			return err
		}
		defer input.Close()
		body, err := io.ReadAll(io.LimitReader(input, continuity.MaxRequest+1))
		if err != nil {
			return err
		}
		if len(body) > continuity.MaxRequest {
			return fmt.Errorf("input exceeds 64 KiB")
		}
		switch {
		case verb == "contract" && action == "accept":
			req.Op = "contract.accept"
			req.Contract = &continuity.ContractInput{}
			err = model.StrictJSON(body, req.Contract)
		case verb == "decision" && action == "accept":
			req.Op = "decision.accept"
			req.Decision = &continuity.DecisionInput{}
			err = model.StrictJSON(body, req.Decision)
		case verb == "resource" && action == "bind":
			req.Op = "resource.bind"
			req.Resource = &continuity.ResourceInput{}
			err = model.StrictJSON(body, req.Resource)
			if err == nil && req.Resource.Root == "" {
				err = fmt.Errorf("resource root required")
			}
			if err == nil {
				req.Resource.Root, err = filepath.Abs(req.Resource.Root)
			}
		case verb == "checkpoint" && action == "create":
			req.Op = "checkpoint.record"
			req.Checkpoint = &continuity.CheckpointInput{}
			err = model.StrictJSON(body, req.Checkpoint)
		default:
			return fmt.Errorf("unsupported %s action", verb)
		}
		if err != nil {
			return err
		}
	}
	if err := req.Validate(); err != nil {
		return err
	}
	return print("POST", "/continuity/command", req)
}
