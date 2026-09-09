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
	"testing"
)

// This is a stopped database dump from the actual 0.7.0 binary, not a current
// projection relabeled with an old marker. Retain it through future migrations.
func TestSchemaSixPublishedFixture(t *testing.T) {
	dir := t.TempDir()
	raw, err := os.ReadFile("../../testdata/continuity/schema6.sql")
	if err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "heimdall.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(string(raw)); err != nil {
		db.Close()
		t.Fatal(err)
	}
	var original string
	if err = db.QueryRow("SELECT body FROM projection_state WHERE id=1").Scan(&original); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	var expected model.State
	if err = json.Unmarshal([]byte(original), &expected); err != nil {
		t.Fatal(err)
	}
	expected.Normalize()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var version int
	if err = s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != SchemaVersion {
		t.Fatal("current schema marker not published", version, err)
	}
	backups, err := filepath.Glob(filepath.Join(dir, "backups", fmt.Sprintf("pre-schema-%d-*.db", SchemaVersion)))
	if err != nil || len(backups) != 1 {
		t.Fatal("missing unique pre-upgrade backup", backups, err)
	}
	backup, err := sql.Open("sqlite", backups[0])
	if err != nil {
		t.Fatal(err)
	}
	defer backup.Close()
	var backupBody, integrity string
	if err = backup.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 6 {
		t.Fatal("backup marker changed", version, err)
	}
	if err = backup.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatal("backup integrity", integrity, err)
	}
	if err = backup.QueryRow("SELECT body FROM projection_state WHERE id=1").Scan(&backupBody); err != nil || backupBody != original {
		t.Fatal("backup changed schema-6 projection", err)
	}
	// Compare immutable events and exact retry receipts, not just task state.
	oldEvents, err := readEvents(context.Background(), backup)
	if err != nil {
		t.Fatal(err)
	}
	newEvents, err := s.Events(context.Background())
	if err != nil || !reflect.DeepEqual(oldEvents, newEvents) {
		t.Fatal("migration changed events", err)
	}
	for _, event := range oldEvents {
		if event.Subject != "command" {
			continue
		}
		var oldHash, newHash, oldResult, newResult string
		if err := backup.QueryRow("SELECT request_hash,result FROM commands WHERE id=?", event.CommandID).Scan(&oldHash, &oldResult); err != nil {
			t.Fatal(err)
		}
		if err := s.db.QueryRow("SELECT request_hash,result FROM commands WHERE id=?", event.CommandID).Scan(&newHash, &newResult); err != nil || oldHash != newHash || oldResult != newResult {
			t.Fatal("migration changed receipt", err)
		}
	}
	ctx := context.Background()
	state, err := s.State(ctx)
	if err != nil || !reflect.DeepEqual(state, expected) {
		t.Fatal("schema-6 state changed on open/upgrade", err)
	}
	grant := state.Grants["00000000000000000000000000000005"]
	if grant.Version != 1 || grant.CheckpointWrite || grant.PermitsCheckpoint(state, "schema-six", state.ContractHeads["schema-six"]) || grant.Contains(state, "other-task") {
		t.Fatal("fixture read grant gained authority")
	}
	if state.CheckpointHeads["schema-six"] != "00000000000000000000000000000004" {
		t.Fatal("checkpoint head lost")
	}
	replayed, err := s.Replay(ctx)
	if err != nil || !reflect.DeepEqual(replayed, expected) {
		t.Fatal("schema-6 replay differs", err)
	}
	var receipts int
	if err = s.db.QueryRow("SELECT count(*) FROM commands").Scan(&receipts); err != nil || receipts != 6 {
		t.Fatal("command receipts not preserved", receipts, err)
	}
}

func TestSchemaSixUpgradeRequiresBackup(t *testing.T) {
	dir := t.TempDir()
	raw, err := os.ReadFile("../../testdata/continuity/schema6.sql")
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
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	// A file obstructing the backup directory simulates publication failure.
	if err = os.WriteFile(filepath.Join(dir, "backups"), []byte("preserve me"), 0600); err != nil {
		t.Fatal(err)
	}
	if s, err := Open(dir); err == nil {
		s.Close()
		t.Fatal("upgrade continued without a backup")
	}
	db, err = sql.Open("sqlite", filepath.Join(dir, "heimdall.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var version int
	if err = db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 6 {
		t.Fatal("failed upgrade changed marker", version, err)
	}
	var receipts int
	if err = db.QueryRow("SELECT count(*) FROM commands").Scan(&receipts); err != nil || receipts != 6 {
		t.Fatal("failed upgrade lost receipts", receipts, err)
	}
}
