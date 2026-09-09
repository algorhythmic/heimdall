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

func TestStoppedSchemaSevenFixtureUpgrade(t *testing.T) {
	testStoppedWorkspaceFixture(t, "../../testdata/workspace/schema7.sql", 7)
}
func TestStoppedSchemaEightFixtureUpgrade(t *testing.T) {
	testStoppedWorkspaceFixture(t, "../../testdata/artifacts/schema8.sql", 8)
}

func TestStoppedSchemaNineFixtureUpgrade(t *testing.T) {
	testStoppedWorkspaceFixture(t, "../../testdata/progress/schema9.sql", 9)
}

func TestStoppedSchemaTenFixtureUpgrade(t *testing.T) {
	testStoppedWorkspaceFixture(t, "../../testdata/progress/schema10.sql", 10)
}

func TestStoppedSchemaElevenFixtureUpgrade(t *testing.T) {
	testStoppedWorkspaceFixture(t, "../../testdata/preservation/schema11.sql", 11)
}

func TestStoppedSchemaTwelveFixtureUpgrade(t *testing.T) {
	testStoppedWorkspaceFixture(t, "../../testdata/dependencies/schema12.sql", 12)
}

func testStoppedWorkspaceFixture(t *testing.T, path string, marker int) {
	t.Helper()
	dir := t.TempDir()
	raw, err := os.ReadFile(path)
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
	var original string
	if err = db.QueryRow("SELECT body FROM projection_state WHERE id=1").Scan(&original); err != nil {
		t.Fatal(err)
	}
	db.Close()
	var expected model.State
	if err = json.Unmarshal([]byte(original), &expected); err != nil {
		t.Fatal(err)
	}
	expected.Normalize()
	if marker < 9 && (len(expected.WorkspaceManifests) == 0 || len(expected.SessionBindings) == 0) {
		t.Fatal("fixture has no W01 state")
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	actual, err := s.State(context.Background())
	if err != nil || !reflect.DeepEqual(actual, expected) {
		t.Fatal("upgrade changed W01 state", err)
	}
	replayed, err := s.Replay(context.Background())
	if err != nil || !reflect.DeepEqual(replayed, expected) {
		t.Fatal("W01 replay differs", err)
	}
	backups, err := filepath.Glob(filepath.Join(dir, "backups", fmt.Sprintf("pre-schema-%d-*.db", SchemaVersion)))
	if err != nil || len(backups) != 1 {
		t.Fatal(backups, err)
	}
	old, err := sql.Open("sqlite", backups[0])
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()
	var version int
	var body, integrity string
	if err = old.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != marker {
		t.Fatal(version, err)
	}
	if err = old.QueryRow("PRAGMA integrity_check").Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatal(integrity, err)
	}
	if err = old.QueryRow("SELECT body FROM projection_state WHERE id=1").Scan(&body); err != nil || body != original {
		t.Fatal("pre-upgrade fixture changed", err)
	}
	oldEvents, _ := readEvents(context.Background(), old)
	newEvents, _ := s.Events(context.Background())
	if !reflect.DeepEqual(oldEvents, newEvents) {
		t.Fatal("events changed")
	}
	for _, e := range oldEvents {
		if e.Subject == "command" {
			var a, b string
			old.QueryRow("SELECT result FROM commands WHERE id=?", e.CommandID).Scan(&a)
			s.db.QueryRow("SELECT result FROM commands WHERE id=?", e.CommandID).Scan(&b)
			if a == "" || a != b {
				t.Fatal("receipt changed")
			}
		}
	}
}

func TestStoppedSchemaThirteenFixtureUpgrade(t *testing.T) {
	testStoppedWorkspaceFixture(t, "../../testdata/viewport/schema13.sql", 13)
}
