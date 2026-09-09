// Package hyprland implements a fixed, read-only IPC vocabulary. There is no
// dispatcher, shell command, focus/move/close or process-launch path here.
package hyprland

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"path/filepath"
	"time"
)

type Connection interface {
	Source() Source
	Read(context.Context, string) ([]byte, error)
	Events() io.Reader
	Close() error
}
type Source struct{ SocketDir, Host, Epoch, Version string }
type Connector func(context.Context, string) (Connection, error)
type socketConnection struct {
	source       Source
	commandEpoch string
	events       net.Conn
}

func hash(s string) string                    { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }
func (c *socketConnection) Source() Source    { return c.source }
func (c *socketConnection) Events() io.Reader { return c.events }
func (c *socketConnection) Close() error      { return c.events.Close() }
func (c *socketConnection) Read(ctx context.Context, command string) ([]byte, error) {
	switch command {
	case "clients", "workspaces", "monitors", "version":
	default:
		return nil, fmt.Errorf("unsupported read command")
	}
	conn, epoch, _, _, err := dial(ctx, filepath.Join(c.source.SocketDir, ".socket.sock"))
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if epoch != c.commandEpoch {
		return nil, fmt.Errorf("source_changed")
	}
	deadline := time.Now().Add(time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	conn.SetDeadline(deadline)
	if _, err = io.WriteString(conn, "j/"+command); err != nil {
		return nil, fmt.Errorf("source_request_failed")
	}
	b, err := io.ReadAll(io.LimitReader(conn, 2<<20+1))
	if err != nil {
		return nil, fmt.Errorf("source_read_failed")
	}
	if len(b) > 2<<20 {
		return nil, fmt.Errorf("inventory_limit")
	}
	return b, nil
}
func Connect(ctx context.Context, dir string) (Connection, error) {
	if err := validDir(dir); err != nil {
		return nil, err
	}
	// Subscribe first: all bootstrap queries happen while the event reader is
	// connected. The manager drains buffered events before accepting a snapshot.
	events, eventEpoch, eventPeer, host, err := dial(ctx, filepath.Join(dir, ".socket2.sock"))
	if err != nil {
		return nil, err
	}
	command, commandEpoch, commandPeer, host2, err := dial(ctx, filepath.Join(dir, ".socket.sock"))
	if err != nil {
		events.Close()
		return nil, err
	}
	command.Close()
	if eventPeer != commandPeer || host != host2 {
		events.Close()
		return nil, fmt.Errorf("source_peer_mismatch")
	}
	return &socketConnection{source: Source{SocketDir: dir, Host: host, Epoch: hash("hyprland-v1/" + eventEpoch + "/" + commandEpoch)}, commandEpoch: commandEpoch, events: events}, nil
}
