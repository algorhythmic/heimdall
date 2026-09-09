package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"heimdall/internal/workspace"
	"io"
	"os"
)

func recoveryCLI(ctx context.Context, o options, target string, args []string, out io.Writer) error {
	f := flag.NewFlagSet("workspace verify", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	r := workspace.RecoveryRequest{Version: 1, Target: target}
	f.StringVar(&r.OperationID, "operation", "", "verify the named operation's selected surfaces and original saved point")
	f.StringVar(&r.SnapshotID, "snapshot", "", "selected saved point; defaults to current head")
	f.StringVar(&r.PlacementPolicy, "placement-policy", "saved", "saved or named-monitor-clamp; observation only")
	f.StringVar(&r.FallbackMonitor, "fallback-monitor", "", "explicit named fallback monitor")
	output := f.String("output", "", "save point-in-time report to a new private JSON file")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("unexpected recovery verification arguments")
	}
	if err := r.Validate(); err != nil {
		return err
	}
	raw, err := call(ctx, o, "POST", "/workspace/verify", r)
	if err != nil {
		return err
	}
	if *output != "" {
		h, err := os.OpenFile(*output, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return err
		}
		_, err = h.Write(append(raw, '\n'))
		if err == nil {
			err = h.Sync()
		}
		closeErr := h.Close()
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
	var v workspace.RecoveryReport
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	fmt.Fprintf(out, "Recovery · %s · %s · full=%t\nAs of %s · expires %s\n", terminalText(target), v.Outcome, v.Full, v.AsOf.Format("15:04:05Z07:00"), v.ExpiresAt.Format("15:04:05Z07:00"))
	for _, row := range v.Surfaces {
		fmt.Fprintf(out, "%s · %s · expected %s · %s\n", terminalText(row.SurfaceID), terminalText(row.Label), row.Expected, row.Outcome)
		for _, c := range []struct {
			name  string
			check workspace.RecoveryCheck
		}{{"existence", row.Existence}, {"ownership", row.Ownership}, {"task", row.TaskMembership}, {"workspace", row.WorkspaceMembership}, {"placement", row.Placement}, {"window state", row.WindowState}, {"application", row.Application}, {"action", row.Action}} {
			if c.check.Status != "not_required" {
				fmt.Fprintf(out, "  %s: %s · %s\n", c.name, c.check.Status, terminalText(c.check.Reason))
			}
		}
	}
	fmt.Fprintf(out, "Coverage: %s · %s\nOperation: %s · %s\n", v.Coverage.Status, terminalText(v.Coverage.Reason), v.Operation.Status, terminalText(v.Operation.Reason))
	for _, issue := range v.Issues {
		fmt.Fprintln(out, terminalText(issue))
	}
	fmt.Fprintln(out, "Observation only; no moves, launches, closure, adoption or task completion.")
	return nil
}
