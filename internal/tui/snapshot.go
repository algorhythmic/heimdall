package tui

import (
	"context"
	"fmt"
	"heimdall/internal/model"

	"github.com/gdamore/tcell/v2"
)

func Snapshot(ctx context.Context, call Call, opts Options, width, height int) (string, error) {
	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		return "", err
	}
	defer screen.Fini()
	screen.SetSize(width, height)
	app := New(screen, call, opts)
	v, err := fetch(ctx, call, opts.Target)
	if err != nil {
		return "", fmt.Errorf("daemon unavailable; start heimdall with this --data-dir: %w", err)
	}
	if opts.Target != "" {
		if _, _, err := model.ResolveTarget(v.State, opts.Target); err != nil {
			return "", err
		}
	}
	app.data = v
	app.connection = "ok"
	app.reconcile()
	if app.selected != "" && app.selected != v.Target {
		v, err = fetch(ctx, call, app.selected)
		if err != nil {
			return "", err
		}
		app.data = v
	}
	if opts.Target != "" {
		app.expanded[rootOf(opts.Target)] = true
	}
	app.Draw()
	return app.Text(), nil
}
