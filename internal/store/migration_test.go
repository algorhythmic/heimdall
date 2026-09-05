package store

import (
	"context"
	"heimdall/internal/model"
	"reflect"
	"testing"
	"time"
)

func TestLegacyProjectionUpgradePreservesState(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	now := time.Now().UTC()
	_, err = s.Transact(ctx, "legacy-capture", "cli", []byte("legacy"), now, func(st model.State) (Change, error) {
		return Change{Events: []Pending{{Subject: "capture", Verb: "created", EntityID: "legacy", Payload: model.Capture{ID: "legacy", Targets: []string{"unassigned"}, CreatedAt: now}}}, Result: map[string]string{"id": "legacy"}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	before, err := s.State(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Version 1 uses the same event envelope/tables but predates browser projections.
	_, err = s.db.Exec(`UPDATE projection_state SET body=json_remove(body,'$.browsers','$.browser_operations'); PRAGMA user_version=1;`)
	if err != nil {
		t.Fatal(err)
	}
	s.Close()
	s, err = Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	state, err := s.State(context.Background())
	if err != nil || state.Browsers == nil || state.BrowserOperations == nil {
		t.Fatal("legacy normalization failed", err)
	}
	if !reflect.DeepEqual(before, state) {
		t.Fatal("upgrade changed existing projection")
	}
	var version int
	if err = s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil || version != SchemaVersion {
		t.Fatal("downgrade guard missing", version, err)
	}
	replayed, err := s.Replay(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, replayed) {
		t.Fatal("upgrade changed replay")
	}
}
