package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func snapshotStore(t *testing.T, surfaces int) (*Store, string) {
	t.Helper()
	dir := t.TempDir()
	raw, err := os.ReadFile("../../testdata/snapshots/schema14.sql")
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "heimdall.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(raw)); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	ctx := context.Background()
	st, _ := s.State(ctx)
	now := time.Now().UTC()
	old := st.WorkspaceManifests[st.WorkspaceHeads["alpha"]]
	m := old
	m.ID = model.NewID()
	m.Previous = old.ID
	m.At = now
	for len(m.Surfaces) < surfaces {
		m.Surfaces = append(m.Surfaces, model.DesiredSurface{ID: model.NewID(), Kind: "terminal", Label: "Synthetic shell", RestorePolicy: "manual"})
	}
	if _, err = s.Transact(ctx, "workspace-"+m.ID, "cli", []byte(m.ID), now, func(st model.State) (Change, error) {
		return Change{Events: []Pending{{Subject: "workspace", Verb: "accepted", EntityID: m.ID, Payload: m}}, Result: m}, nil
	}); err != nil {
		t.Fatal(err)
	}
	for i, surface := range m.Surfaces {
		st, _ = s.State(ctx)
		id := model.NewID()
		b := model.ViewportBinding{Version: 1, ID: id, Target: "alpha", TaskRevision: 1, ManifestID: m.ID, SurfaceID: surface.ID, Previous: st.ViewportHeads[surface.ID], Active: true, SourceID: st.DesktopSourceHead, SnapshotID: strings.Repeat("f", 64), Window: &model.WindowIdentity{SourceEpoch: st.DesktopSources[st.DesktopSourceHead].Epoch, StableID: fmt.Sprintf("18000%03x", i)}, Actor: "cli", At: now}
		if _, err = s.Transact(ctx, "viewport-"+id, "cli", []byte(id), now, func(st model.State) (Change, error) {
			return Change{Events: []Pending{{Subject: "viewport", Verb: "bound", EntityID: id, Payload: b}}, Result: b}, nil
		}); err != nil {
			t.Fatal(err)
		}
	}
	st, _ = s.State(ctx)
	id := model.NewID()
	policy := model.SnapshotPolicy{Version: 1, ID: id, Target: "alpha", Enabled: true, TaskRevision: 1, ManifestID: m.ID, SourceID: st.DesktopSourceHead, DebounceSeconds: 2, MaxDirtySeconds: 30, RetainCount: 16, Actor: "cli", At: now}
	if _, err = s.Transact(ctx, "snapshot-"+id, "cli", []byte(id), now, func(st model.State) (Change, error) {
		return Change{Events: []Pending{{Subject: "snapshot", Verb: "policy", EntityID: id, Payload: policy}}, Result: policy}, nil
	}); err != nil {
		t.Fatal(err)
	}
	return s, dir
}
func snapshotChange(st model.State, id string, offset int, now time.Time) Change {
	target := "alpha"
	m := st.WorkspaceManifests[st.WorkspaceHeads[target]]
	source := st.DesktopSources[st.DesktopSourceHead]
	payload := model.WorkspacePointPayload{Boundary: model.SnapshotBoundary{Method: "double_inventory_with_buffered_unsequenced_events", StartedAt: now, FinishedAt: now}, Version: 1, Target: target, TaskRevision: 1, ManifestID: m.ID, SourceEpoch: source.Epoch, ObservedAt: now, Coverage: "complete", Monitors: []model.DesktopMonitor{{ID: 0, Name: "SYNTHETIC-1", Width: 1920, Height: 1080, Scale: 1}}, Workspaces: []model.DesktopWorkspace{{ID: 1, Name: "planning", MonitorID: 0, MonitorName: "SYNTHETIC-1"}}, Surfaces: []model.SnapshotSurface{}}
	for _, surface := range m.Surfaces {
		b := st.ViewportBindings[st.ViewportHeads[surface.ID]]
		payload.Surfaces = append(payload.Surfaces, model.SnapshotSurface{SurfaceID: surface.ID, ViewportBindingID: b.ID, SessionBindingID: st.SessionHeads[surface.ID], Status: "observed", Window: &model.DesktopWindow{Identity: *b.Window, Address: "0x" + b.Window.StableID, PID: 123, Class: "synthetic-shell", Title: strings.Repeat("Synthetic window ", 28), WorkspaceID: 1, MonitorID: 0, At: [2]int{offset, 20}, Size: [2]int{800, 600}}})
	}
	raw, _ := json.Marshal(payload)
	point := model.WorkspacePoint{Version: 1, ID: id, Target: target, TaskRevision: 1, ManifestID: m.ID, SourceID: source.ID, SourceEpoch: source.Epoch, PreviousHead: st.SnapshotHeads[target].ID, InputDigest: model.SnapshotInputDigest(st, target), ContentDigest: model.SnapshotContentDigest(payload), PayloadDigest: model.SnapshotHash(raw), PayloadBytes: len(raw), Kind: "automatic", PolicyID: st.SnapshotPolicies[target].ID, Coverage: "complete", Published: true, ObservedAt: now, Actor: "snapshotter", At: now}
	return Change{Revision: st.Revision, Events: []Pending{{Subject: "snapshot", Verb: "captured", EntityID: id, Payload: point}}, Result: point, SnapshotPayloads: map[string]json.RawMessage{id: raw}}
}
func saveTestSnapshot(t *testing.T, s *Store, offset int) string {
	t.Helper()
	id := model.NewID()
	now := time.Now().UTC()
	if _, err := s.Transact(context.Background(), "snapshot-"+id, "snapshotter", []byte(id), now, func(st model.State) (Change, error) { return snapshotChange(st, id, offset, now), nil }); err != nil {
		t.Fatal(err)
	}
	return id
}
func TestSnapshotAtomicFailuresAndCorruption(t *testing.T) {
	for _, failure := range []string{"missing_payload", "bad_digest", "projection_write", "disk_full"} {
		t.Run(failure, func(t *testing.T) {
			s, _ := snapshotStore(t, 8)
			ctx := context.Background()
			saveTestSnapshot(t, s, 0)
			before, _ := s.State(ctx)
			events, _ := s.Events(ctx)
			if failure == "projection_write" {
				if _, err := s.db.Exec("CREATE TRIGGER fail_projection BEFORE UPDATE ON projection_state BEGIN SELECT RAISE(ABORT,'injected publication failure'); END;"); err != nil {
					t.Fatal(err)
				}
			}
			if failure == "disk_full" {
				var pages int
				s.db.QueryRow("PRAGMA page_count").Scan(&pages)
				if _, err := s.db.Exec(fmt.Sprintf("PRAGMA max_page_count=%d", pages)); err != nil {
					t.Fatal(err)
				}
			}
			id := model.NewID()
			now := time.Now().UTC()
			_, err := s.Transact(ctx, "snapshot-"+id, "snapshotter", []byte(id), now, func(st model.State) (Change, error) {
				c := snapshotChange(st, id, 1, now)
				if failure == "missing_payload" {
					c.SnapshotPayloads = nil
				}
				if failure == "bad_digest" {
					c.SnapshotPayloads[id] = json.RawMessage(`{}`)
				}
				return c, nil
			})
			if err == nil {
				t.Fatal("injected failure committed")
			}
			if failure == "disk_full" && !strings.Contains(err.Error(), "full") {
				t.Fatal("expected SQLite allocation failure", err)
			}
			after, _ := s.State(ctx)
			afterEvents, _ := s.Events(ctx)
			if !reflect.DeepEqual(before, after) || !reflect.DeepEqual(events, afterEvents) {
				t.Fatal("failed snapshot changed head/events")
			}
			var payloads, records int
			s.db.QueryRow("SELECT count(*) FROM workspace_snapshot_payloads").Scan(&payloads)
			s.db.QueryRow("SELECT count(*) FROM workspace_snapshots").Scan(&records)
			if payloads != 1 || records != 1 {
				t.Fatal("failed snapshot leaked payload or metadata", payloads, records)
			}
		})
	}
	s, _ := snapshotStore(t, 1)
	id := saveTestSnapshot(t, s, 0)
	ctx := context.Background()
	before, _ := s.State(ctx)
	if _, err := s.db.Exec("DELETE FROM workspace_snapshot_payloads WHERE snapshot_id=?", id); err != nil {
		t.Fatal(err)
	}
	if _, err := s.WorkspacePoint(ctx, "alpha", id); err == nil {
		t.Fatal("missing retained payload read succeeded")
	}
	if _, err := s.Replay(ctx); err == nil {
		t.Fatal("replay silently recaptured or accepted missing payload")
	}
	after, _ := s.State(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("failed replay changed head")
	}
}
func TestSnapshotRetentionProtectsPinsAndReplaysUnavailableHistory(t *testing.T) {
	s, _ := snapshotStore(t, 1)
	ctx := context.Background()
	first := saveTestSnapshot(t, s, 0)
	second := saveTestSnapshot(t, s, 1)
	now := time.Now().UTC()
	pinID := model.NewID()
	pin := model.SnapshotPin{Version: 1, ID: pinID, Target: "alpha", SnapshotID: first, Active: true, Reason: "reserved for explicit recovery review", Actor: "cli", At: now}
	if _, err := s.Transact(ctx, "snapshot-"+pinID, "cli", []byte(pinID), now, func(st model.State) (Change, error) {
		return Change{Events: []Pending{{Subject: "snapshot", Verb: "pin", EntityID: pinID, Payload: pin}}, Result: pin}, nil
	}); err != nil {
		t.Fatal(err)
	}
	prune := func(id, actor string) error {
		request := model.NewID()
		p := model.SnapshotPrune{Version: 1, ID: request, Target: "alpha", SnapshotIDs: []string{id}, Reason: "test pruning", Actor: actor, At: now}
		_, err := s.Transact(ctx, "snapshot-"+request, actor, []byte(request), now, func(st model.State) (Change, error) {
			return Change{Events: []Pending{{Subject: "snapshot", Verb: "pruned", EntityID: request, Payload: p}}, Result: p}, nil
		})
		return err
	}
	if prune(first, "cli") == nil || prune(second, "cli") == nil || prune(first, "snapshotter") == nil {
		t.Fatal("protected snapshot was pruned")
	}
	unpin := pin
	unpin.ID = model.NewID()
	unpin.Previous = pin.ID
	unpin.Active = false
	if _, err := s.Transact(ctx, "snapshot-"+unpin.ID, "cli", []byte(unpin.ID), now, func(st model.State) (Change, error) {
		return Change{Events: []Pending{{Subject: "snapshot", Verb: "pin", EntityID: unpin.ID, Payload: unpin}}, Result: unpin}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := prune(first, "cli"); err != nil {
		t.Fatal(err)
	}
	before, _ := s.State(ctx)
	replayed, err := s.Replay(ctx)
	if err != nil || !reflect.DeepEqual(before, replayed) {
		t.Fatal("retention replay differs", err)
	}
	point, err := s.WorkspacePoint(ctx, "alpha", first)
	if err != nil || !point.Pruned || point.Payload != nil {
		t.Fatal("pruned record not preserved", err)
	}
	if point, err = s.WorkspacePoint(ctx, "alpha", second); err != nil || point.Payload == nil {
		t.Fatal("retained head unavailable", err)
	}
}
func TestStoppedSchemaFourteenFixtureUpgrade(t *testing.T) {
	testStoppedWorkspaceFixture(t, "../../testdata/snapshots/schema14.sql", 14)
}

func TestRetainedSnapshotSQLFixtureReplayAndBackup(t *testing.T) {
	dir := t.TempDir()
	raw, err := os.ReadFile("../../testdata/snapshots/schema15-retained.sql")
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "heimdall.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(raw)); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx := context.Background()
	before, _ := s.State(ctx)
	points, err := s.WorkspacePoints(ctx, "alpha", 0, 256)
	if err != nil || len(points) != 5 {
		t.Fatal("snapshot fixture missing history", len(points), err)
	}
	if _, err = s.Replay(ctx); err != nil {
		t.Fatal(err)
	}
	after, _ := s.State(ctx)
	if !reflect.DeepEqual(before, after) {
		t.Fatal("fixture replay changed heads/pins/policy")
	}
	pruned, retained := 0, 0
	for _, point := range points {
		v, err := s.WorkspacePoint(ctx, "alpha", point.Point.ID)
		if err != nil {
			t.Fatal(err)
		}
		if v.Pruned {
			pruned++
			if v.Payload != nil {
				t.Fatal("pruned payload restored")
			}
		} else {
			retained++
			if v.Payload == nil {
				t.Fatal("retained payload absent")
			}
		}
	}
	if pruned != 2 || retained != 3 {
		t.Fatal(pruned, retained)
	}
	backup := filepath.Join(t.TempDir(), "snapshot.db")
	if err = s.Backup(ctx, backup); err != nil {
		t.Fatal(err)
	}
	restored := t.TempDir()
	raw, err = os.ReadFile(backup)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(restored, "heimdall.db"), raw, 0600); err != nil {
		t.Fatal(err)
	}
	restore, err := Open(restored)
	if err != nil {
		t.Fatal(err)
	}
	defer restore.Close()
	if _, err = restore.Replay(ctx); err != nil {
		t.Fatal(err)
	}
	head := before.SnapshotHeads["alpha"]
	if v, err := restore.WorkspacePoint(ctx, "alpha", head.ID); err != nil || v.Payload == nil {
		t.Fatal("backup lost head payload", err)
	}
}
