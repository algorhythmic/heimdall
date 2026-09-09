package hyprland

import (
	"context"
	"fmt"
	"heimdall/internal/model"
	"io"
	"net"
	"strings"
	"testing"
	"time"
)

func TestNativeDispatchVocabularyAndStableIdentity(t *testing.T) {
	window := model.WindowIdentity{SourceEpoch: strings.Repeat("a", 64), StableID: "18000001"}
	for kind, want := range map[string]string{"focus": "dispatch focuswindow stableid:18000001", "close": "dispatch closewindow stableid:18000001", "move": "dispatch movetoworkspacesilent name:planning,stableid:18000001"} {
		c := Command{Kind: kind, Window: window}
		if kind == "move" {
			c.Workspace = "planning"
		}
		got, err := c.wire()
		if err != nil || got != want {
			t.Fatal(got, err)
		}
	}
	for _, c := range []Command{{Kind: "kill", Window: window}, {Kind: "exec", Window: window}, {Kind: "close", Window: window, Workspace: "wrong"}, {Kind: "move", Window: window, Workspace: "planning;dispatch exit"}, {Kind: "move", Window: window, Workspace: "planning,active"}, {Kind: "move", Window: window, Workspace: "planning\nexit"}, {Kind: "focus", Window: model.WindowIdentity{SourceEpoch: window.SourceEpoch, StableID: "active"}}} {
		if _, err := c.wire(); err == nil {
			t.Fatal("unsafe command accepted", c)
		}
	}
}

func TestNativeDispatchLuaVocabularyHasExplicitSelectors(t *testing.T) {
	window := model.WindowIdentity{SourceEpoch: strings.Repeat("a", 64), StableID: "18000001"}
	for kind, want := range map[string]string{"focus": `dispatch hl.dsp.focus({window="stableid:18000001"})`, "close": `dispatch hl.dsp.window.close({window="stableid:18000001"})`, "move": `dispatch hl.dsp.window.move({window="stableid:18000001",workspace="name:planning",follow=false})`} {
		c := Command{Kind: kind, Window: window}
		if kind == "move" {
			c.Workspace = "planning"
		}
		got, err := c.wireFor("lua")
		if err != nil || got != want {
			t.Fatal(got, err)
		}
		if _, err := c.wireFor("unknown"); err == nil {
			t.Fatal("unknown provider admitted")
		}
	}
	for _, workspace := range []string{`x"});os.execute("bad")--`, "x\\n", "x\n", "x,window=active", "x;exit", "x)"} {
		if _, err := (Command{Kind: "move", Window: window, Workspace: workspace}).wireFor("lua"); err == nil {
			t.Fatal("Lua injection admitted", workspace)
		}
	}
}
func preparedFixture() (*preparedCommand, net.Conn) {
	client, server := net.Pipe()
	observed := Status{Fresh: true, Snapshot: &model.DesktopSnapshot{ID: strings.Repeat("a", 64), CapturedAt: time.Now()}}
	return &preparedCommand{observer: &Observer{status: observed}, observed: observed, command: "dispatch closewindow stableid:18000001", conn: client, began: time.Now()}, server
}
func TestNativeDispatchCommitsBeforeInputAndNeverRepeats(t *testing.T) {
	p, server := preparedFixture()
	defer p.Close()
	defer server.Close()
	committed := false
	received := make(chan string, 1)
	go func() {
		raw := make([]byte, 200)
		n, _ := server.Read(raw)
		received <- string(raw[:n])
		io.WriteString(server, "ok")
		server.Close()
	}()
	result := p.Send(context.Background(), func() error {
		select {
		case <-received:
			t.Error("input preceded durable callback")
		default:
		}
		committed = true
		return nil
	})
	if !committed || !result.Submitted || !result.Acknowledged || <-received != p.command {
		t.Fatal(result)
	}
	if got := p.Send(context.Background(), func() error { t.Fatal("second dispatch callback invoked"); return nil }); got.Submitted {
		t.Fatal("input repeated")
	}
}
func TestNativeDispatchRefusesBeforeJournalAndRetainsLostAck(t *testing.T) {
	for _, reason := range []string{"callback", "expired", "observation", "nil"} {
		t.Run(reason, func(t *testing.T) {
			p, server := preparedFixture()
			defer p.Close()
			defer server.Close()
			callback := func() error { return nil }
			switch reason {
			case "callback":
				callback = func() error { return fmt.Errorf("authority changed") }
			case "expired":
				p.began = time.Now().Add(-3 * time.Second)
			case "observation":
				p.observer.status.Fresh = false
			case "nil":
				callback = nil
			}
			got := p.Send(context.Background(), callback)
			if got.Submitted || got.Acknowledged {
				t.Fatal(got)
			}
			server.SetReadDeadline(time.Now().Add(time.Millisecond))
			b := make([]byte, 100)
			n, _ := server.Read(b)
			if n != 0 {
				t.Fatal("input preceded authority", string(b[:n]))
			}
		})
	}
	p, server := preparedFixture()
	defer p.Close()
	go func() { b := make([]byte, 200); server.Read(b); server.Close() }()
	got := p.Send(context.Background(), func() error { return nil })
	if !got.Submitted || got.Acknowledged {
		t.Fatal("lost ACK reported verified", got)
	}
}
