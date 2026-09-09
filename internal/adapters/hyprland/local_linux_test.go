//go:build linux

package hyprland

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestFixedReadVocabularyAndSocketEpoch(t *testing.T) {
	dir := t.TempDir()
	var mu sync.Mutex
	commands := []string{}
	var connections []net.Conn
	start := func(name string) *net.UnixListener {
		l, err := net.ListenUnix("unix", &net.UnixAddr{Name: filepath.Join(dir, name), Net: "unix"})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { l.Close() })
		go func() {
			for {
				c, err := l.Accept()
				if err != nil {
					return
				}
				mu.Lock()
				connections = append(connections, c)
				mu.Unlock()
				if name == ".socket.sock" {
					go func() {
						defer c.Close()
						b := make([]byte, 100)
						n, _ := c.Read(b)
						if n > 0 {
							mu.Lock()
							commands = append(commands, string(b[:n]))
							mu.Unlock()
							io.WriteString(c, `{"version":"0.56.2"}`)
						}
					}()
				}
			}
		}()
		return l
	}
	start(".socket2.sock")
	command := start(".socket.sock")
	t.Cleanup(func() {
		mu.Lock()
		defer mu.Unlock()
		for _, c := range connections {
			c.Close()
		}
	})
	c, err := Connect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err = c.Read(context.Background(), "version"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"dispatch closewindow", "version; exec anything", "[batch]", "keyword"} {
		if _, err = c.Read(context.Background(), bad); err == nil {
			t.Fatal("non-read command accepted")
		}
	}
	mu.Lock()
	calls := append([]string{}, commands...)
	mu.Unlock()
	if len(calls) != 1 || calls[0] != "j/version" {
		t.Fatal("unexpected IPC vocabulary", calls)
	}
	old := c.Source().Epoch
	command.Close()
	start(".socket.sock")
	if _, err = c.Read(context.Background(), "clients"); err == nil {
		t.Fatal("socket replacement reused epoch")
	}
	c2, err := Connect(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer c2.Close()
	if old == c2.Source().Epoch {
		t.Fatal("same-process socket replacement had same epoch")
	}
	os.Chmod(dir, 0777)
	if _, err := Connect(context.Background(), dir); err == nil {
		t.Fatal("unsafe directory accepted")
	}
	os.Chmod(dir, 0700)
}
func TestNativeReadOnlyMetadata(t *testing.T) {
	dir := os.Getenv("HEIMDALL_TEST_HYPRLAND_DIR")
	if dir == "" {
		t.Skip("explicit native observer test socket directory not supplied")
	}
	s, err := Probe(context.Background(), dir, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !s.Fresh || s.Snapshot.SourceEpoch == "" || s.Snapshot.CompositorVersion != "0.56.2" {
		t.Fatal("native source identity unavailable")
	}
	b, err := json.Marshal(s)
	if err != nil || len(b) > 512<<10 {
		t.Fatal("native response exceeds bound", err)
	}
	// Deliberately print no titles, application names, addresses or workspace names.
	t.Logf("Hyprland %s: monitors=%d workspaces=%d windows=%d, fresh=%t", s.Snapshot.CompositorVersion, len(s.Snapshot.Monitors), len(s.Snapshot.Workspaces), len(s.Snapshot.Windows), s.Fresh)
	if strings.Contains(s.Coverage, "atomic") {
		t.Fatal("unsequenced source cannot claim atomic coverage")
	}
}
