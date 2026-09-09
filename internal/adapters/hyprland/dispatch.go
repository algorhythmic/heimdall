package hyprland

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"io"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
)

// Command is deliberately smaller than Hyprland's dispatcher vocabulary.
// Window selection always uses the compositor's stable ID within a pinned epoch.
type Command struct {
	Kind      string               `json:"kind"`
	Window    model.WindowIdentity `json:"window"`
	Workspace string               `json:"workspace,omitempty"`
}

const DispatchCommit = "efb50993780079460b0cbed1363e2166a2de1d9f"

func (c Command) wire() (string, error) {
	if c.Window.Validate() != nil {
		return "", fmt.Errorf("exact native window identity required")
	}
	selector := "stableid:" + c.Window.StableID
	if c.Kind != "move" && c.Workspace != "" {
		return "", fmt.Errorf("workspace is valid only for move")
	}
	switch c.Kind {
	case "focus":
		return "dispatch focuswindow " + selector, nil
	case "close":
		return "dispatch closewindow " + selector, nil
	case "move":
		if c.Workspace == "" || len(c.Workspace) > 256 || strings.TrimSpace(c.Workspace) != c.Workspace || strings.ContainsFunc(c.Workspace, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r) && !strings.ContainsRune(" _./-", r)
		}) {
			return "", fmt.Errorf("explicit safe workspace name required")
		}
		return "dispatch movetoworkspacesilent name:" + c.Workspace + "," + selector, nil
	default:
		return "", fmt.Errorf("unsupported native action")
	}
}

// The Lua provider's dispatch endpoint accepts one dispatcher constructor.
// These fixed templates admit no caller-supplied Lua code, optional selector,
// shell command, function name or eval/repl endpoint.
func (c Command) wireFor(provider string) (string, error) {
	legacy, err := c.wire()
	if err != nil {
		return "", err
	}
	if provider == "hyprlang" {
		return legacy, nil
	}
	if provider != "lua" {
		return "", fmt.Errorf("unsupported compositor configuration provider")
	}
	window := `window="stableid:` + c.Window.StableID + `"`
	switch c.Kind {
	case "focus":
		return `dispatch hl.dsp.focus({` + window + `})`, nil
	case "close":
		return `dispatch hl.dsp.window.close({` + window + `})`, nil
	case "move":
		return `dispatch hl.dsp.window.move({` + window + `,workspace="name:` + c.Workspace + `",follow=false})`, nil
	}
	return "", fmt.Errorf("unsupported native action")
}

type DispatchReceipt struct {
	// submitted means bytes may have reached the compositor. It is independent
	// of API acknowledgment, window closure, application data and verification.
	Submitted    bool   `json:"submitted"`
	Acknowledged bool   `json:"acknowledged"`
	Detail       string `json:"detail"`
}
type PreparedCommand interface {
	Observation() Status
	Check() error
	Send(context.Context, func() error) DispatchReceipt
	Close() error
}
type Dispatcher struct{ Observer *Observer }
type preparedCommand struct {
	mu       sync.Mutex
	observer *Observer
	observed Status
	command  string
	conn     net.Conn
	began    time.Time
	used     bool
}

// Prepare performs reads and opens the authenticated command socket before the
// caller takes the durable writer. It sends no dispatcher bytes.
func (d Dispatcher) Prepare(ctx context.Context, source model.DesktopSource, c Command) (PreparedCommand, error) {
	wire, err := c.wire()
	if err != nil {
		return nil, err
	}
	if d.Observer == nil || !source.Active || source.Epoch != c.Window.SourceEpoch || source.CompositorVersion != "0.56.2" {
		return nil, fmt.Errorf("selected supported source required")
	}
	observed, err := d.Observer.Read(ctx, true)
	if err != nil {
		return nil, err
	}
	if !observed.Fresh || observed.Snapshot == nil || observed.Snapshot.SourceEpoch != source.Epoch {
		return nil, fmt.Errorf("fresh selected compositor required")
	}
	found := false
	for _, w := range observed.Snapshot.Windows {
		if w.Identity == c.Window {
			found = true
		}
	}
	if !found {
		return nil, fmt.Errorf("exact owned window is absent")
	}
	if c.Kind == "move" {
		count := 0
		for _, ws := range observed.Snapshot.Workspaces {
			if ws.Name == c.Workspace {
				count++
			}
		}
		if count != 1 {
			return nil, fmt.Errorf("destination workspace is absent or ambiguous")
		}
	}
	connection, err := Connect(ctx, source.SocketDir)
	if err != nil {
		return nil, err
	}
	defer connection.Close()
	local, ok := connection.(*socketConnection)
	if !ok || local.source.Epoch != source.Epoch {
		return nil, fmt.Errorf("compositor changed before dispatch preparation")
	}
	version, err := connection.Read(ctx, "version")
	var build struct {
		Version, Commit string
		Dirty           bool
	}
	if err != nil || json.Unmarshal(version, &build) != nil || build.Version != "0.56.2" || build.Commit != DispatchCommit || build.Dirty {
		return nil, fmt.Errorf("native dispatch requires the pinned clean Hyprland build")
	}
	status, err := connection.Read(ctx, "status")
	var provider struct {
		ConfigProvider string `json:"configProvider"`
	}
	if err != nil || json.Unmarshal(status, &provider) != nil {
		return nil, fmt.Errorf("native dispatch requires an observed configuration provider")
	}
	wire, err = c.wireFor(provider.ConfigProvider)
	if err != nil {
		return nil, err
	}
	conn, epoch, _, _, err := dial(ctx, filepath.Join(source.SocketDir, ".socket.sock"))
	if err != nil {
		return nil, err
	}
	if epoch != local.commandEpoch {
		conn.Close()
		return nil, fmt.Errorf("compositor command socket changed")
	}
	return &preparedCommand{observer: d.Observer, observed: observed, command: wire, conn: conn, began: time.Now()}, nil
}
func (p *preparedCommand) Observation() Status { return model.Clone(p.observed) }
func (p *preparedCommand) Close() error        { return p.conn.Close() }

// Check performs no IPC and is safe in a transaction's authorization callback.
func (p *preparedCommand) Check() error {
	if time.Since(p.began) < 0 || time.Since(p.began) >= 2*time.Second {
		return fmt.Errorf("native preparation expired")
	}
	return p.observer.Check(p.observed.Snapshot.ID)
}

// The callback must commit a still-authorized shared action dispatch before
// returning. A prepared socket is single use, including after callback failure.
func (p *preparedCommand) Send(ctx context.Context, commitDispatch func() error) DispatchReceipt {
	p.mu.Lock()
	if p.used {
		p.mu.Unlock()
		return DispatchReceipt{Detail: "prepared command already consumed"}
	}
	p.used = true
	p.mu.Unlock()
	if commitDispatch == nil {
		return DispatchReceipt{Detail: "durable dispatch callback required"}
	}
	if err := ctx.Err(); err != nil {
		return DispatchReceipt{Detail: err.Error()}
	}
	if err := p.Check(); err != nil {
		return DispatchReceipt{Detail: err.Error()}
	}
	if err := commitDispatch(); err != nil {
		return DispatchReceipt{Detail: err.Error()}
	}
	if err := p.Check(); err != nil {
		return DispatchReceipt{Detail: err.Error()}
	}
	// Input after the durable dispatch boundary is conservatively uncertain if
	// cancellation, timeout or disconnect now intervenes. No caller may retry it.
	if err := ctx.Err(); err != nil {
		return DispatchReceipt{Detail: err.Error()}
	}
	deadline := time.Now().Add(time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	p.conn.SetDeadline(deadline)
	n, err := io.WriteString(p.conn, p.command)
	result := DispatchReceipt{Submitted: n > 0}
	if err != nil {
		result.Detail = "native dispatch write failed"
		return result
	}
	result.Submitted = true
	raw, err := io.ReadAll(io.LimitReader(p.conn, 4097))
	if err != nil {
		result.Detail = "native dispatch acknowledgment unavailable"
		return result
	}
	if len(raw) > 4096 {
		result.Detail = "native acknowledgment exceeded limit"
		return result
	}
	result.Acknowledged = strings.TrimSpace(string(raw)) == "ok"
	if result.Acknowledged {
		result.Detail = "Compositor acknowledged request; postcondition not asserted"
	} else {
		result.Detail = "Compositor refused request"
	}
	return result
}
