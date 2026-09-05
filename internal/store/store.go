// Package store owns the event log and its disposable projection.
package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"heimdall/internal/model"
	_ "modernc.org/sqlite"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var ErrConflict = errors.New("revision or idempotency conflict")

const SchemaVersion = 5

type Event struct {
	ID        int64           `json:"id"`
	Version   int             `json:"event_version"`
	TS        time.Time       `json:"ts"`
	Subject   string          `json:"subject"`
	Verb      string          `json:"verb"`
	Actor     string          `json:"actor"`
	EntityID  string          `json:"entity_id,omitempty"`
	CommandID string          `json:"command_id"`
	Payload   json.RawMessage `json:"payload"`
}
type Pending struct {
	Subject, Verb, EntityID string
	Payload                 any
}
type Change struct {
	Revision int64
	Events   []Pending
	Result   any
}
type Acceptance struct {
	Hash     string          `json:"request_hash"`
	Revision int64           `json:"revision"`
	Result   json.RawMessage `json:"result"`
}
type TaskChange struct {
	Record model.TaskRecord `json:"record"`
	Fields []string         `json:"fields"`
}
type Store struct {
	mu   sync.Mutex
	db   *sql.DB
	lock *os.File
}

func Open(dir string) (*Store, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "writer.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = lockFile(f); err != nil {
		f.Close()
		return nil, fmt.Errorf("another Heimdall process owns this data directory: %w", err)
	}
	dbPath := filepath.Join(dir, "heimdall.db")
	dbFile, err := os.OpenFile(dbPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		f.Close()
		return nil, err
	}
	dbFile.Close()
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		f.Close()
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, lock: f}
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		s.Close()
		return nil, err
	}
	if version < 0 || version > SchemaVersion {
		s.Close()
		return nil, fmt.Errorf("unsupported database schema version %d", version)
	}
	if version > 0 && version < SchemaVersion {
		if _, err = db.Exec("PRAGMA synchronous=FULL"); err != nil {
			s.Close()
			return nil, err
		}
		backupDir := filepath.Join(dir, "backups")
		if err = os.MkdirAll(backupDir, 0700); err != nil {
			s.Close()
			return nil, err
		}
		backupPath := filepath.Join(backupDir, fmt.Sprintf("pre-schema-%d-%s.db", SchemaVersion, model.NewID()))
		if err = s.backup(context.Background(), backupPath); err != nil {
			s.Close()
			return nil, fmt.Errorf("pre-upgrade backup failed: %w", err)
		}
	}
	_, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA synchronous=FULL; PRAGMA busy_timeout=2500;
 CREATE TABLE IF NOT EXISTS events(id INTEGER PRIMARY KEY,event_version INTEGER NOT NULL,ts TEXT NOT NULL,subject TEXT NOT NULL,verb TEXT NOT NULL,actor TEXT NOT NULL,entity_id TEXT NOT NULL,command_id TEXT NOT NULL,payload TEXT NOT NULL CHECK(json_valid(payload)),idempotency_key TEXT NOT NULL UNIQUE);
 CREATE TABLE IF NOT EXISTS projection_state(id INTEGER PRIMARY KEY CHECK(id=1),body TEXT NOT NULL CHECK(json_valid(body)));
 CREATE TABLE IF NOT EXISTS commands(id TEXT PRIMARY KEY,request_hash TEXT NOT NULL,result TEXT NOT NULL);
 PRAGMA user_version=5;`)
	if err != nil {
		s.Close()
		return nil, err
	}
	b, _ := json.Marshal(model.Empty())
	_, err = db.Exec("INSERT OR IGNORE INTO projection_state VALUES(1,?)", string(b))
	if err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}
func (s *Store) Close() error { err := s.db.Close(); s.lock.Close(); return err }
func hash(b []byte) string    { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func readState(ctx context.Context, q interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}) (model.State, error) {
	var b []byte
	var st model.State
	err := q.QueryRowContext(ctx, "SELECT body FROM projection_state WHERE id=1").Scan(&b)
	if err == nil {
		err = json.Unmarshal(b, &st)
		st.Normalize()
	}
	return st, err
}
func (s *Store) State(ctx context.Context) (model.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return readState(ctx, s.db)
}
func (s *Store) Transact(ctx context.Context, id, actor string, request []byte, now time.Time, build func(model.State) (Change, error)) (json.RawMessage, error) {
	return s.TransactChecked(ctx, id, actor, request, now, nil, build)
}

// TransactChecked checks current authority before exposing a cached result and
// again before committing fresh events. Both checks share the writer transaction.
func (s *Store) TransactChecked(ctx context.Context, id, actor string, request []byte, now time.Time, authorize func(model.State) error, build func(model.State) (Change, error)) (json.RawMessage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id == "" || actor == "" || now.IsZero() {
		return nil, fmt.Errorf("command requires id, actor and timestamp")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	st, err := readState(ctx, tx)
	if err != nil {
		return nil, err
	}
	if authorize != nil {
		if err = authorize(st); err != nil {
			return nil, err
		}
	}
	var oldHash string
	var old []byte
	err = tx.QueryRowContext(ctx, "SELECT request_hash,result FROM commands WHERE id=?", id).Scan(&oldHash, &old)
	h := hash(request)
	if err == nil {
		if h != oldHash {
			return nil, ErrConflict
		}
		return old, nil
	}
	if err != sql.ErrNoRows {
		return nil, err
	}
	change, err := build(st)
	if err != nil {
		return nil, err
	}
	result, err := json.Marshal(change.Result)
	if err != nil {
		return nil, err
	}
	// Empty background scans do not grow the event log.
	if actor == "scheduler" && len(change.Events) == 0 {
		return result, nil
	}
	accepted := Pending{"command", "accepted", id, Acceptance{h, change.Revision, result}}
	events := append([]Pending{accepted}, change.Events...)
	for i, p := range events {
		payload, err := json.Marshal(p.Payload)
		if err != nil {
			return nil, err
		}
		e := Event{Version: 1, TS: now.UTC(), Subject: p.Subject, Verb: p.Verb, Actor: actor, EntityID: p.EntityID, CommandID: id, Payload: payload}
		row, err := tx.ExecContext(ctx, "INSERT INTO events(event_version,ts,subject,verb,actor,entity_id,command_id,payload,idempotency_key) VALUES(?,?,?,?,?,?,?,?,?)", 1, e.TS.Format(time.RFC3339Nano), e.Subject, e.Verb, actor, e.EntityID, id, string(payload), fmt.Sprintf("%s/%d", id, i))
		if err != nil {
			return nil, err
		}
		e.ID, err = row.LastInsertId()
		if err != nil {
			return nil, err
		}
		if err = Apply(&st, e); err != nil {
			return nil, err
		}
	}
	b, err := json.Marshal(st)
	if err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE projection_state SET body=? WHERE id=1", string(b)); err != nil {
		return nil, err
	}
	if _, err = tx.ExecContext(ctx, "INSERT INTO commands VALUES(?,?,?)", id, h, string(result)); err != nil {
		return nil, err
	}
	if authorize != nil {
		if err = authorize(st); err != nil {
			return nil, err
		}
	}
	if err = tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

// Inspect serializes authorization and response construction with writes, including
// revocation. Callbacks must not call Store methods or retain the supplied state.
func (s *Store) Inspect(ctx context.Context, inspect func(model.State) error) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, err := readState(ctx, s.db)
	if err != nil {
		return err
	}
	return inspect(st)
}
func Apply(st *model.State, e Event) error {
	st.Normalize()
	if e.Version != 1 {
		return fmt.Errorf("unsupported event version %d at %d", e.Version, e.ID)
	}
	switch e.Subject + "." + e.Verb {
	case "grant.issued", "grant.revoked":
		if err := applyGrant(st, e); err != nil {
			return err
		}
	case "contract.accepted", "decision.accepted", "resource.bound", "resource.unbound", "checkpoint.recorded":
		if err := applyContinuity(st, e); err != nil {
			return err
		}
	case "browser.profile_seen", "browser.pairing_changed", "browser.inventory_observed":
		var p model.BrowserProfile
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if p.ID != e.EntityID {
			return fmt.Errorf("invalid browser profile identity")
		}
		st.Browsers[p.ID] = p
	case "browser.command_queued", "browser.command_finished":
		var p model.BrowserOperation
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if p.ID != e.EntityID {
			return fmt.Errorf("invalid browser operation identity")
		}
		st.BrowserOperations[p.ID] = p
	case "command.accepted":
		var a Acceptance
		if err := json.Unmarshal(e.Payload, &a); err != nil {
			return err
		}
		st.Revision = a.Revision
	case "task.created", "task.updated", "task.completed", "task.reopened", "task.dropped":
		var p TaskChange
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if p.Record.Task.ID != e.EntityID || !model.ValidID(e.EntityID) {
			return fmt.Errorf("invalid task projection identity")
		}
		st.Tasks[e.EntityID] = p.Record
	case "capture.created", "capture.assigned", "capture.expired":
		var p model.Capture
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if p.ID != e.EntityID {
			return fmt.Errorf("invalid capture identity")
		}
		st.Captures[p.ID] = p
	case "proposal.created", "proposal.accepted", "proposal.rejected", "proposal.superseded":
		var p model.Proposal
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if p.ID != e.EntityID {
			return fmt.Errorf("invalid proposal identity")
		}
		st.Proposals[p.ID] = p
	case "timer.scheduled", "timer.due", "timer.cancelled":
		var p model.Timer
		if err := json.Unmarshal(e.Payload, &p); err != nil {
			return err
		}
		if p.ID != e.EntityID {
			return fmt.Errorf("invalid timer identity")
		}
		st.Timers[p.ID] = p
	default:
		return fmt.Errorf("unsupported event %s.%s at %d", e.Subject, e.Verb, e.ID)
	}
	st.LastEventID = e.ID
	return nil
}
func readEvents(ctx context.Context, q interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}) ([]Event, error) {
	rows, err := q.QueryContext(ctx, "SELECT id,event_version,ts,subject,verb,actor,entity_id,command_id,payload FROM events ORDER BY id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		var ts, payload string
		if err = rows.Scan(&e.ID, &e.Version, &ts, &e.Subject, &e.Verb, &e.Actor, &e.EntityID, &e.CommandID, &payload); err != nil {
			return nil, err
		}
		e.Payload = json.RawMessage(payload)
		e.TS, err = time.Parse(time.RFC3339Nano, ts)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
func (s *Store) Events(ctx context.Context) ([]Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return readEvents(ctx, s.db)
}
func (s *Store) Replay(ctx context.Context) (model.State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	empty := model.Empty()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return empty, err
	}
	defer tx.Rollback()
	events, err := readEvents(ctx, tx)
	if err != nil {
		return empty, err
	}
	st := model.Empty()
	// Validate/reduce entirely before replacing either projection.
	for _, e := range events {
		if err = Apply(&st, e); err != nil {
			return empty, err
		}
	}
	if _, err = tx.ExecContext(ctx, "DELETE FROM commands"); err != nil {
		return empty, err
	}
	for _, e := range events {
		if e.Subject == "command" && e.Verb == "accepted" {
			var a Acceptance
			_ = json.Unmarshal(e.Payload, &a)
			if _, err = tx.ExecContext(ctx, "INSERT INTO commands VALUES(?,?,?)", e.CommandID, a.Hash, string(a.Result)); err != nil {
				return empty, err
			}
		}
	}
	b, err := json.Marshal(st)
	if err != nil {
		return empty, err
	}
	if _, err = tx.ExecContext(ctx, "UPDATE projection_state SET body=? WHERE id=1", string(b)); err != nil {
		return empty, err
	}
	if err = tx.Commit(); err != nil {
		return empty, err
	}
	return st, nil
}
