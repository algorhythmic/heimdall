package tui

import (
	"fmt"
	"heimdall/internal/continuity"
	"heimdall/internal/model"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/gdamore/tcell/v2"
	"github.com/rivo/uniseg"
)

var (
	bg         = tcell.NewHexColor(0x0e1012)
	fg         = tcell.NewHexColor(0xd4cfc3)
	gray       = tcell.NewHexColor(0x78818e)
	gold       = tcell.NewHexColor(0xe5a727)
	green      = tcell.NewHexColor(0x80ac68)
	red        = tcell.NewHexColor(0xd36b55)
	border     = tcell.NewHexColor(0x394048)
	selectedBG = tcell.NewHexColor(0x1d232a)
	base       = tcell.StyleDefault.Background(bg).Foreground(fg)
)

type span struct {
	text  string
	color tcell.Color
}
type line []span

func plain(s string) line { return line{{s, fg}} }
func safe(s string) string {
	var b strings.Builder
	for _, r := range s {
		if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			fmt.Fprintf(&b, "\\u%04x", r)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
func (a *App) text(x, y, width int, s string, style tcell.Style) int {
	if width <= 0 {
		return x
	}
	end := x + width
	g := uniseg.NewGraphemes(safe(s))
	for g.Next() {
		rs := g.Runes()
		n := g.Width()
		if n == 0 {
			continue
		}
		if x+n > end {
			break
		}
		a.screen.SetContent(x, y, rs[0], rs[1:], style)
		x += n
	}
	return x
}
func (a *App) line(x, y, w int, parts line, style tcell.Style) {
	end := x + w
	for _, p := range parts {
		x = a.text(x, y, end-x, p.text, style.Foreground(p.color))
		if x >= end {
			break
		}
	}
}
func (a *App) fill(x, y, w, h int, style tcell.Style) {
	for row := y; row < y+h; row++ {
		for col := x; col < x+w; col++ {
			a.screen.SetContent(col, row, ' ', nil, style)
		}
	}
}
func (a *App) box(x, y, w, h int, title, subtitle string, color tcell.Color) {
	if w < 2 || h < 2 {
		return
	}
	s := base.Foreground(color)
	for i := 1; i < w-1; i++ {
		a.screen.SetContent(x+i, y, '─', nil, s)
		a.screen.SetContent(x+i, y+h-1, '─', nil, s)
	}
	for i := 1; i < h-1; i++ {
		a.screen.SetContent(x, y+i, '│', nil, s)
		a.screen.SetContent(x+w-1, y+i, '│', nil, s)
	}
	for _, p := range []struct {
		x, y int
		r    rune
	}{{x, y, '╭'}, {x + w - 1, y, '╮'}, {x, y + h - 1, '╰'}, {x + w - 1, y + h - 1, '╯'}} {
		a.screen.SetContent(p.x, p.y, p.r, nil, s)
	}
	a.text(x+2, y, w-4, " "+title+" ", base.Foreground(gold).Bold(true))
	sw := uniseg.StringWidth(safe(subtitle)) + 2
	if sw < w-uniseg.StringWidth(safe(title))-8 {
		a.text(x+w-sw-2, y, sw, " "+subtitle+" ", base.Foreground(gray))
	}
}
func (a *App) Draw() {
	w, h := a.screen.Size()
	a.screen.Clear()
	a.screen.HideCursor()
	if w < 45 || h < 14 {
		a.text(1, 1, w-2, "heimdall · enlarge terminal to 45 × 14", base.Foreground(gold))
		a.text(1, 3, w-2, "q quit", base)
		a.screen.Show()
		return
	}
	if a.opts.Compact || w < 90 || h < 28 {
		a.drawCompact(w, h)
	} else {
		a.drawDashboard(w, h)
	}
	if a.modal != nil {
		a.drawDialog(w, h)
	}
	a.screen.Show()
}
func (a *App) header(w int) {
	color := green
	if a.connection != "ok" {
		color = red
	}
	bound := 0
	spaces := map[string]bool{}
	for id, head := range a.data.State.SessionHeads {
		b := a.data.State.SessionBindings[head]
		if !b.Active || !a.inScope(b.Target) || id != b.SurfaceID {
			continue
		}
		bound++
		if b.Locator != nil {
			spaces[b.Locator.WorkspaceID] = true
		}
	}
	working, blockedA, idle, other := 0, 0, 0, 0
	agents := a.agents()
	for _, r := range agents {
		switch r.Status {
		case "working":
			working++
		case "blocked":
			blockedA++
		case "idle":
			idle++
		default:
			other++
		}
	}
	parts := line{{"heimdall", gold}, {"   daemon ", gray}, {a.connection, color}, {"   refreshed ", gray}, {age(a.data.At, a.now()), fg}}
	if a.loading {
		parts = append(parts, span{" · loading", gold})
	}
	if bound > 0 {
		parts = append(parts, span{fmt.Sprintf("   herdr %d recorded spaces · %d bindings · %d agents", len(spaces), bound, len(agents)), gray})
		if len(agents) > 0 {
			parts = append(parts, span{fmt.Sprintf("   %d● %d? %d○", working, blockedA, idle), fg})
			if other > 0 {
				parts = append(parts, span{fmt.Sprintf(" %d·", other), gray})
			}
		}
	} else {
		parts = append(parts, span{"   herdr · no bound sessions", gray})
	}
	a.line(2, 1, w-4, parts, base)
}
func (a *App) drawDashboard(w, h int) {
	a.header(w)
	ns := a.needs()
	nh := min(10, max(4, len(ns)+2))
	dh := min(11, max(8, h/5))
	wy := 3 + nh + 1
	wh := h - wy - dh - 3
	if wh < 6 {
		nh = 4
		wy = 8
		wh = h - wy - dh - 3
	}
	c := border
	if a.panel == 0 {
		c = red
	}
	a.box(2, 3, w-4, nh, "needs you", fmt.Sprintf("%d · enter inspect", len(ns)), c)
	start := max(0, a.needIndex-(nh-3))
	if len(ns) == 0 {
		a.text(4, 5, w-8, "All clear. No proposals or unsaved workstreams need attention.", base.Foreground(gray))
	}
	for i := start; i < len(ns) && i-start < nh-2; i++ {
		n := ns[i]
		y := 4 + i - start
		style := base
		if a.panel == 0 && i == a.needIndex {
			style = style.Background(selectedBG)
			a.fill(3, y, w-6, 1, style)
		}
		color := gold
		action := "↵ inspect"
		if n.Kind == "completion" || n.Kind == "decision" || n.Kind == "artifact" {
			color = red
			action = "↵ review"
		}
		if n.Kind == "unsaved" {
			action = "c save progress"
		}
		if n.Kind == "agent" {
			color = red
			action = "j jump · inspect"
		}
		if n.Kind == "uncertain" {
			color = gold
			action = "r re-observe"
		}
		if n.Kind == "unbound" {
			action = "b bind"
		}
		if n.Kind == "link" {
			action = "↵ file link"
		}
		a.text(4, y, 11, n.Kind, style.Foreground(color))
		x := 15
		a.text(x, y, w-36, n.Target+" · "+n.Text, style)
		a.text(w-21, y, 16, action, style.Foreground(gold))
	}
	c = border
	if a.panel == 1 {
		c = gold
	}
	a.box(2, wy, w-4, wh, "workstreams", "by resume-by, then saved", c)
	a.drawRows(4, wy+1, w-8, wh-2)
	c = border
	if a.panel == 2 {
		c = gold
	}
	dy := wy + wh + 1
	a.box(2, dy, w-4, dh, a.selected, "selected · enter for checks", c)
	a.drawLines(4, dy+1, w-8, dh-2, a.contextLines(), a.detailOffset)
	a.footer(w, h)
}
func (a *App) drawRows(x, y, w, h int) {
	rows := a.rows()
	if len(rows) == 0 {
		msg := "No workstreams. Add a task with heimdall add."
		if a.query != "" {
			msg = "No matching tasks or steps."
		}
		a.text(x, y+1, w, msg, base.Foreground(gray))
		return
	}
	wide := w >= 122
	xwide := w >= 140
	idw := 25
	statusw := 11
	nextw := w - idw - statusw - 10 - 7 - 13
	if wide {
		nextw -= 17
	}
	if xwide {
		nextw -= 10
	}
	colNext, colStatus := x+idw, x+idw+nextw
	colBy, colSaved, colSteps := colStatus+statusw, colStatus+statusw+10, colStatus+statusw+17
	colAgents := colSteps + 31
	for _, p := range []struct {
		x, w int
		s    string
	}{{x, idw, "id"}, {colNext, nextw, "next action"}, {colStatus, statusw, "status"}, {colBy, 10, "by"}, {colSaved, 7, "saved"}, {colSteps, 13, "steps"}} {
		a.text(p.x, y, p.w, p.s, base.Foreground(gray))
	}
	if wide {
		a.text(colSteps+13, y, 17, "sessions", base.Foreground(gray))
	}
	if xwide {
		a.text(colAgents, y, 10, "agents", base.Foreground(gray))
	}
	idx := 0
	for i, r := range rows {
		if r.Target == a.selected {
			idx = i
		}
	}
	start := max(0, idx-max(0, h-2))
	for i := start; i < len(rows) && i-start < h-1; i++ {
		r := rows[i]
		yy := y + 1 + i - start
		st := a.data.State
		t := st.Tasks[rootOf(r.Target)]
		style := base
		if r.Target == a.selected {
			style = style.Background(selectedBG)
			a.fill(x-1, yy, w+2, 1, style)
		}
		if r.Step {
			_, s, err := model.ResolveTarget(st, r.Target)
			if err != nil {
				continue
			}
			symbol, color := "○", gray
			if s.Status == "done" {
				symbol, color = "✓", green
			}
			if s.Status == "active" {
				symbol, color = "▸", gold
			}
			cp := st.Checkpoints[st.CheckpointHeads[t.Task.ID]]
			label := s.Status
			if cp.CurrentStep == s.ID {
				symbol, color, label = "▸", gold, "current"
			}
			a.text(x, yy, idw-1, strings.Repeat(" ", min(r.Depth*2, 12))+symbol+" "+s.ID, style.Foreground(color))
			a.text(colNext, yy, nextw-2, s.Title, style)
			a.text(colStatus, yy, w-(colStatus-x), label, style.Foreground(color))
			for _, p := range st.Proposals {
				if p.Target == r.Target && p.Status == "pending" {
					a.text(colStatus, yy, w-(colStatus-x), label+" · completion proposed", style.Foreground(red))
					break
				}
			}
			continue
		}
		arrow := "▸"
		if a.expanded[r.Target] {
			arrow = "▾"
		}
		name := strings.Repeat(" ", min(r.Depth*2, 12)) + arrow + " " + r.Target
		a.text(x, yy, idw-1, name, style)
		next, fresh := continuity.RecordedNextAction(st, r.Target)
		if next == "" {
			next = "—"
		}
		nc := fg
		if fresh == "checkpoint_needs_review" {
			nc = gold
			next = "review · " + next
		}
		a.text(colNext, yy, nextw-2, next, style.Foreground(nc))
		a.text(colStatus, yy, statusw-1, "● "+t.Task.Status, style.Foreground(statusColor(t.Task.Status)))
		by := t.Task.ResumeBy
		if by == "" {
			by = "—"
		}
		bc := gray
		if t.Task.ResumeBy != "" && t.Task.ResumeBy < a.now().Format("2006-01-02") {
			bc = red
		}
		a.text(colBy, yy, 10, by, style.Foreground(bc))
		cp := st.Checkpoints[st.CheckpointHeads[r.Target]]
		cc := fg
		if cp.At.IsZero() || a.now().Sub(cp.At) > 5*24*time.Hour {
			cc = gold
		}
		a.text(colSaved, yy, 7, age(cp.At, a.now()), style.Foreground(cc))
		done, total := 0, 0
		for _, s := range t.Task.Subtasks {
			if s.Status != "dropped" {
				total++
				if s.Status == "done" {
					done++
				}
			}
		}
		blocks := 0
		if total > 0 {
			blocks = done * 8 / total
		}
		a.text(colSteps, yy, 8, strings.Repeat("▰", blocks), style.Foreground(green))
		a.text(colSteps+blocks, yy, 8-blocks, strings.Repeat("▰", 8-blocks), style.Foreground(border))
		a.text(colSteps+9, yy, 4, fmt.Sprintf("%d/%d", done, total), style)
		if wide {
			count := 0
			for _, head := range st.SessionHeads {
				b := st.SessionBindings[head]
				if b.Target == r.Target && b.Active {
					count++
				}
			}
			value := "—"
			if count > 0 {
				value = fmt.Sprintf("%d recorded", count)
			}
			a.text(colSteps+13, yy, 17, value, style.Foreground(gray))
		}
		if xwide {
			dots := ""
			for _, r := range a.agentsForTask(r.Target) {
				symbol, color := "○", gray
				switch r.Status {
				case "working":
					symbol, color = "●", green
				case "blocked":
					symbol, color = "?", red
				case "done":
					symbol, color = "✓", green
				}
				a.text(colAgents+len([]rune(dots)), yy, 1, symbol, style.Foreground(color))
				dots += " "
			}
			if dots == "" {
				a.text(colAgents, yy, 10, "—", style.Foreground(gray))
			}
		}
	}
}
func statusColor(s string) tcell.Color {
	switch s {
	case "active", "done", "completed":
		return green
	case "waiting", "blocked":
		return gold
	case "dropped":
		return red
	}
	return gray
}
func (a *App) contextLines() []line {
	if a.selected == "" {
		return append([]line{plain("Select a workstream to inspect its saved context.")}, a.attentionLines()...)
	}
	v := a.data.Resume
	if v == nil || v.Target != a.selected {
		if a.data.DetailError != "" {
			return append([]line{{{"Context unavailable: " + a.data.DetailError, red}}}, a.attentionLines()...)
		}
		return append([]line{{{"Loading selected context…", gray}}}, a.attentionLines()...)
	}
	out := []line{}
	add := func(label, text string, color tcell.Color) {
		out = append(out, line{{fmt.Sprintf("%-11s", label), gray}, {text, color}})
	}
	direction := []string{}
	for _, c := range v.Contracts {
		direction = append(direction, c.Objective)
	}
	for _, d := range v.Decisions {
		direction = append(direction, d.Text)
	}
	if len(direction) == 0 {
		add("direction", "No accepted contract", gold)
	} else {
		add("direction", strings.Join(direction, " · "), fg)
	}
	if cp := v.Checkpoint; cp != nil {
		add("saved", age(cp.At, a.now())+" · "+cp.Summary, fg)
		add("next", cp.NextAction, gold)
		if len(cp.Blockers) > 0 {
			add("blockers", strings.Join(cp.Blockers, " · "), red)
		}
	} else {
		add("saved", "No checkpoint yet · c to save progress", gold)
		add("next", v.Task.Task.NextAction, fg)
	}
	files := line{{"files      ", gray}}
	deps := continuity.DependencyViews(a.data.State, rootOf(a.selected), func(string) bool { return true })
	for _, d := range deps {
		color := gold
		if d.Status == "satisfied" {
			color = green
		}
		add("depends", d.Title+" · "+d.Status, color)
	}
	if len(v.Resources) == 0 {
		files = append(files, span{"No bound resources", gray})
	}
	for i, r := range v.Resources {
		if i > 0 {
			files = append(files, span{" · ", gray})
		}
		files = append(files, span{filepath.Base(r.Resource.Path) + " ", fg}, span{r.Status, checkColor(r.Status)})
	}
	out = append(out, files)
	sessions := []string{}
	for _, head := range a.data.State.SessionHeads {
		b := a.data.State.SessionBindings[head]
		if b.Target == rootOf(a.selected) && b.Active && b.Locator != nil {
			sessions = append(sessions, b.Locator.WorkspaceID+":"+b.Locator.PaneID+" · recorded "+age(b.At, a.now()))
		}
	}
	if len(sessions) == 0 {
		sessions = append(sessions, "No bound sessions")
	}
	add("sessions", strings.Join(sessions, " · "), gray)
	agents := line{{"agents     ", gray}}
	related := a.agentsForTask(rootOf(a.selected))
	for i, r := range related {
		if i > 0 {
			agents = append(agents, span{" · ", gray})
		}
		symbol, color := "○", gray
		switch r.Status {
		case "working":
			symbol, color = "●", green
		case "blocked":
			symbol, color = "?", red
		case "done":
			symbol, color = "✓", green
		}
		_, bound := a.agentTask(r)
		label := r.WorkspaceID + ":" + r.PaneID
		marker := "unbound"
		if bound {
			marker = "bound"
		}
		agents = append(agents, span{symbol + " ", color}, span{label + " " + r.Agent + " · " + r.Status + " · " + marker, color})
	}
	if len(related) == 0 {
		agents = append(agents, span{"No observed agents", gray})
	}
	out = append(out, agents)
	for _, issue := range v.Issues {
		if issue.Code == "unresolved_progress" {
			continue
		}
		add("attention", issue.Detail, gold)
	}
	out = append(out, a.attentionLines()...)
	out = append(out, a.observedLines(rootOf(a.selected))...)
	return out
}
func checkColor(s string) tcell.Color {
	switch s {
	case "matched", "current", "accepted":
		return green
	case "missing", "rejected", "not_matched", "disconnected":
		return red
	case "changed", "unknown", "stale", "unsupported":
		return gold
	}
	return gray
}
func (a *App) drawLines(x, y, w, h int, lines []line, offset int) {
	wrapped := []line{}
	// Wrap per grapheme while retaining status colors; clipping never hides the rest forever.
	for _, l := range lines {
		current := line{}
		used := 0
		for _, p := range l {
			g := uniseg.NewGraphemes(safe(p.text))
			for g.Next() {
				width := g.Width()
				if used+width > w {
					wrapped = append(wrapped, current)
					current = line{}
					used = 0
				}
				current = append(current, span{g.Str(), p.color})
				used += width
			}
		}
		wrapped = append(wrapped, current)
	}
	if len(wrapped) == 0 {
		return
	}
	offset = min(max(0, offset), max(0, len(wrapped)-h))
	for i := offset; i < len(wrapped) && i-offset < h; i++ {
		a.line(x, y+i-offset, w, wrapped[i], base)
	}
	if len(wrapped) > h {
		a.text(x+w-9, y+h-1, 9, "↑↓ scroll", base.Foreground(gray))
	}
}
func (a *App) footer(w, h int) {
	if a.searching {
		a.text(2, h-2, w-4, "/ "+a.query+"_", base.Foreground(gold))
		return
	}
	if a.message != "" {
		a.text(2, h-3, w-4, a.message, base.Foreground(gray))
	}
	a.line(2, h-2, w-4, line{{"tab", gold}, {" panel  ", gray}, {"space", gold}, {" expand  ", gray}, {"↵", gold}, {" open  ", gray}, {"c", gold}, {" save  ", gray}, {"g", gold}, {" active  ", gray}, {"b", gold}, {" bind  ", gray}, {"f", gold}, {" files  ", gray}, {"p", gold}, {" desktop  ", gray}, {"r", gold}, {" refresh  ", gray}, {"/", gold}, {" find  ", gray}, {"?", gold}, {" help  ", gray}, {"q", gold}, {" quit", gray}}, base)
}
func (a *App) drawCompact(w, h int) {
	a.header(w)
	ns := a.needs()
	y := 4
	label := "NEEDS YOU"
	if a.panel == 0 {
		label = "▸ " + label
	}
	a.text(2, y, w-4, fmt.Sprintf("%s · %d", label, len(ns)), base.Foreground(gold))
	y++
	limit := min(len(ns), max(2, (h-12)/2))
	start := max(0, a.needIndex-limit+1)
	for i := start; i < len(ns) && i < start+limit; i++ {
		n := ns[i]
		style := base
		if a.panel == 0 && i == a.needIndex {
			style = style.Background(selectedBG)
		}
		a.text(2, y, w-4, "● "+n.Kind+" · "+n.Target+" · "+n.Text, style.Foreground(gold))
		y++
	}
	if len(ns) == 0 {
		a.text(2, y, w-4, "All clear", base.Foreground(gray))
		y++
	}
	y++
	label = "WORKSTREAMS"
	if a.panel == 1 {
		label = "▸ " + label
	}
	a.text(2, y, w-4, label+" · ↑↓ select · space expand", base.Foreground(gold))
	y++
	rows := a.rows()
	idx := 0
	for i, r := range rows {
		if r.Target == a.selected {
			idx = i
		}
	}
	room := max(1, h-y-4)
	start = max(0, idx-room+1)
	for i := start; i < len(rows) && y < h-4; i++ {
		r := rows[i]
		style := base
		if r.Target == a.selected {
			style = style.Background(selectedBG)
			a.fill(2, y, w-4, 1, style)
		}
		task, step, _ := model.ResolveTarget(a.data.State, r.Target)
		status := task.Task.Status
		if step != nil {
			status = step.Status
		}
		prefix := strings.Repeat(" ", min(12, r.Depth*2)) + "● "
		a.text(2, y, w-4, prefix+r.Target+" · "+status, style.Foreground(statusColor(status)))
		y++
	}
	if a.message != "" {
		a.text(2, h-3, w-4, a.message, base.Foreground(gray))
	}
	a.text(2, h-2, w-4, "tab panel  ↵ open  c save  / find  ? help  q quit", base.Foreground(gray))
	if a.searching {
		a.text(2, h-2, w-4, "/ "+a.query+"_", base.Foreground(gold))
	}
}
