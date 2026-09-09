package hyprland_test

import (
	"context"
	"encoding/json"
	"heimdall/internal/adapters/hyprland"
	"heimdall/internal/model"
	"heimdall/internal/testdesktop"
	"strings"
	"testing"
	"time"
)

func setup(t *testing.T) (*hyprland.Observer, *testdesktop.Fake) {
	t.Helper()
	f := testdesktop.New()
	o := hyprland.New()
	o.Connector = f.Connect
	o.Configure(model.DesktopSource{ID: model.NewID(), Active: true, SocketDir: "/synthetic/hypr", Epoch: strings.Repeat("a", 64), Host: strings.Repeat("c", 64)})
	t.Cleanup(o.Close)
	return o, f
}
func fresh(t *testing.T, o *hyprland.Observer) hyprland.Status {
	t.Helper()
	s, err := o.Read(context.Background(), true)
	if err != nil || !s.Fresh {
		t.Fatal(s, err)
	}
	return s
}
func await(t *testing.T, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("observer did not converge")
}
func TestIdentityDuplicatesMovesReuseAndGaps(t *testing.T) {
	o, f := setup(t)
	first := fresh(t, o)
	if len(first.Snapshot.Windows) != 2 || first.Snapshot.Windows[0].Identity == first.Snapshot.Windows[1].Identity {
		t.Fatal("duplicate title merged windows")
	}
	f.Set("clients", []map[string]any{testdesktop.Window("18000001", "0x64", 2, "review"), testdesktop.Window("18000002", "0x65", 1, "planning")})
	f.Emit("movewindowv2>>64,2,review")
	await(t, func() bool { s, _ := o.Read(context.Background(), false); return !s.Fresh })
	moved := fresh(t, o)
	if moved.Snapshot.Windows[0].Identity != first.Snapshot.Windows[0].Identity || moved.Snapshot.Windows[0].WorkspaceID != 2 {
		t.Fatal("move changed identity or lost named workspace")
	}
	f.Windows("18000003", "18000002")
	f.Emit("closewindow>>64")
	f.Emit("openwindow>>64,planning,same-app,Same title")
	reused := fresh(t, o)
	if reused.Snapshot.Windows[1].Identity == first.Snapshot.Windows[0].Identity {
		t.Fatal("address reuse inherited identity")
	}
	f.Disconnect()
	await(t, func() bool { s, _ := o.Read(context.Background(), false); return s.Gaps > 0 && !s.Fresh })
	recovered := fresh(t, o)
	if recovered.Snapshot.ID != reused.Snapshot.ID || recovered.Gaps != 1 {
		t.Fatal("gap reconciliation changed native identity")
	}
	f.ChangeEpoch()
	f.Disconnect()
	await(t, func() bool { s, _ := o.Read(context.Background(), false); return !s.Fresh })
	changed, err := o.Read(context.Background(), true)
	if err == nil || changed.Fresh || changed.Snapshot.ID != reused.Snapshot.ID {
		t.Fatal("source restart silently adopted or erased last good observation", err)
	}
}
func TestBootstrapEventReconciliationAndMalformedStream(t *testing.T) {
	o, f := setup(t)
	once := true
	f.Hook = func(command string) {
		if once && command == "clients" {
			once = false
			f.Windows("18000003")
			f.Emit("openwindow>>64,planning,same-app,Same title")
		}
	}
	s := fresh(t, o)
	if len(s.Snapshot.Windows) != 1 || s.Snapshot.Windows[0].Identity.StableID != "18000003" {
		t.Fatal("bootstrap buffered update lost")
	}
	f.Emit("not-an-event")
	await(t, func() bool { s, _ := o.Read(context.Background(), false); return s.Gaps == 1 && !s.Fresh })
	fresh(t, o)
	f.Emit(strings.Repeat("x", 70<<10))
	await(t, func() bool { s, _ := o.Read(context.Background(), false); return s.Gaps == 2 })
	fresh(t, o)
}
func TestInconsistentAndUnsupportedInventoriesKeepLastGood(t *testing.T) {
	for _, which := range []string{"duplicate_id", "duplicate_address", "missing_id", "bad_workspace", "empty_monitors", "wrong_version"} {
		t.Run(which, func(t *testing.T) {
			o, f := setup(t)
			good := fresh(t, o)
			switch which {
			case "duplicate_id":
				f.Windows("18000001", "18000001")
			case "duplicate_address":
				f.Set("clients", []map[string]any{testdesktop.Window("18000001", "0x64", 1, "planning"), testdesktop.Window("18000002", "0x64", 1, "planning")})
			case "missing_id":
				f.Windows("")
			case "bad_workspace":
				f.Set("clients", []map[string]any{testdesktop.Window("18000001", "0x64", 1, "WRONG")})
			case "empty_monitors":
				f.Set("monitors", []any{})
			case "wrong_version":
				f.Set("version", map[string]any{"version": "0.55.0"})
				f.Disconnect()
				await(t, func() bool { s, _ := o.Read(context.Background(), false); return !s.Fresh })
			}
			s, err := o.Read(context.Background(), true)
			if err == nil || s.Fresh || s.Snapshot.ID != good.Snapshot.ID {
				t.Fatal("invalid inventory accepted or last good lost", err)
			}
		})
	}
}
func TestPeriodicReconciliationRepairsSilentDropAndReadCopies(t *testing.T) {
	o, f := setup(t)
	s := fresh(t, o)
	s.Snapshot.Windows[0].Title = "tampered"
	cached, _ := o.Read(context.Background(), false)
	if cached.Snapshot.Windows[0].Title == "tampered" {
		t.Fatal("shared cache escaped")
	}
	f.Windows("18000009") // Intentionally omit the event: source streams are unsequenced.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { defer close(done); o.Run(ctx) }()
	defer func() { cancel(); <-done }()
	deadline := time.Now().Add(7 * time.Second)
	for time.Now().Before(deadline) {
		v, _ := o.Read(ctx, false)
		if v.Snapshot != nil && len(v.Snapshot.Windows) == 1 && v.Snapshot.Windows[0].Identity.StableID == "18000009" {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("periodic reconciliation missed silent event drop")
}
func TestInventoryBoundsAndTerminalText(t *testing.T) {
	o, f := setup(t)
	w := testdesktop.Window("18000001", "0x64", 1, "planning")
	w["title"] = "evil\x1b[2J\u202e" + strings.Repeat("界", 500000)
	f.Set("clients", []any{w})
	s := fresh(t, o)
	title := s.Snapshot.Windows[0].Title
	if len(title) > 512 || strings.ContainsAny(title, "\x1b\u202e") {
		t.Fatal("unsafe display title")
	}
	raw, _ := json.Marshal(s)
	if len(raw) > 512<<10 {
		t.Fatal("unbounded snapshot")
	}
}
