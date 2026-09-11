package tui

import (
	"context"
	"fmt"
	"heimdall/internal/browser"
	"heimdall/internal/model"
	"sort"
	"strings"
	"time"

	"github.com/rivo/uniseg"
)

type observedRow struct {
	container  model.ObservedSurfaceContainer
	surface    model.ObservedSurface
	hasSurface bool
	focus      *model.SurfaceFocusSpan
	historical bool
}

type activeRead struct {
	State browser.ActiveState
	At    time.Time
}

// observedRows joins a task only through the same reviewed open ownership used
// by browser.selectActive. The content catalog alone never implies a task link.
func observedRows(st model.State, target string) []observedRow {
	rows := []observedRow{}
	for _, container := range st.SurfaceContainers {
		o := container.Observation
		if observedOwner(st, o) != target {
			continue
		}
		p, exists := st.Browsers[o.Profile]
		row := observedRow{container: container, historical: !exists || !p.Paired || p.Epoch != o.Epoch}
		if o.SurfaceID != "" {
			row.surface, row.hasSurface = st.ObservedSurfaces[o.SurfaceID]
		}
		if f, ok := st.SurfaceFocusSpans[o.Profile]; ok && f.Profile == o.Profile && f.Epoch == o.Epoch && f.TabID == o.TabID {
			span := f
			row.focus = &span
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool {
		left, right := rows[i].container.Observation, rows[j].container.Observation
		if left.Profile != right.Profile {
			return left.Profile < right.Profile
		}
		if left.Epoch != right.Epoch {
			return left.Epoch < right.Epoch
		}
		return left.TabID < right.TabID
	})
	return rows
}

func observedOwner(st model.State, o model.BrowserSurfaceObservation) string {
	if p, ok := st.Browsers[o.Profile]; ok && p.Epoch == o.Epoch {
		for _, tab := range p.Tabs {
			if tab.ID == o.TabID {
				if target := openOwner(st, tab.OwnerID, o.Profile, o.Epoch); target != "" {
					return target
				}
			}
		}
	}
	// Once a profile has a new epoch, only an accepted open report can retain the
	// exact old tab identity. Without that evidence, the container is unbound.
	for id, action := range st.Actions {
		b := action.Intent.Browser
		if b == nil || action.Intent.ID != id || b.Action != "open" || b.Profile != o.Profile || b.Epoch != o.Epoch ||
			action.Report == nil || action.Report.Status != "succeeded" || action.Report.TabID != o.TabID {
			continue
		}
		if target := openOwner(st, id, o.Profile, o.Epoch); target != "" {
			return target
		}
	}
	return ""
}

func openOwner(st model.State, owner, profile, epoch string) string {
	a, ok := st.Actions[owner]
	if !ok {
		return ""
	}
	b := a.Intent.Browser
	m := st.WorkspaceManifests[st.WorkspaceHeads[a.Intent.Target]]
	task, exists := st.Tasks[a.Intent.Target]
	if !exists || b == nil || b.Action != "open" || b.Profile != profile || b.Epoch != epoch || a.DeliveryID == "" ||
		m.ID == "" || m.ID != a.Intent.ManifestID || m.Target != task.Task.ID || m.TaskRevision != task.Revision || a.Intent.TaskRevision != task.Revision {
		return ""
	}
	for _, desired := range m.Surfaces {
		if desired.ID == a.Intent.SurfaceID && desired.Kind == "browser" {
			return task.Task.ID
		}
	}
	return ""
}

func (a *App) observedLines(target string) []line {
	rows := observedRows(a.data.State, target)
	out := []line{plain(""), line{{"observed   ", gray}, {"browser surfaces related by exact recorded tab ownership", gold}}}
	if len(rows) == 0 {
		return append(out, line{{"observed   ", gray}, {"No task-owned browser observations", gray}})
	}
	for _, row := range rows {
		o := row.container.Observation
		kind, pointer := "browser", ""
		if o.Gap != "" {
			pointer = "gap " + o.Gap
		} else if row.hasSurface {
			kind, pointer = row.surface.Kind, row.surface.NormalizedPointer
		} else {
			pointer = "gap surface_identity_unavailable"
		}
		state := "current"
		if row.historical {
			state = "historical"
		}
		presence := "last observed"
		if row.container.Present {
			presence = "present"
		}
		out = append(out,
			line{{"surface    ", gray}, {kind + " · " + bounded(pointer, 96), fg}},
			line{{"title      ", gray}, {bounded(o.Title, 96), fg}},
			line{{"observed   ", gray}, {presence + " " + age(o.ObservedAt, a.now()) + " · epoch " + bounded(o.Epoch, 24) + " · " + state, checkColor(state)}},
		)
		if row.focus == nil {
			out = append(out, line{{"tier       ", gray}, {"0 existence", gray}})
			continue
		}
		f := row.focus
		out = append(out,
			line{{"tier       ", gray}, {"1 attention", green}},
			line{{"focus      ", gray}, {age(f.StartedAt, a.now()) + " ago · " + duration(f.DurationSeconds), green}},
		)
	}
	return out
}

// conversationLines shows provider conversations explicitly bound to the task.
// Description text is evidence-side; the panel shows kind/digest/lifecycle only.
func (a *App) conversationLines(target string) []line {
	convs := []model.Conversation{}
	for _, c := range a.data.State.Conversations {
		if c.Task != nil && c.Task.Target == target {
			convs = append(convs, c)
		}
	}
	sort.Slice(convs, func(i, j int) bool { return convs[i].StartedAt.After(convs[j].StartedAt) })
	out := []line{line{{"conv       ", gray}, {"provider conversations bound to this task", gold}}}
	if len(convs) == 0 {
		return append(out, line{{"conv       ", gray}, {"No bound conversations", gray}})
	}
	for i, c := range convs {
		if i >= 6 {
			out = append(out, line{{"conv       ", gray}, {fmt.Sprintf("+%d more", len(convs)-6), gray}})
			break
		}
		lifecycle := c.Lifecycle(a.now())
		color := checkColor(map[string]string{"ended": "matched", "resumed": "matched", "started": "unknown", "inactive-by-policy": "stale"}[lifecycle])
		desc := "no description"
		if d := c.CurrentDescription; d != nil {
			desc = d.Kind + " " + d.Availability + " " + bounded(d.RetainedDigest, 12)
			if d.Availability == "withdrawn" {
				desc = d.Kind + " withdrawn"
			}
		}
		out = append(out, line{{"conv       ", gray},
			{c.Kind + " " + lifecycle, color}, {" · " + desc + " · started " + age(c.StartedAt, a.now()), fg}})
	}
	return out
}

// sensorLines reports degraded sensors only; healthy sensors stay silent.
func (a *App) sensorLines() []line {
	out := []line{}
	names := []string{}
	for name, s := range a.data.State.SensorHealth {
		if s.Status == "degraded" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	for _, name := range names {
		s := a.data.State.SensorHealth[name]
		out = append(out, line{{"sensor     ", red}, {name + " degraded: " + s.Reason + " · " + age(s.At, a.now()), red}})
	}
	return out
}

func (a *App) attentionLines() []line {
	profiles := []string{}
	for id, p := range a.data.State.Browsers {
		if p.Paired {
			if _, ok := a.data.State.SurfaceFocusSpans[id]; ok {
				profiles = append(profiles, id)
			}
		}
	}
	sort.Strings(profiles)
	if len(profiles) == 0 {
		return []line{line{{"attention  ", gray}, {"No recorded browser focus spans · g reads active now", gray}}}
	}
	out := []line{}
	for _, id := range profiles {
		f := a.data.State.SurfaceFocusSpans[id]
		p := a.data.State.Browsers[id]
		state := "current"
		if f.Epoch != p.Epoch {
			state = "historical"
		}
		out = append(out, line{{"attention  ", gray}, {"browser " + bounded(id, 16) + " · tab " + fmt.Sprint(f.TabID) + " · focus " + age(f.StartedAt, a.now()) + " ago · " + duration(f.DurationSeconds) + " · " + state + " · g active read", checkColor(state)}})
	}
	return out
}

func duration(seconds float64) string { return fmt.Sprintf("%ds", int(seconds)) }

func bounded(s string, width int) string {
	s = safe(s)
	if uniseg.StringWidth(s) <= width {
		return s
	}
	var out strings.Builder
	used := 0
	g := uniseg.NewGraphemes(s)
	for g.Next() {
		if used+g.Width()+3 > width {
			break
		}
		out.WriteString(g.Str())
		used += g.Width()
	}
	return out.String() + "..."
}

func (a *App) openActive() {
	d := a.newDialog("active", "browser attention · active read", "")
	d.lines = []line{plain("On-demand browser read; it does not poll or bind a task.")}
	d.busy = true
	a.async("active", "", d.generation, func(ctx context.Context) (any, error) {
		raw, err := a.call(ctx, "GET", "/state?active=1", nil)
		if err != nil {
			return nil, err
		}
		var state browser.ActiveState
		if err := model.StrictJSON(raw, &state); err != nil {
			return nil, err
		}
		if !model.Contains([]string{"active", "unbound", "ambiguous", "unknown", "none"}, state.Status) {
			return nil, fmt.Errorf("invalid active status %q", state.Status)
		}
		return activeRead{State: state, At: a.now()}, nil
	})
}

func (a *App) activeLines(v activeRead) []line {
	color := checkColor(v.State.Status)
	if v.State.Status == "active" {
		color = green
	}
	out := []line{line{{"status     ", gray}, {v.State.Status, color}}, line{{"read age   ", gray}, {age(v.At, a.now()), gray}}}
	if f := v.State.Focus; f != nil {
		locator := "profile " + bounded(f.Profile, 24) + " · epoch " + bounded(f.Epoch, 24) + " · tab " + fmt.Sprint(f.TabID) + " · window " + fmt.Sprint(f.WindowID)
		if f.SurfaceID != "" {
			locator += " · surface " + bounded(f.SurfaceID, 24)
		}
		out = append(out, line{{"locator    ", gray}, {locator, fg}})
	}
	if v.State.Target != "" {
		out = append(out, line{{"task       ", gray}, {v.State.Target, green}})
	}
	if len(v.State.Gaps) == 0 {
		return append(out, line{{"gaps       ", gray}, {"none", gray}})
	}
	for _, gap := range v.State.Gaps {
		out = append(out, line{{"gap        ", red}, {bounded(gap.Profile, 24) + " · " + bounded(gap.Reason, 64), red}})
	}
	return out
}

func (a *App) activeUnavailable(err error) {
	a.modal.lines = []line{plain("On-demand browser read; it does not poll or bind a task."), line{{"gap        ", red}, {"daemon_unavailable", red}}}
	a.modal.err = err.Error()
}
