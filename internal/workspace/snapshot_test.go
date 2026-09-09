package workspace

import (
	"encoding/json"
	"heimdall/internal/model"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

func snapshotSetup(t *testing.T) (*fixture, *ViewportService, *SnapshotService, ViewportRequest) {
	t.Helper()
	f, v, _, source := viewportSetup(t)
	_, m := f.manifest("alpha")
	binding := viewportRequest(t, f, v, m, source.ID)
	if _, err := v.Execute(f.ctx, binding, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	s := &SnapshotService{Store: f.e.Store, Observer: v.Observer}
	return f, v, s, source
}
func captureRequest(t *testing.T, f *fixture, s *SnapshotService) SnapshotRequest {
	t.Helper()
	v, err := s.Status(f.ctx, "alpha", f.now)
	if err != nil {
		t.Fatal(err)
	}
	head := "none"
	if v.Head != nil {
		head = v.Head.ID
	}
	return SnapshotRequest{Version: 1, ID: model.NewID(), Op: "capture", Target: "alpha", Previous: head, ExpectedTaskRevision: v.TaskRevision, ManifestID: v.ManifestID, SourceID: v.SourceID, InputDigest: v.InputDigest}
}
func capturePoint(t *testing.T, f *fixture, s *SnapshotService) model.WorkspacePoint {
	t.Helper()
	r := captureRequest(t, f, s)
	raw, err := s.Execute(f.ctx, r, "cli", f.now)
	if err != nil {
		t.Fatal(err)
	}
	var v model.WorkspacePoint
	if err = json.Unmarshal(raw, &v); err != nil {
		t.Fatal(err)
	}
	return v
}
func TestManualSnapshotStorageScopePinsAndReplay(t *testing.T) {
	f, v, s, _ := snapshotSetup(t)
	r := captureRequest(t, f, s)
	raw, err := s.Execute(f.ctx, r, "cli", f.now)
	if err != nil {
		t.Fatal(err)
	}
	var first model.WorkspacePoint
	json.Unmarshal(raw, &first)
	point, err := f.e.Store.WorkspacePoint(f.ctx, "alpha", first.ID)
	if err != nil || point.Pruned || point.Payload == nil || len(point.Payload.Surfaces) != 1 || point.Payload.Surfaces[0].Window.Identity.StableID != "18000001" {
		t.Fatal(point, err)
	}
	if len(point.Payload.Workspaces) != 1 || point.Payload.Workspaces[0].Name != "planning" {
		t.Fatal("unowned workspace metadata entered task payload")
	}
	if _, err = f.e.Store.WorkspacePoint(f.ctx, "beta", first.ID); err == nil {
		t.Fatal("cross-task snapshot read")
	}
	st, _ := f.e.Store.State(f.ctx)
	stateRaw, _ := json.Marshal(st)
	if strings.Contains(string(stateRaw), "Same title") {
		t.Fatal("desktop payload copied into task projection")
	}
	if len(st.SnapshotHeads) != 1 || len(st.SnapshotPins) != 1 {
		t.Fatal("manual head/pin missing")
	}
	v.Observer.Close()
	retry, err := s.Execute(f.ctx, r, "cli", f.now)
	if err != nil || string(retry) != string(raw) {
		t.Fatal("retry depended on live desktop", err)
	}
	if err = v.Restore(f.ctx); err != nil {
		t.Fatal(err)
	}
	release := SnapshotRequest{Version: 1, ID: model.NewID(), Op: "unpin", Target: "alpha", Previous: first.ID, SnapshotID: first.ID, Reason: "release manual pin"}
	if _, err = s.Execute(f.ctx, release, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	prune := SnapshotRequest{Version: 1, ID: model.NewID(), Op: "prune", Target: "alpha", Previous: "none", SnapshotIDs: []string{first.ID}, Reason: "retention test"}
	if _, err = s.Execute(f.ctx, prune, "cli", f.now); err == nil {
		t.Fatal("current head pruned")
	}
	second := capturePoint(t, f, s)
	if _, err = s.Execute(f.ctx, prune, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	old, err := f.e.Store.WorkspacePoint(f.ctx, "alpha", first.ID)
	if err != nil || !old.Pruned || old.Payload != nil {
		t.Fatal("pruned history not identifiable", old, err)
	}
	pin := SnapshotRequest{Version: 1, ID: model.NewID(), Op: "pin", Target: "alpha", Previous: "none", SnapshotID: first.ID, Reason: "cannot resurrect payload"}
	if _, err = s.Execute(f.ctx, pin, "cli", f.now); err == nil {
		t.Fatal("pruned payload pinned")
	}
	before, _ := f.e.Store.State(f.ctx)
	v.Observer.Close()
	replayed, err := f.e.Store.Replay(f.ctx)
	if err != nil || !reflect.DeepEqual(before, replayed) {
		t.Fatal("snapshot replay differs", err)
	}
	old, err = f.e.Store.WorkspacePoint(f.ctx, "alpha", first.ID)
	if err != nil || !old.Pruned {
		t.Fatal("replay resurrected pruned point", err)
	}
	point, err = f.e.Store.WorkspacePoint(f.ctx, "alpha", second.ID)
	if err != nil || point.Payload == nil {
		t.Fatal("replay lost retained payload", err)
	}
}
func TestSnapshotRequestAuthorityAndBindingCAS(t *testing.T) {
	f, v, s, _ := snapshotSetup(t)
	r := captureRequest(t, f, s)
	if _, err := s.Execute(f.ctx, r, "browser", f.now); err == nil {
		t.Fatal("browser captured desktop")
	}
	r.InputDigest = strings.Repeat("f", 64)
	if _, err := s.Execute(f.ctx, r, "cli", f.now); err == nil {
		t.Fatal("wrong capture input accepted")
	}
	r = captureRequest(t, f, s)
	if _, err := s.Execute(f.ctx, r, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	stale := r
	stale.ID = model.NewID()
	if _, err := s.Execute(f.ctx, stale, "cli", f.now); err == nil {
		t.Fatal("old snapshot head accepted")
	}
	before, _ := f.e.Store.State(f.ctx)
	v.Observer.Close()
	if _, err := s.Execute(f.ctx, captureRequest(t, f, s), "cli", f.now); err == nil {
		t.Fatal("unavailable desktop captured")
	}
	after, _ := f.e.Store.State(f.ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("outage erased last good state")
	}
}
func TestAutosnapshotDebounceDeadlineRetentionAndOutage(t *testing.T) {
	f, v, fake, source := viewportSetup(t)
	_, m := f.manifest("alpha")
	binding := viewportRequest(t, f, v, m, source.ID)
	if _, err := v.Execute(f.ctx, binding, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	s := &SnapshotService{Store: f.e.Store, Observer: v.Observer}
	policy := SnapshotRequest{Version: 1, ID: model.NewID(), Op: "policy", Target: "alpha", Previous: "none", ExpectedTaskRevision: 1, ManifestID: m.ID, SourceID: source.ID, Policy: &SnapshotPolicyInput{Enabled: true, DebounceSeconds: 2, MaxDirtySeconds: 5, RetainCount: 2}}
	if _, err := s.Execute(f.ctx, policy, "cli", f.now); err != nil {
		t.Fatal(err)
	}
	for second := 0; second <= 17; second++ {
		window := map[string]any{"address": "0x64", "stableId": "18000001", "pid": 123, "class": "same-app", "title": "Owned window", "mapped": true, "workspace": map[string]any{"id": 1, "name": "planning"}, "monitor": 0, "at": []int{second, 20}, "size": []int{800, 600}}
		fake.Set("clients", []any{window})
		if _, err := v.Observer.Read(f.ctx, true); err != nil {
			t.Fatal(err)
		}
		s.Scan(f.ctx, f.now.Add(time.Duration(second)*time.Second))
		points, err := s.List(f.ctx, "alpha", 0, 50)
		if err != nil {
			t.Fatal(err)
		}
		expected := (second + 1) / 6
		if len(points) != expected {
			t.Fatalf("at %ds got %d points; expected %d", second, len(points), expected)
		}
	}
	points, _ := s.List(f.ctx, "alpha", 0, 50)
	if len(points) != 3 || !points[2].Pruned || points[0].Pruned || points[1].Pruned {
		t.Fatal("automatic retention did not retain exactly two unpinned points", points)
	}
	before, _ := f.e.Store.State(f.ctx)
	events, _ := f.e.Store.Events(f.ctx)
	for second := 18; second <= 30; second++ {
		s.Scan(f.ctx, f.now.Add(time.Duration(second)*time.Second))
	}
	nowEvents, _ := f.e.Store.Events(f.ctx)
	if !reflect.DeepEqual(events, nowEvents) {
		t.Fatal("unchanged desktop grew event history")
	}
	fake.Windows()
	if _, err := v.Observer.Read(f.ctx, true); err != nil {
		t.Fatal(err)
	}
	s.Scan(f.ctx, f.now.Add(time.Minute))
	after, _ := f.e.Store.State(f.ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("empty/shutdown inventory erased desired workspace or head")
	}
	partial := capturePoint(t, f, s)
	if partial.Published || partial.Coverage != "partial" {
		t.Fatal("partial capture published head")
	}
	after, _ = f.e.Store.State(f.ctx)
	if after.SnapshotHeads["alpha"].ID != before.SnapshotHeads["alpha"].ID {
		t.Fatal("manual partial replaced last good")
	}
	v.Observer.Close()
	s.Scan(f.ctx, f.now.Add(2*time.Minute))
	status, err := s.Status(f.ctx, "alpha", f.now)
	if err != nil || !status.HeadAvailable || status.Capture.Issue != "fresh_source_unavailable" {
		t.Fatal("outage status", status, err)
	}
	replay, err := f.e.Store.Replay(f.ctx)
	if err != nil || !reflect.DeepEqual(after, replay) {
		t.Fatal("automatic/pruned replay changed state", err)
	}
}

func TestFrozenSnapshotRequests(t *testing.T) {
	raw, err := os.ReadFile("../../testdata/snapshots/requests-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var requests []json.RawMessage
	if err = json.Unmarshal(raw, &requests); err != nil {
		t.Fatal(err)
	}
	for _, raw := range requests {
		if _, err := DecodeSnapshot(raw); err != nil {
			t.Fatal(err)
		}
		var v map[string]any
		json.Unmarshal(raw, &v)
		v["observed_payload"] = true
		bad, _ := json.Marshal(v)
		if _, err := DecodeSnapshot(bad); err == nil {
			t.Fatal("caller-supplied observation accepted")
		}
	}
}
