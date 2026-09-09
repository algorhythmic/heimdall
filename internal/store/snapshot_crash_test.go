package store

import (
	"bufio"
	"context"
	"fmt"
	"heimdall/internal/model"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestSnapshotCrashWorker(t *testing.T) {
	dir := os.Getenv("HEIMDALL_SNAPSHOT_CRASH_DIR")
	if dir == "" {
		t.Skip("subprocess helper")
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	stage := os.Getenv("HEIMDALL_SNAPSHOT_CRASH_STAGE")
	id := os.Getenv("HEIMDALL_SNAPSHOT_CRASH_ID")
	s.testSnapshotHook = func(at string) {
		if at == stage {
			fmt.Println("SNAPSHOT_READY " + stage)
			time.Sleep(30 * time.Second)
		}
	}
	now := time.Now().UTC()
	if _, err = s.Transact(context.Background(), "snapshot-"+id, "snapshotter", []byte(id), now, func(st model.State) (Change, error) { return snapshotChange(st, id, 9, now), nil }); err != nil {
		t.Fatal(err)
	}
}
func TestSnapshotProcessKillPublicationBoundaries(t *testing.T) {
	for _, stage := range []string{"payload", "before_commit", "after_commit"} {
		t.Run(stage, func(t *testing.T) {
			s, dir := snapshotStore(t, 1)
			old := saveTestSnapshot(t, s, 0)
			s.Close()
			next := model.NewID()
			cmd := exec.Command(os.Args[0], "-test.run=^TestSnapshotCrashWorker$", "-test.v")
			cmd.Env = append(os.Environ(), "HEIMDALL_SNAPSHOT_CRASH_DIR="+dir, "HEIMDALL_SNAPSHOT_CRASH_STAGE="+stage, "HEIMDALL_SNAPSHOT_CRASH_ID="+next)
			output, err := cmd.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			var errors strings.Builder
			cmd.Stderr = &errors
			if err = cmd.Start(); err != nil {
				t.Fatal(err)
			}
			defer cmd.Process.Kill()
			ready := make(chan bool, 1)
			go func() {
				scanner := bufio.NewScanner(output)
				for scanner.Scan() {
					if scanner.Text() == "SNAPSHOT_READY "+stage {
						ready <- true
						return
					}
				}
				ready <- false
			}()
			select {
			case ok := <-ready:
				if !ok {
					cmd.Wait()
					t.Fatal("child exited before publication boundary", errors.String())
				}
			case <-time.After(10 * time.Second):
				cmd.Process.Kill()
				cmd.Wait()
				t.Fatal("publication boundary timeout", errors.String())
			}
			if err = cmd.Process.Kill(); err != nil {
				t.Fatal(err)
			}
			cmd.Wait()
			reopened, err := Open(dir)
			if err != nil {
				t.Fatal(err)
			}
			defer reopened.Close()
			st, err := reopened.State(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			want := old
			count := 1
			if stage == "after_commit" {
				want = next
				count = 2
			}
			if st.SnapshotHeads["alpha"].ID != want {
				t.Fatal("head did not match commit boundary", stage, st.SnapshotHeads["alpha"].ID, want)
			}
			point, err := reopened.WorkspacePoint(context.Background(), "alpha", want)
			if err != nil || point.Payload == nil {
				t.Fatal("surviving head lacks complete payload", err)
			}
			var integrity string
			reopened.db.QueryRow("PRAGMA integrity_check").Scan(&integrity)
			if integrity != "ok" {
				t.Fatal(integrity)
			}
			for _, table := range []string{"workspace_snapshots", "workspace_snapshot_payloads"} {
				var got int
				reopened.db.QueryRow("SELECT count(*) FROM " + table).Scan(&got)
				if got != count {
					t.Fatal("partial publication survived", table, got, count)
				}
			}
			if _, err := reopened.Replay(context.Background()); err != nil {
				t.Fatal("killed store did not replay", err)
			}
		})
	}
}
