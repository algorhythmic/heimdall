package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
)

const snapshotTables = `
CREATE TABLE IF NOT EXISTS workspace_snapshots(id TEXT PRIMARY KEY,target TEXT NOT NULL,event_id INTEGER NOT NULL UNIQUE,payload_digest TEXT NOT NULL,payload_bytes INTEGER NOT NULL,pruned INTEGER NOT NULL DEFAULT 0,metadata TEXT NOT NULL CHECK(json_valid(metadata)));
CREATE INDEX IF NOT EXISTS workspace_snapshots_target_event ON workspace_snapshots(target,event_id DESC);
CREATE INDEX IF NOT EXISTS workspace_snapshots_retained ON workspace_snapshots(target,event_id DESC) WHERE pruned=0;
CREATE INDEX IF NOT EXISTS workspace_snapshots_retained_bytes ON workspace_snapshots(payload_bytes) WHERE pruned=0;
CREATE TABLE IF NOT EXISTS workspace_snapshot_payloads(snapshot_id TEXT PRIMARY KEY,body BLOB NOT NULL);
`
const MaxSnapshotStoreBytes = 128 << 20

type PointView struct {
	Point   model.WorkspacePoint         `json:"point"`
	Pruned  bool                         `json:"pruned"`
	Payload *model.WorkspacePointPayload `json:"payload,omitempty"`
	EventID int64                        `json:"event_id"`
}

// Payload insertion, index metadata, event and head use the same transaction.
// The immutable payload table is authoritative retained data. Its metadata index
// is rebuilt by replay; pruning is an explicit event and never recaptures bytes.
func applySnapshotSQL(ctx context.Context, tx *sql.Tx, st model.State, e Event, payloads map[string]json.RawMessage, replay bool) error {
	if e.Subject != "snapshot" {
		return nil
	}
	switch e.Verb {
	case "captured":
		var v model.WorkspacePoint
		if err := model.StrictJSON(e.Payload, &v); err != nil {
			return err
		}
		raw, ok := payloads[v.ID]
		if replay {
			err := tx.QueryRowContext(ctx, "SELECT body FROM workspace_snapshot_payloads WHERE snapshot_id=?", v.ID).Scan(&raw)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
			ok = err == nil
		}
		if !ok && !replay {
			return fmt.Errorf("snapshot payload missing from publication")
		}
		if ok {
			if err := model.ValidateSnapshotPayload(st, v, raw); err != nil {
				return err
			}
		}
		if _, err := tx.ExecContext(ctx, "INSERT INTO workspace_snapshots(id,target,event_id,payload_digest,payload_bytes,metadata) VALUES(?,?,?,?,?,?)", v.ID, v.Target, e.ID, v.PayloadDigest, v.PayloadBytes, string(e.Payload)); err != nil {
			return err
		}
		if !replay {
			if _, err := tx.ExecContext(ctx, "INSERT INTO workspace_snapshot_payloads(snapshot_id,body) VALUES(?,?)", v.ID, []byte(raw)); err != nil {
				return err
			}
		}
	case "pin":
		var v model.SnapshotPin
		if err := model.StrictJSON(e.Payload, &v); err != nil {
			return err
		}
		var target string
		var pruned bool
		if err := tx.QueryRowContext(ctx, "SELECT target,pruned FROM workspace_snapshots WHERE id=?", v.SnapshotID).Scan(&target, &pruned); err != nil {
			return fmt.Errorf("snapshot pin target unavailable")
		}
		if target != v.Target || pruned {
			return fmt.Errorf("cannot pin a foreign or pruned snapshot")
		}
		if !replay && v.Active {
			var body []byte
			var digest string
			var size int
			if err := tx.QueryRowContext(ctx, "SELECT p.body,s.payload_digest,s.payload_bytes FROM workspace_snapshots s JOIN workspace_snapshot_payloads p ON p.snapshot_id=s.id WHERE s.id=?", v.SnapshotID).Scan(&body, &digest, &size); err != nil || len(body) != size || model.SnapshotHash(body) != digest {
				return fmt.Errorf("cannot pin missing or damaged payload")
			}
		}
	case "pruned":
		var v model.SnapshotPrune
		if err := model.StrictJSON(e.Payload, &v); err != nil {
			return err
		}
		for _, id := range v.SnapshotIDs {
			res, err := tx.ExecContext(ctx, "UPDATE workspace_snapshots SET pruned=1 WHERE id=? AND target=? AND pruned=0", id, v.Target)
			if err != nil {
				return err
			}
			n, err := res.RowsAffected()
			if err != nil || n != 1 {
				return fmt.Errorf("snapshot retention target missing, foreign or already pruned")
			}
			if _, err := tx.ExecContext(ctx, "DELETE FROM workspace_snapshot_payloads WHERE snapshot_id=?", id); err != nil {
				return err
			}
		}
	}
	return nil
}
func validateSnapshotStorage(ctx context.Context, tx *sql.Tx) error {
	// Stream retained payloads rather than copying all desktop history into State.
	rows, err := tx.QueryContext(ctx, "SELECT s.id,s.pruned,s.payload_digest,s.payload_bytes,p.body FROM workspace_snapshots s LEFT JOIN workspace_snapshot_payloads p ON p.snapshot_id=s.id")
	if err != nil {
		return err
	}
	defer rows.Close()
	total := int64(0)
	for rows.Next() {
		var id, digest string
		var pruned bool
		var size int
		var raw []byte
		if err := rows.Scan(&id, &pruned, &digest, &size, &raw); err != nil {
			return err
		}
		if pruned {
			if raw != nil {
				return fmt.Errorf("pruned snapshot still has payload")
			}
			continue
		}
		if raw == nil || len(raw) != size || model.SnapshotHash(raw) != digest {
			return fmt.Errorf("retained snapshot payload missing or damaged: %s", id)
		}
		total += int64(size)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	if total > MaxSnapshotStoreBytes {
		return fmt.Errorf("snapshot payload capacity (128 MiB) reached; release pins or prune history")
	}
	var orphan int
	if err := tx.QueryRowContext(ctx, "SELECT count(*) FROM workspace_snapshot_payloads p LEFT JOIN workspace_snapshots s ON s.id=p.snapshot_id WHERE s.id IS NULL").Scan(&orphan); err != nil {
		return err
	}
	if orphan != 0 {
		return fmt.Errorf("orphan snapshot payload")
	}
	return nil
}
func snapshotCapacity(ctx context.Context, tx *sql.Tx) error {
	var total int64
	if err := tx.QueryRowContext(ctx, "SELECT coalesce(sum(payload_bytes),0) FROM workspace_snapshots WHERE pruned=0").Scan(&total); err != nil {
		return err
	}
	if total > MaxSnapshotStoreBytes {
		return fmt.Errorf("snapshot payload capacity (128 MiB) reached; release pins or prune history")
	}
	return nil
}

func (s *Store) WorkspacePoint(ctx context.Context, target, id string) (PointView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.workspacePoint(ctx, target, id)
}

// WorkspacePreviewInputs samples retention metadata/payload and current heads
// under the same writer lock. No external observation runs inside this read.
func (s *Store) WorkspacePreviewInputs(ctx context.Context, target, id string) (model.State, PointView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := readState(ctx, s.db)
	if err != nil || id == "" {
		return st, PointView{}, err
	}
	point, err := s.workspacePoint(ctx, target, id)
	return st, point, err
}

func (s *Store) workspacePoint(ctx context.Context, target, id string) (PointView, error) {
	var v PointView
	var raw []byte
	if err := s.db.QueryRowContext(ctx, "SELECT metadata,pruned,event_id FROM workspace_snapshots WHERE id=? AND target=?", id, target).Scan(&raw, &v.Pruned, &v.EventID); err != nil {
		return PointView{}, fmt.Errorf("snapshot not found for task")
	}
	if err := model.StrictJSON(raw, &v.Point); err != nil {
		return PointView{}, err
	}
	if err := v.Point.Validate(); err != nil {
		return PointView{}, err
	}
	if v.Point.ID != id || v.Point.Target != target {
		return PointView{}, fmt.Errorf("snapshot metadata identity mismatch")
	}
	if !v.Pruned {
		if err := s.db.QueryRowContext(ctx, "SELECT body FROM workspace_snapshot_payloads WHERE snapshot_id=?", id).Scan(&raw); err != nil {
			return PointView{}, fmt.Errorf("retained snapshot payload unavailable")
		}
		if len(raw) != v.Point.PayloadBytes || model.SnapshotHash(raw) != v.Point.PayloadDigest {
			return PointView{}, fmt.Errorf("retained snapshot payload damaged")
		}
		var p model.WorkspacePointPayload
		if err := model.StrictJSON(raw, &p); err != nil {
			return PointView{}, err
		}
		v.Payload = &p
	}
	return v, nil
}
func (s *Store) WorkspacePoints(ctx context.Context, target string, before int64, limit int) ([]PointView, error) {
	if !model.ValidID(target) || before < 0 || limit < 1 || limit > 256 {
		return nil, fmt.Errorf("snapshot list requires target, nonnegative cursor and limit 1..256")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.QueryContext(ctx, "SELECT metadata,pruned,event_id FROM workspace_snapshots WHERE target=? AND (?=0 OR event_id<?) ORDER BY event_id DESC LIMIT ?", target, before, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PointView{}
	for rows.Next() {
		var v PointView
		var raw []byte
		if err := rows.Scan(&raw, &v.Pruned, &v.EventID); err != nil {
			return nil, err
		}
		if err := model.StrictJSON(raw, &v.Point); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// WorkspaceRetainedPoints supports retention independently of historical list
// pagination. Old released pins must remain eligible even behind many tombstones.
func (s *Store) WorkspaceRetainedPoints(ctx context.Context, target string) ([]PointView, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	rows, err := s.db.QueryContext(ctx, "SELECT metadata,event_id FROM workspace_snapshots WHERE target=? AND pruned=0 ORDER BY event_id DESC LIMIT 513", target)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []PointView{}
	for rows.Next() {
		var v PointView
		var raw []byte
		if err := rows.Scan(&raw, &v.EventID); err != nil {
			return nil, err
		}
		if err := model.StrictJSON(raw, &v.Point); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if len(out) > 512 {
		return nil, fmt.Errorf("retention candidate limit exceeded; prune explicit historical points")
	}
	return out, rows.Err()
}
