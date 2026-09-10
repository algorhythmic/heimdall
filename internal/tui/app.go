package tui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/gdamore/tcell/v2"
)

type result struct {
	kind       string
	target     string
	generation int
	value      any
	err        error
}
type App struct {
	screen                                tcell.Screen
	call                                  Call
	opts                                  Options
	ctx                                   context.Context
	results                               chan result
	workers                               sync.WaitGroup
	data                                  snapshot
	selected, query, message, connection  string
	expanded                              map[string]bool
	panel, needIndex, detailOffset        int
	searching, loading, quitting, pasting bool
	modal                                 *dialog
	generation                            int
}

func New(screen tcell.Screen, call Call, opts Options) *App {
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &App{screen: screen, call: call, opts: opts, selected: opts.Target, expanded: map[string]bool{rootOf(opts.Target): true}, results: make(chan result, 16), connection: "connecting", panel: 1}
}
func (a *App) now() time.Time { return a.opts.Now() }
func Run(ctx context.Context, call Call, opts Options) error {
	screen, err := tcell.NewScreen()
	if err != nil {
		return fmt.Errorf("open terminal: %w", err)
	}
	if err = screen.Init(); err != nil {
		return fmt.Errorf("an interactive terminal is required (use --snapshot for plain text): %w", err)
	}
	defer screen.Fini()
	return New(screen, call, opts).Run(ctx)
}
func (a *App) Run(ctx context.Context) error {
	a.ctx = ctx
	runCtx, cancel := context.WithCancel(ctx)
	a.ctx = runCtx
	defer func() { cancel(); a.workers.Wait() }()
	a.screen.SetStyle(base)
	a.screen.EnablePaste()
	a.screen.HideCursor()
	events := make(chan tcell.Event, 16)
	go a.screen.ChannelEvents(events, runCtx.Done())
	tick := time.NewTicker(5 * time.Second)
	defer tick.Stop()
	a.refresh()
	a.Draw()
	for !a.quitting {
		select {
		case <-runCtx.Done():
			return nil
		case event, ok := <-events:
			if !ok {
				return nil
			}
			switch e := event.(type) {
			case *tcell.EventResize:
				a.screen.Sync()
			case *tcell.EventPaste:
				a.pasting = e.Start()
			case *tcell.EventKey:
				a.key(e)
			}
		case r := <-a.results:
			a.apply(r)
		case <-tick.C:
			a.refresh()
		}
		a.Draw()
	}
	return nil
}
func (a *App) async(kind, target string, generation int, work func(context.Context) (any, error)) {
	a.workers.Add(1)
	go func() {
		defer a.workers.Done()
		ctx, cancel := context.WithTimeout(a.ctx, 12*time.Second)
		defer cancel()
		v, err := work(ctx)
		select {
		case a.results <- result{kind, target, generation, v, err}:
		case <-a.ctx.Done():
		}
	}()
}
func (a *App) refresh() {
	if a.loading || a.call == nil || a.ctx == nil {
		return
	}
	a.loading = true
	target := a.selected
	a.async("snapshot", target, 0, func(ctx context.Context) (any, error) { return fetch(ctx, a.call, target) })
}
func (a *App) apply(r result) {
	if r.kind == "snapshot" {
		a.loading = false
		if r.err != nil {
			a.connection = "unavailable"
			a.message = "Daemon unavailable · start heimdall with the same --data-dir · r retry"
			return
		}
		incoming := r.value.(snapshot)
		// State remains useful after selection changes; context belongs only to its target.
		a.data = incoming
		a.connection = "ok"
		if strings.HasPrefix(a.message, "Daemon unavailable") || strings.HasPrefix(a.message, "Refreshing") {
			a.message = ""
		}
		a.reconcile()
		if incoming.Target != a.selected {
			a.data.Resume = nil
			a.refresh()
		}
		return
	}
	if a.modal == nil || a.modal.generation != r.generation {
		return
	}
	a.modal.busy = false
	if r.err != nil {
		if a.modal.kind == "active" {
			a.activeUnavailable(r.err)
			return
		}
		a.modal.err = r.err.Error()
		return
	}
	a.applyDialog(r)
}
func (a *App) key(e *tcell.EventKey) {
	if e.Key() == tcell.KeyCtrlC {
		a.quitting = true
		return
	}
	if a.pasting {
		if a.modal != nil {
			a.editKey(e)
		} else if a.searching && e.Key() == tcell.KeyRune {
			a.query += string(e.Rune())
		}
		return
	}
	if a.modal != nil {
		a.dialogKey(e)
		return
	}
	if a.searching {
		switch e.Key() {
		case tcell.KeyEscape:
			a.searching = false
			a.query = ""
			a.reconcile()
		case tcell.KeyEnter:
			a.searching = false
			a.reconcile()
			a.refresh()
		case tcell.KeyBackspace, tcell.KeyBackspace2:
			a.query = backspace(a.query)
			a.reconcile()
		case tcell.KeyRune:
			if len(a.query) < 256 {
				a.query += string(e.Rune())
				a.reconcile()
			}
		}
		return
	}
	switch e.Key() {
	case tcell.KeyTab:
		a.panel = (a.panel + 1) % a.panelCount()
		return
	case tcell.KeyBacktab:
		a.panel = (a.panel + a.panelCount() - 1) % a.panelCount()
		return
	case tcell.KeyUp:
		a.move(-1)
		return
	case tcell.KeyDown:
		a.move(1)
		return
	case tcell.KeyPgUp:
		a.move(-8)
		return
	case tcell.KeyPgDn:
		a.move(8)
		return
	case tcell.KeyEnter:
		a.openSelected()
		return
	case tcell.KeyEscape:
		a.query = ""
		a.reconcile()
		return
	}
	if e.Key() != tcell.KeyRune {
		return
	}
	switch e.Rune() {
	case 'q':
		a.quitting = true
	case 'j':
		a.move(1)
	case 'k':
		a.move(-1)
	case ' ':
		id := rootOf(a.selected)
		a.expanded[id] = !a.expanded[id]
		a.reconcile()
	case '/':
		a.searching = true
	case 'r':
		a.message = "Refreshing recorded state and selected files…"
		a.refresh()
	case 'g':
		a.openActive()
	case 'c':
		a.openDraft(a.actionTarget())
	case 'f':
		a.openFiles(a.actionTarget())
	case 'p':
		a.openWorkspace(a.actionTarget())
	case 'b':
		a.openBind(a.actionTarget())
	case 'a', 'x':
		a.openSelected() // inspection and an explicit dialog action precede mutation
	case '?':
		a.openHelp()
	case 'v':
		a.opts.Compact = !a.opts.Compact
	}
}
func (a *App) actionTarget() string {
	if a.panel == 0 {
		ns := a.needs()
		if len(ns) > a.needIndex && ns[a.needIndex].Target != "" {
			return ns[a.needIndex].Target
		}
	}
	return a.selected
}
func (a *App) move(delta int) {
	switch a.panel {
	case 0:
		a.needIndex = max(0, min(len(a.needs())-1, a.needIndex+delta))
	case 1:
		rows := a.rows()
		i := 0
		for n, r := range rows {
			if r.Target == a.selected {
				i = n
				break
			}
		}
		if len(rows) > 0 {
			a.choose(rows[max(0, min(len(rows)-1, i+delta))].Target)
		}
	case 2:
		a.detailOffset = max(0, a.detailOffset+delta)
	}
}
func backspace(s string) string {
	r := []rune(s)
	if len(r) > 0 {
		r = r[:len(r)-1]
	}
	return string(r)
}

func (a *App) panelCount() int {
	w, h := a.screen.Size()
	if a.opts.Compact || w < 90 || h < 28 {
		return 2
	}
	return 3
}
