package tui

import (
	"fmt"
	"strings"

	"github.com/rivo/uniseg"
)

func tail(s string, w int) string {
	s = safe(s)
	for uniseg.StringWidth(s) > w && len(s) > 0 {
		g := uniseg.NewGraphemes(s)
		g.Next()
		s = s[len(g.Str()):]
	}
	return s
}
func (a *App) drawDialog(w, h int) {
	d := a.modal
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			r, c, s, _ := a.screen.GetContent(x, y)
			a.screen.SetContent(x, y, r, c, s.Foreground(border))
		}
	}
	mw := min(w-6, 108)
	mh := min(h-4, max(18, len(d.lines)+len(d.fields)+9))
	x, y := (w-mw)/2, (h-mh)/2
	a.fill(x, y, mw, mh, base)
	a.box(x, y, mw, mh, d.title, "esc close", gold)
	cy := y + 2
	if len(d.fields) > 0 {
		for i, f := range d.fields {
			style := base
			if d.field == i {
				style = style.Background(selectedBG)
			}
			a.text(x+2, cy, 15, f.label, base.Foreground(gray))
			a.fill(x+18, cy, mw-21, 1, style)
			value := f.value
			if d.field == i {
				value = tail(value, mw-23)
			}
			a.text(x+18, cy, mw-21, value, style)
			if d.field == i && !d.busy && d.pending == nil {
				a.screen.ShowCursor(x+18+min(uniseg.StringWidth(safe(value)), mw-22), cy)
			}
			cy++
		}
		cy++
	}
	bottom := y + mh - 3
	if d.err != "" {
		bottom -= 3
		a.drawLines(x+2, bottom, mw-4, 3, []line{{{d.err, red}}}, 0)
	}
	if d.pending != nil {
		bottom--
		a.text(x+2, bottom, mw-4, "Original request retained · enter retries unchanged · esc closes", base.Foreground(gold))
	}
	lines := d.lines
	if d.busy {
		lines = append(append([]line{}, lines...), line{{"Loading…", gold}})
	}
	a.drawLines(x+2, cy, mw-4, max(0, bottom-cy), lines, d.offset)
	hint := "↑↓ scroll   esc close"
	switch d.kind {
	case "step":
		hint = "a accept completion   x reject   f recheck   e run check   o reopen"
	case "progress":
		hint = "tab note   a accept   x reject   m reviewed   r recheck"
	case "draft":
		hint = "enter / ctrl-s save   tab field   e edit in $EDITOR   n new draft"
	case "workspace":
		hint = "r re-observe   b bind terminal   preview only"
	case "bind", "capture", "evaluate":
		hint = "enter / ctrl-s submit   tab next field   esc leave field"
	case "confirm":
		hint = "enter confirm   esc cancel"
	}
	if d.field >= 0 {
		hint = "typing · tab next field   esc actions   ctrl-s submit"
	}
	if d.busy {
		hint = "Request in progress…"
	}
	a.text(x+2, y+mh-2, mw-4, hint, base.Foreground(gold))
}

// Snapshot renders exactly the same cells as the interactive screen without ANSI.
func (a *App) Text() string {
	w, h := a.screen.Size()
	var out strings.Builder
	for y := 0; y < h; y++ {
		var b strings.Builder
		for x := 0; x < w; {
			r, c, _, width := a.screen.GetContent(x, y)
			if r == 0 {
				r = ' '
			}
			b.WriteRune(r)
			for _, v := range c {
				b.WriteRune(v)
			}
			x += max(1, width)
		}
		fmt.Fprintln(&out, strings.TrimRight(b.String(), " "))
	}
	return out.String()
}
