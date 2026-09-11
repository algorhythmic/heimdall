package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"heimdall/internal/conversation"
	"io"
	"strings"
)

func sourceCLI(ctx context.Context, o options, args []string, out io.Writer) error {
	if len(args) == 0 {
		return fmt.Errorf("source add --provider PROVIDER --root PATH | source list [--json]")
	}
	switch args[0] {
	case "add":
		fs := flag.NewFlagSet("source add", flag.ContinueOnError)
		provider := fs.String("provider", "", "provider (claude_code|codex)")
		root := fs.String("root", "", "native source root directory")
		if err := fs.Parse(args[1:]); err != nil || *provider == "" || *root == "" || fs.NArg() != 0 {
			return fmt.Errorf("source add --provider claude_code|codex --root PATH")
		}
		raw, err := call(ctx, o, "POST", "/session/source", map[string]any{"version": 1, "provider": *provider, "root": *root})
		if err != nil {
			return err
		}
		if o.json {
			_, err = out.Write(append(raw, '\n'))
			return err
		}
		var receipt map[string]string
		if err := json.Unmarshal(raw, &receipt); err != nil {
			return err
		}
		fmt.Fprintf(out, "Source root %s configured; discovered streams register on the next poll.\n", receipt["source_root_id"])
		return nil
	case "list":
		raw, err := call(ctx, o, "GET", "/session/sources", nil)
		if err != nil {
			return err
		}
		var payload struct {
			Roots   []conversation.SourceRoot `json:"roots"`
			Streams []conversation.Source     `json:"streams"`
		}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return err
		}
		if o.json {
			return json.NewEncoder(out).Encode(payload)
		}
		var b strings.Builder
		if len(payload.Roots) == 0 {
			b.WriteString("No configured source roots. `heimdall source add --provider PROVIDER --root PATH`.\n")
		}
		for _, r := range payload.Roots {
			state := "active"
			if !r.Active {
				state = "inactive"
			}
			fmt.Fprintf(&b, "%s  %s  %s  %s\n", r.ID, r.Provider, state, r.Root)
		}
		for _, s := range payload.Streams {
			state := "active"
			if !s.Active {
				state = "lost: " + s.Lost
			}
			cp := fmt.Sprintf("%d@%d", s.Checkpoint.Ordinal, s.Checkpoint.Offset)
			if s.Checkpoint.HasGaps {
				cp += " gaps"
			}
			fmt.Fprintf(&b, "  %s  %s %s  %s  %s\n", s.NativeID, s.Provider, state, cp, s.Path)
		}
		_, err = io.WriteString(out, b.String())
		return err
	default:
		return fmt.Errorf("source add --provider PROVIDER --root PATH | source list [--json]")
	}
}
