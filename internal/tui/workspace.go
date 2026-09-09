package tui

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"heimdall/internal/workspace"
	"net/url"
	"strings"
)

type workspaceDialogView struct {
	View     workspace.View
	Snapshot workspace.SnapshotStatus
	Lines    []line
}

func (a *App) workspaceView(ctx context.Context, target string) (any, error) {
	v := workspaceDialogView{}
	q := "?target=" + url.QueryEscape(target)
	for _, request := range []struct {
		path string
		dest any
	}{{"/workspace/state", &v.View}, {"/workspace/snapshot/status", &v.Snapshot}} {
		raw, err := a.call(ctx, "GET", request.path+q, nil)
		if err != nil {
			return nil, err
		}
		if err := json.Unmarshal(raw, request.dest); err != nil {
			return nil, err
		}
	}
	raw, err := a.call(ctx, "GET", "/workspace/diff"+q, nil)
	if err != nil {
		return nil, err
	}
	var preview workspace.Preview
	if err := json.Unmarshal(raw, &preview); err != nil {
		return nil, err
	}
	head := ""
	if v.Snapshot.Head != nil {
		head = v.Snapshot.Head.ID
	}
	if head != preview.Request.SnapshotID || v.Snapshot.InputDigest != preview.InputDigest || v.View.ManifestHead != preview.Request.ManifestID {
		return nil, fmt.Errorf("workspace changed while observing; press r to refresh")
	}
	v.Lines = workspacePreviewLines(preview, v.Snapshot)
	return v, nil
}

func workspacePreviewLines(p workspace.Preview, status workspace.SnapshotStatus) []line {
	lines := []line{line{{"Saved workspace · desired membership · observed now", gray}}, plain("")}
	if p.Request.ManifestID == "" {
		lines = append(lines, line{{"Accept a desired workspace with heimdall workspace accept.", gold}})
	}
	if p.Request.SnapshotID == "" {
		lines = append(lines, line{{"No saved point · s captures and pins the current owned workspace", gold}})
	} else {
		lines = append(lines, line{{"Saved point  " + p.Request.SnapshotID, fg}}, line{{fmt.Sprintf("Age %ds · protected %t · payload available %t", p.SnapshotAgeSeconds, p.SnapshotProtected, status.HeadAvailable), gray}})
	}
	policy := "off"
	if status.Policy != nil && status.Policy.Enabled {
		policy = fmt.Sprintf("%ds debounce · %ds maximum · retain %d", status.Policy.DebounceSeconds, status.Policy.MaxDirtySeconds, status.Policy.RetainCount)
	}
	lines = append(lines, line{{"Autosave     " + policy, gray}})
	if status.Capture.Issue != "" {
		lines = append(lines, line{{"Capture      " + strings.ReplaceAll(status.Capture.Issue, "_", " "), gold}})
	}
	fresh := "unavailable"
	color := gold
	if p.Fresh {
		fresh = "fresh double inventory"
		color = green
	}
	lines = append(lines, line{{"Observation  " + fresh, color}}, plain(""))
	for _, row := range p.Surfaces {
		color := gold
		if row.Disposition == "leave-open" {
			color = green
		}
		lines = append(lines, line{{row.Kind + " · " + row.Label + "  ", fg}, {row.Disposition, color}}, line{{row.SurfaceID, gray}})
		if row.Observed != nil {
			lines = append(lines, line{{"Now    " + row.Observed.Workspace + " · " + row.Observed.Monitor, gray}})
		}
		if row.Desired != nil {
			lines = append(lines, line{{"Saved  " + row.Desired.Workspace + " · " + row.Desired.Monitor, gray}})
		}
		if row.Kind == "terminal" {
			lines = append(lines, line{{"Session  " + row.SessionStatus + " · attachment requires verification", gray}})
		}
		for _, items := range [][]string{row.Changes, row.Issues, row.Requirements} {
			if len(items) > 0 {
				lines = append(lines, line{{strings.ReplaceAll(strings.Join(items, " · "), "_", " "), gold}})
			}
		}
		lines = append(lines, plain(""))
	}
	if len(p.Issues) > 0 {
		lines = append(lines, line{{strings.ReplaceAll(strings.Join(p.Issues, " · "), "_", " "), gold}})
	}
	lines = append(lines, line{{fmt.Sprintf("%d unowned windows in relevant workspaces left open", p.UnownedLeftOpen), gray}}, line{{"Preview only · application actions require a reviewed operation", gray}})
	return lines
}

func (a *App) confirmSnapshot(d *dialog) {
	status := d.snapshotStatus
	if status == nil || status.ManifestID == "" || status.SourceID == "" {
		d.err = "Select a desktop source and accept a manifest before saving a point."
		return
	}
	previous := "none"
	if status.Head != nil {
		previous = status.Head.ID
	}
	r := workspace.SnapshotRequest{Version: 1, ID: model.NewID(), Op: "capture", Target: d.target, Previous: previous, ExpectedTaskRevision: status.TaskRevision, ManifestID: status.ManifestID, SourceID: status.SourceID, InputDigest: status.InputDigest}
	raw, _ := json.Marshal(r)
	next := a.newDialog("confirm", "save workspace point · "+d.target, d.target)
	next.lines = []line{plain("Enter captures and pins fresh task-owned window layout."), plain("Partial coverage is retained without replacing the complete saved point."), plain("Escape cancels.")}
	next.pending = &savedRequest{1, d.target, "/workspace/snapshot/command", raw}
}
