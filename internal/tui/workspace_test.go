package tui

import (
	"encoding/json"
	"heimdall/internal/model"
	"heimdall/internal/workspace"
	"strings"
	"testing"
)

func TestWorkspacePreviewDialogAndFrozenCapture(t *testing.T) {
	f := newFixture(t)
	f.a.openWorkspace("alpha#outline")
	waitResult(t, f.a)
	d := f.a.modal
	if d.err != "" || d.target != "alpha" || d.snapshotStatus == nil {
		t.Fatal(d.err, d.target)
	}
	if text := flattenWorkspaceLines(d.lines); !strings.Contains(text, "No saved point") || !strings.Contains(text, "unavailable") {
		t.Fatal(text)
	}
	status := workspace.SnapshotStatus{Target: "alpha", TaskRevision: 4, ManifestID: model.NewID(), SourceID: model.NewID(), InputDigest: strings.Repeat("a", 64), Head: &model.WorkspacePoint{ID: model.NewID()}}
	d.snapshotStatus = &status
	f.a.confirmSnapshot(d)
	next := f.a.modal
	if next.kind != "confirm" || next.pending.Path != "/workspace/snapshot/command" {
		t.Fatal(next)
	}
	var r workspace.SnapshotRequest
	if err := json.Unmarshal(next.pending.Body, &r); err != nil {
		t.Fatal(err)
	}
	if err := r.Validate(); err != nil {
		t.Fatal(err)
	}
	if r.Target != "alpha" || r.Previous != status.Head.ID || r.InputDigest != status.InputDigest || r.ExpectedTaskRevision != 4 {
		t.Fatal(r)
	}
	before := string(next.pending.Body)
	d.snapshotStatus.TaskRevision++
	if string(next.pending.Body) != before {
		t.Fatal("capture request changed after review")
	}
	state, _ := f.e.Store.State(f.ctx)
	if len(state.SnapshotHeads) != 0 {
		t.Fatal("opening capture confirmation wrote state")
	}
}

func TestWorkspaceOperationConfirmationRetainsReviewedScope(t *testing.T) {
	f := newFixture(t)
	d := f.a.newDialog("workspace", "workspace", "alpha")
	d.workspacePreview = &workspace.Preview{Version: 1, Fresh: true, Request: workspace.PreviewRequest{Version: 1, Target: "alpha", ManifestID: model.NewID(), SnapshotID: model.NewID()}, TaskRevision: 4, Digest: strings.Repeat("a", 64), Token: strings.Repeat("b", 64), Surfaces: []workspace.PreviewSurface{{SurfaceID: model.NewID(), Membership: "desired", Label: "Owned view", ObservationStatus: "observed"}}}
	f.a.confirmWorkspaceOperation(d, "close")
	next := f.a.modal
	if next.pending == nil || next.pending.Path != "/workspace/operation/queue" {
		t.Fatal("review lacks frozen operation")
	}
	var r workspace.OperationRequest
	if err := json.Unmarshal(next.pending.Body, &r); err != nil {
		t.Fatal(err)
	}
	if r.Kind != "close" || r.Preview.Request.Target != "alpha" || r.Preview.TaskRevision != 4 || len(r.SurfaceIDs) != 1 {
		t.Fatal(r)
	}
	before := string(next.pending.Body)
	d.workspacePreview.TaskRevision++
	if string(next.pending.Body) != before {
		t.Fatal("confirmation followed mutable selection")
	}
	st, _ := f.e.Store.State(f.ctx)
	if len(st.WorkspaceOperations) != 0 {
		t.Fatal("opening confirmation authorized input")
	}
}

func TestWorkspacePreviewDialogShowsDispositionAndBoundaries(t *testing.T) {
	p := workspace.Preview{Fresh: true, Request: workspace.PreviewRequest{ManifestID: "manifest", SnapshotID: "point"}, SnapshotProtected: true, SnapshotAgeSeconds: 60, Surfaces: []workspace.PreviewSurface{{SurfaceID: "surface", Kind: "terminal", Label: "Shell", Disposition: "move", SessionStatus: "current", Observed: &workspace.Placement{Workspace: "review", Monitor: "DP-1"}, Desired: &workspace.Placement{Workspace: "planning", Monitor: "DP-1"}, Changes: []string{"workspace_changed"}, Requirements: []string{"pane_attachment_not_verified"}}}, UnownedLeftOpen: 2}
	text := flattenWorkspaceLines(workspacePreviewLines(p, workspace.SnapshotStatus{HeadAvailable: true}))
	for _, expected := range []string{"Saved point", "Age 60s", "fresh double inventory", "move", "review", "planning", "attachment requires verification", "2 unowned"} {
		if !strings.Contains(text, expected) {
			t.Fatal(expected, text)
		}
	}
	if strings.Contains(text, "topology and recovery are not implemented") {
		t.Fatal("retired workspace placeholder")
	}
}

func flattenWorkspaceLines(lines []line) string {
	var text strings.Builder
	for _, line := range lines {
		for _, segment := range line {
			text.WriteString(segment.text)
		}
		text.WriteByte('\n')
	}
	return text.String()
}
