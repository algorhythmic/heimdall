package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"heimdall/internal/model"
	"heimdall/internal/workspace"
	"io"
	"net/url"
	"os"
	"strings"
)

func previewCLI(ctx context.Context, o options, action, target string, args []string, out io.Writer) error {
	if !model.ValidID(target) {
		return fmt.Errorf("workspace requires an explicit task ID")
	}
	f := flag.NewFlagSet("workspace "+action, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	var manifest, point, file, output string
	var age int
	if action == "preview" {
		f.StringVar(&manifest, "manifest", "", "selected current manifest ID")
		f.StringVar(&point, "snapshot", "", "selected retained snapshot ID")
		f.IntVar(&age, "max-snapshot-age-seconds", 0, "optional maximum age; zero imposes no age limit")
	}
	if action == "validate" {
		f.StringVar(&file, "file", "", "complete preview JSON issued by this daemon")
	}
	if action == "preview" || action == "diff" {
		f.StringVar(&output, "output", "", "save complete preview to a new private file")
	}
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected preview arguments")
	}
	method, path := "GET", "/workspace/"+action+"?target="+url.QueryEscape(target)
	var input any
	if action == "preview" {
		r := workspace.PreviewRequest{Version: 1, Target: target, ManifestID: manifest, SnapshotID: point, MaxSnapshotAgeSeconds: age}
		if err := r.Validate(); err != nil {
			return err
		}
		input, method, path = r, "POST", "/workspace/preview"
	}
	if action == "validate" {
		if file == "" {
			return fmt.Errorf("validate requires --file PREVIEW.json")
		}
		f, err := os.Open(file)
		if err != nil {
			return err
		}
		defer f.Close()
		raw, err := io.ReadAll(io.LimitReader(f, workspace.PreviewMaxBytes+1))
		if err != nil {
			return err
		}
		if len(raw) > workspace.PreviewMaxBytes {
			return fmt.Errorf("preview exceeds 256 KiB")
		}
		var v workspace.Preview
		if err := model.StrictJSON(raw, &v); err != nil {
			return err
		}
		if v.Request.Target != target {
			return fmt.Errorf("preview belongs to another task")
		}
		input, method, path = v, "POST", "/workspace/validate"
	}
	raw, err := call(ctx, o, method, path, input)
	if err != nil {
		return err
	}
	if output != "" {
		f, err := os.OpenFile(output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		_, err = f.Write(append(raw, '\n'))
		if err == nil {
			err = f.Sync()
		}
		closeErr := f.Close()
		if err != nil {
			return err
		}
		if closeErr != nil {
			return closeErr
		}
	}
	if o.json {
		_, err = fmt.Fprintln(out, string(raw))
		return err
	}
	if action == "list" {
		var v workspace.WorkspaceList
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		fmt.Fprintf(out, "Workspace %s · fresh=%t\n", terminalText(v.Target), v.Fresh)
		for _, row := range v.Surfaces {
			fmt.Fprintf(out, "%s · %s: %s · session %s\n", terminalText(row.SurfaceID), terminalText(row.Label), terminalText(row.Status), terminalText(row.SessionStatus))
		}
		fmt.Fprintln(out, terminalText(strings.Join(v.Issues, ", ")))
		return nil
	}
	if action == "validate" {
		var v workspace.PreviewValidation
		if err := json.Unmarshal(raw, &v); err != nil {
			return err
		}
		fmt.Fprintf(out, "Preview current: %t · %s\nValidation grants no execution authority.\n", v.Current, terminalText(v.Issue))
		return nil
	}
	var v workspace.Preview
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	fmt.Fprintf(out, "Workspace preview · %s\nManifest %s · snapshot %s · age %ds\nFresh=%t · review required=%t · expires %s\n", terminalText(target), terminalText(v.Request.ManifestID), terminalText(v.Request.SnapshotID), v.SnapshotAgeSeconds, v.Fresh, v.ReviewRequired, v.ExpiresAt.Format("15:04:05Z07:00"))
	for _, row := range v.Surfaces {
		fmt.Fprintf(out, "%s · %s: %s\n", terminalText(row.SurfaceID), terminalText(row.Label), row.Disposition)
		if row.ApplicationRecipe != nil {
			fmt.Fprintln(out, "  Reviewed "+terminalText(model.ApplicationSummary(*row.ApplicationRecipe)))
		}
		for _, text := range []string{strings.Join(row.Changes, ", "), strings.Join(row.Issues, ", "), strings.Join(row.Requirements, ", ")} {
			if text != "" {
				fmt.Fprintln(out, "  "+terminalText(text))
			}
		}
	}
	if len(v.Issues) > 0 {
		fmt.Fprintln(out, terminalText(strings.Join(v.Issues, ", ")))
	}
	fmt.Fprintf(out, "%d unowned windows in relevant workspaces left open.\nPreview only; no application actions were dispatched.\n", v.UnownedLeftOpen)
	return nil
}
