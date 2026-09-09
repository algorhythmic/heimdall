package main

import (
	"context"
	"flag"
	"fmt"
	"heimdall/internal/tui"
	"io"
	"os"

	"github.com/gdamore/tcell/v2"
)

// ui is a compatibility spelling for the terminal interface, not a browser session.
func uiCLI(ctx context.Context, o options, args []string, out io.Writer) error {
	target := ""
	if len(args) > 0 && len(args[0]) > 0 && args[0][0] != '-' {
		target = args[0]
		args = args[1:]
	}
	f := flag.NewFlagSet("tui", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	snapshot := f.Bool("snapshot", false, "print the terminal layout once without ANSI")
	compact := f.Bool("compact", false, "compact needs-you and workstream overview")
	width := f.Int("width", 140, "snapshot columns")
	height := f.Int("height", 44, "snapshot rows")
	request := f.String("request", "", "inspect and explicitly retry a retained TUI request file")
	// Existing invocations remain usable; local terminal commands use CLI authority.
	f.Bool("progress-review", false, "legacy option; terminal review is available directly")
	if err := f.Parse(args); err != nil {
		return err
	}
	if f.NArg() != 0 {
		return fmt.Errorf("tui [TARGET] [--compact] [--snapshot] [--request FILE]")
	}
	clock, err := nowFunc(o.now)
	if err != nil {
		return err
	}
	opts := tui.Options{Target: target, DataDir: o.dir, Compact: *compact, Now: clock}
	caller := func(ctx context.Context, method, path string, body any) ([]byte, error) {
		return call(ctx, o, method, path, body)
	}
	if *snapshot {
		if *width < 45 || *width > 300 || *height < 14 || *height > 150 {
			return fmt.Errorf("snapshot requires width 45–300 and height 14–150")
		}
		text, err := tui.Snapshot(ctx, caller, opts, *width, *height)
		if err != nil {
			return err
		}
		_, err = io.WriteString(out, text)
		return err
	}
	if out != os.Stdout {
		return fmt.Errorf("TUI requires a terminal; use --snapshot for output capture")
	}
	screen, err := tcell.NewScreen()
	if err != nil {
		return err
	}
	if err = screen.Init(); err != nil {
		return fmt.Errorf("interactive terminal required; use tui --snapshot: %w", err)
	}
	defer screen.Fini()
	app := tui.New(screen, caller, opts)
	if *request != "" {
		if err = app.OpenRequest(*request); err != nil {
			return err
		}
	}
	return app.Run(ctx)
}
