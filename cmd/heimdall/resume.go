package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"heimdall/internal/continuity"
	"io"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"
)

func resumeCLI(ctx context.Context, o options, args []string, out io.Writer) error {
	target, tail, err := positional(args)
	if err != nil {
		return fmt.Errorf("resume requires TARGET [--budget N] [--json]")
	}
	f := flag.NewFlagSet("resume", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	budget := f.Int("budget", 16000, "mandatory context estimate budget")
	if err = f.Parse(tail); err != nil {
		return err
	}
	if f.NArg() != 0 || *budget < 0 {
		return fmt.Errorf("resume requires one target and a nonnegative budget")
	}
	raw, err := call(ctx, o, "GET", "/continuity/resume?target="+url.QueryEscape(target)+"&budget="+strconv.Itoa(*budget), nil)
	if err != nil {
		return err
	}
	if o.json {
		_, err = fmt.Fprintln(out, string(raw))
		return err
	}
	var view continuity.ResumeView
	if err = json.Unmarshal(raw, &view); err != nil {
		return err
	}
	clock, err := nowFunc(o.now)
	if err != nil {
		return err
	}
	return renderResume(out, view, clock())
}

// Escape terminal control and Unicode format characters, including OSC, ANSI,
// carriage return and bidi controls. All task-derived text goes through here.
func terminalText(value string) string {
	var out strings.Builder
	for _, r := range value {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			if r < 256 {
				fmt.Fprintf(&out, "\\x%02x", r)
			} else {
				fmt.Fprintf(&out, "\\u%04x", r)
			}
		} else {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func checkpointAge(at, now time.Time) string {
	if at.After(now) {
		return "in the future; check clock"
	}
	d := now.Sub(at)
	if d < time.Minute {
		return "less than a minute ago"
	}
	if d < time.Hour {
		return fmt.Sprintf("%d minutes ago", int(d/time.Minute))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%d hours ago", int(d/time.Hour))
	}
	return fmt.Sprintf("%d days ago", int(d/(24*time.Hour)))
}

func renderResume(out io.Writer, v continuity.ResumeView, now time.Time) error {
	var b strings.Builder
	line := func(label, value string) { fmt.Fprintf(&b, "%s%s\n", label, terminalText(value)) }
	line("Task: ", v.Task.Task.Title+" ("+v.Target+")")
	status := v.Task.Task.Status
	if _, step, ok := strings.Cut(v.Target, "#"); ok {
		for _, s := range v.Task.Task.Subtasks {
			if s.ID == step {
				line("Step: ", s.Title)
				status = s.Status
			}
		}
	}
	line("Status: ", status+" | Resume: "+strings.ReplaceAll(v.ResumeStatus, "_", " "))
	for _, parent := range v.Ancestors {
		line("Parent: ", parent.Task.Title+" ("+parent.Task.ID+")")
	}
	b.WriteString("\nAccepted direction\n")
	if len(v.Contracts) == 0 {
		b.WriteString("  No accepted contract.\n")
	}
	for _, c := range v.Contracts {
		line("  ", c.Target+": "+c.Objective)
		if c.Acceptance.Text != "" {
			line("  Acceptance: ", c.Acceptance.Text)
		}
		for _, check := range c.Acceptance.Checks {
			line("  Check: ", check.ID+" ("+check.Kind+")")
		}
		for _, constraint := range c.Constraints {
			line("  Constraint: ", constraint)
		}
	}
	for _, d := range v.Decisions {
		line("  Decision: ", d.Text)
	}
	if len(v.Progress) > 0 {
		b.WriteString("\nProgress review (separate from task completion)\n")
		for _, p := range v.Progress {
			line("  ", p.Kind+" "+p.ID+": "+p.Status+" ("+p.Freshness+")")
			line("    ", p.Text)
		}
	}
	b.WriteString("\nSaved progress\n")
	if cp := v.Checkpoint; cp != nil {
		line("  Checkpoint: ", cp.At.UTC().Format(time.RFC3339)+" ("+checkpointAge(cp.At, now)+")")
		line("  Summary: ", cp.Summary)
		if cp.CurrentStep != "" {
			line("  Current step: ", cp.CurrentStep)
		}
		line("  Next action (checkpoint): ", cp.NextAction)
		for _, blocker := range cp.Blockers {
			line("  Blocker: ", blocker)
		}
	} else {
		b.WriteString("  No saved checkpoint.\n")
		if v.Task.Task.NextAction != "" {
			line("  Next action (task): ", v.Task.Task.NextAction)
		}
	}
	resourceIDs := map[string]bool{}
	if len(v.Resources) > 0 {
		b.WriteString("\nResources\n")
		for _, r := range v.Resources {
			resourceIDs[r.Resource.ID] = true
			state := r.Status
			if r.Expected == nil && r.Observed != nil {
				state = "observed; no checkpoint baseline"
			}
			line("  ", filepath.Join(r.Resource.Root, r.Resource.Path)+": "+state)
			if r.Detail != "" {
				line("    ", r.Detail)
			}
		}
	}
	if len(v.Artifacts) > 0 {
		b.WriteString("\nPinned artifacts\n")
		for _, a := range v.Artifacts {
			line("  ", a.Artifact.Name+": "+a.Status)
			line("    Version: ", a.Record.ID)
			line("    SHA-256: ", a.Record.Observation.Digest)
		}
	}
	if len(v.Issues) > 0 {
		b.WriteString("\nNeeds attention\n")
		for _, issue := range v.Issues {
			if issue.Target == v.Target || resourceIDs[issue.Target] {
				line("  ", issue.Detail)
			} else {
				line("  ", issue.Target+": "+issue.Detail)
			}
		}
	}
	b.WriteString("\nReview\n")
	if v.Reviews == (continuity.ReviewSummary{}) {
		b.WriteString("  No recorded pending proposals or evidence issues.\n")
	} else {
		fmt.Fprintf(&b, "  Pending completion proposals: %d\n", v.Reviews.PendingProposals)
		fmt.Fprintf(&b, "  Evidence: %d failed, %d stale, %d unknown/in progress.\n", v.Reviews.FailedEvidence, v.Reviews.StaleEvidence, v.Reviews.UnknownEvidence)
	}
	b.WriteString("  Evidence is revalidated when completion is accepted.\n")
	_, err := io.WriteString(out, b.String())
	return err
}
