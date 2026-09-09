package store

import (
	"context"
	"encoding/json"
	"heimdall/internal/model"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"testing"
	"time"
)

// An opt-in real SQLite writer measurement: a continuously dirty workspace for
// a day at 30-second intervals. No wall-clock sleeps or simulated I/O timings.
func TestSnapshotDailyVolume(t *testing.T) {
	count, err := strconv.Atoi(os.Getenv("HEIMDALL_SNAPSHOT_VOLUME"))
	if err != nil || count < 32 {
		t.Skip("set HEIMDALL_SNAPSHOT_VOLUME=2880 for a full-day capture measurement")
	}
	s, dir := snapshotStore(t, 8)
	ctx := context.Background()
	initial, _ := s.State(ctx)
	before, _ := json.Marshal(initial)
	ids := []string{}
	latencies := []time.Duration{}
	now := time.Now().UTC()
	var payloadBytes int
	start := time.Now()
	for i := 0; i < count; i++ {
		id := model.NewID()
		at := now.Add(time.Duration(i) * 30 * time.Second)
		began := time.Now()
		_, err := s.Transact(ctx, "snapshot-"+id, "snapshotter", []byte(id), at, func(st model.State) (Change, error) {
			c := snapshotChange(st, id, i, at)
			payloadBytes = len(c.SnapshotPayloads[id])
			if len(ids) >= 16 {
				c.Events = append(c.Events, Pending{Subject: "snapshot", Verb: "pruned", EntityID: id, Payload: model.SnapshotPrune{Version: 1, ID: id, Target: "alpha", SnapshotIDs: []string{ids[len(ids)-16]}, Reason: "measured retention", Actor: "snapshotter", At: at}})
			}
			return c, nil
		})
		if err != nil {
			t.Fatal(i, err)
		}
		latencies = append(latencies, time.Since(began))
		ids = append(ids, id)
	}
	elapsed := time.Since(start)
	final, _ := s.State(ctx)
	after, _ := json.Marshal(final)
	if len(after) > len(before)+2000 {
		t.Fatal("snapshot history grew task projection", len(before), len(after))
	}
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	lookup := time.Now()
	for i := 0; i < 1000; i++ {
		if _, err := s.WorkspacePoint(ctx, "alpha", ids[len(ids)-1]); err != nil {
			t.Fatal(err)
		}
	}
	lookupElapsed := time.Since(lookup)
	replay := time.Now()
	if _, err := s.Replay(ctx); err != nil {
		t.Fatal(err)
	}
	replayElapsed := time.Since(replay)
	s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	var retained, payloadTotal, eventBytes, metadataBytes int64
	s.db.QueryRow("SELECT count(*),coalesce(sum(length(body)),0) FROM workspace_snapshot_payloads").Scan(&retained, &payloadTotal)
	s.db.QueryRow("SELECT coalesce(sum(length(payload)),0) FROM events").Scan(&eventBytes)
	s.db.QueryRow("SELECT coalesce(sum(length(metadata)),0) FROM workspace_snapshots").Scan(&metadataBytes)
	if retained != 16 {
		t.Fatal("retention not bounded", retained)
	}
	info, err := os.Stat(filepath.Join(dir, "heimdall.db"))
	if err != nil {
		t.Fatal(err)
	}
	report := map[string]any{"captures": count, "surfaces": 8, "cadence_seconds": 30, "simulated_hours": float64(count) / 120, "elapsed_ms": float64(elapsed.Microseconds()) / 1000, "writer_p50_ms": float64(latencies[len(latencies)/2].Microseconds()) / 1000, "writer_p95_ms": float64(latencies[len(latencies)*95/100].Microseconds()) / 1000, "writer_p99_ms": float64(latencies[len(latencies)*99/100].Microseconds()) / 1000, "writer_max_ms": float64(latencies[len(latencies)-1].Microseconds()) / 1000, "lookup_mean_ms": float64(lookupElapsed.Microseconds()) / 1000000, "replay_ms": float64(replayElapsed.Microseconds()) / 1000, "projection_before_bytes": len(before), "projection_after_bytes": len(after), "payload_bytes_each": payloadBytes, "retained_payloads": retained, "retained_payload_bytes": payloadTotal, "event_payload_bytes": eventBytes, "metadata_index_bytes": metadataBytes, "database_bytes": info.Size()}
	raw, _ := json.Marshal(report)
	t.Log(string(raw))
	if output := os.Getenv("HEIMDALL_SNAPSHOT_VOLUME_REPORT"); output != "" {
		if err := os.WriteFile(output, append(raw, '\n'), 0600); err != nil {
			t.Fatal(err)
		}
	}
}
