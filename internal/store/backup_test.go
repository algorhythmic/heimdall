package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func TestSchemaTwoBackupAndUpgrade(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = s.db.Exec(`UPDATE projection_state SET body=json_remove(body,'$.contracts','$.contract_heads','$.decisions','$.resources','$.checkpoints','$.checkpoint_heads'); PRAGMA user_version=2;`); err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	files, err := filepath.Glob(filepath.Join(dir, "backups", fmt.Sprintf("pre-schema-%d-*.db", SchemaVersion)))
	if err != nil || len(files) != 1 {
		t.Fatal(files, err)
	}
	old, err := sql.Open("sqlite", files[0])
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	var version int
	if err = old.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != 2 {
		t.Fatal("backup was not pre-upgrade", version, err)
	}
	var check string
	if err = old.QueryRow("PRAGMA integrity_check").Scan(&check); err != nil || check != "ok" {
		t.Fatal(check, err)
	}
	output := filepath.Join(t.TempDir(), "snapshot.db")
	if err = s.Backup(context.Background(), output); err != nil {
		t.Fatal(err)
	}
	bytes, _ := os.ReadFile(output)
	if err = s.Backup(context.Background(), output); err == nil {
		t.Fatal("backup overwrote existing output")
	}
	after, _ := os.ReadFile(output)
	if string(bytes) != string(after) {
		t.Fatal("backup modified existing output")
	}
	if _, err = s.Replay(context.Background()); err != nil {
		t.Fatal(err)
	}
}
