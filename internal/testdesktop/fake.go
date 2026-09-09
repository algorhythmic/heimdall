// Package testdesktop supplies synthetic, content-free IPC fixtures for tests.
package testdesktop

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/adapters/hyprland"
	"io"
	"strings"
	"sync"
)

type Fake struct {
	mu              sync.Mutex
	Data            map[string][]byte
	Epoch           string
	Pipes           []*io.PipeWriter
	Reads, Connects int
	Hook            func(string)
}

func New() *Fake {
	f := &Fake{Data: map[string][]byte{}, Epoch: strings.Repeat("a", 64)}
	f.Set("version", map[string]any{"version": "0.56.2", "dirty": false})
	f.Set("monitors", []map[string]any{{"id": 0, "name": "SYNTHETIC-1", "x": 0, "y": 0, "width": 1920, "height": 1080, "scale": 1, "transform": 0, "activeWorkspace": map[string]any{"id": 1}}})
	f.Set("workspaces", []map[string]any{{"id": 1, "name": "planning", "monitor": "SYNTHETIC-1", "monitorID": 0}, {"id": 2, "name": "review", "monitor": "SYNTHETIC-1", "monitorID": 0}})
	f.Windows("18000001", "18000002")
	return f
}
func (f *Fake) Windows(ids ...string) {
	windows := []map[string]any{}
	for i, id := range ids {
		w := Window(id, fmt.Sprintf("0x%x", 100+i), 1, "planning")
		windows = append(windows, w)
	}
	f.Set("clients", windows)
}
func Window(id, address string, ws int, name string) map[string]any {
	return map[string]any{"address": address, "stableId": id, "pid": 123, "class": "same-app", "title": "Same title", "workspace": map[string]any{"id": ws, "name": name}, "monitor": 0, "at": []int{10, 20}, "size": []int{800, 600}, "mapped": true}
}
func (f *Fake) Set(command string, v any) {
	b, _ := json.Marshal(v)
	f.mu.Lock()
	defer f.mu.Unlock()
	f.Data[command] = b
}
func (f *Fake) ChangeEpoch()      { f.mu.Lock(); defer f.mu.Unlock(); f.Epoch = strings.Repeat("b", 64) }
func (f *Fake) Count() (int, int) { f.mu.Lock(); defer f.mu.Unlock(); return f.Reads, f.Connects }
func (f *Fake) Disconnect() {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, p := range f.Pipes {
		p.Close()
	}
	f.Pipes = nil
}
func (f *Fake) Emit(frame string) {
	f.mu.Lock()
	pipes := append([]*io.PipeWriter{}, f.Pipes...)
	f.mu.Unlock()
	for _, p := range pipes {
		_, _ = io.WriteString(p, frame+"\n")
	}
}
func (f *Fake) Connect(_ context.Context, dir string) (hyprland.Connection, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, w := io.Pipe()
	f.Pipes = append(f.Pipes, w)
	f.Connects++
	return &connection{f: f, epoch: f.Epoch, dir: dir, r: r}, nil
}

type connection struct {
	f          *Fake
	epoch, dir string
	r          *io.PipeReader
}

func (c *connection) Source() hyprland.Source {
	return hyprland.Source{SocketDir: c.dir, Host: strings.Repeat("c", 64), Epoch: c.epoch}
}
func (c *connection) Events() io.Reader { return c.r }
func (c *connection) Close() error      { return c.r.Close() }
func (c *connection) Read(ctx context.Context, command string) ([]byte, error) {
	c.f.mu.Lock()
	c.f.Reads++
	b := append([]byte{}, c.f.Data[command]...)
	hook := c.f.Hook
	epoch := c.f.Epoch
	c.f.mu.Unlock()
	if epoch != c.epoch {
		return nil, fmt.Errorf("source_changed")
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if hook != nil {
		hook(command)
	}
	return b, nil
}
