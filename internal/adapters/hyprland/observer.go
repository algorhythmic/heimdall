package hyprland

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"reflect"
	"strings"
	"sync"
	"time"
)

type Status struct {
	FocusKnown      bool                   `json:"focus_known,omitempty"`
	FocusedWindow   *model.WindowIdentity  `json:"focused_window,omitempty"`
	Version         int                    `json:"version"`
	Selected        bool                   `json:"selected"`
	Fresh           bool                   `json:"fresh"`
	Issue           string                 `json:"issue"`
	Gaps            uint64                 `json:"known_event_gaps"`
	Reconciliations uint64                 `json:"reconciliations"`
	Coverage        string                 `json:"coverage"`
	Snapshot        *model.DesktopSnapshot `json:"snapshot,omitempty"`
}

// The event stream has no sequence numbers. We count known disconnects,
// malformed/overlong frames and connection replacement; silent upstream drops
// cannot be counted. Full inventory reconciliation repairs observable state.
type Observer struct {
	Connector                       Connector
	refresh                         sync.Mutex
	mu                              sync.Mutex
	source                          model.DesktopSource
	conn                            Connection
	generation                      uint64
	sequence                        uint64
	dirtyAt, lastEvent, lastAttempt time.Time
	broken                          bool
	status                          Status
}

func New() *Observer {
	return &Observer{Connector: Connect, status: Status{Version: 1, Issue: "source_not_selected", Coverage: "double_inventory_with_buffered_unsequenced_events"}}
}
func (o *Observer) Configure(s model.DesktopSource) {
	o.refresh.Lock()
	defer o.refresh.Unlock()
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.source.ID == s.ID {
		return
	}
	if o.conn != nil {
		o.conn.Close()
	}
	o.conn = nil
	o.generation++
	o.source = s
	o.broken = false
	o.sequence = 0
	o.dirtyAt = time.Time{}
	o.lastAttempt = time.Time{}
	o.status = Status{Version: 1, Selected: s.Active, Issue: "awaiting_inventory", Coverage: "double_inventory_with_buffered_unsequenced_events"}
	if !s.Active {
		o.status.Issue = "source_not_selected"
	}
}
func (o *Observer) Close() { o.Configure(model.DesktopSource{}) }
func (o *Observer) Run(ctx context.Context) {
	tick := time.NewTicker(100 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			o.Close()
			return
		case <-tick.C:
			o.mu.Lock()
			now := time.Now()
			due := o.source.Active && now.Sub(o.lastAttempt) >= time.Second && (o.status.Snapshot == nil || now.Sub(o.status.Snapshot.CapturedAt) >= 5*time.Second || (!o.dirtyAt.IsZero() && (now.Sub(o.lastEvent) >= 100*time.Millisecond || now.Sub(o.dirtyAt) >= time.Second)) || o.broken)
			o.mu.Unlock()
			if due {
				_, _ = o.Read(ctx, true)
			}
		}
	}
}
func (o *Observer) frames(c Connection, generation uint64) {
	defer c.Close()
	scanner := bufio.NewScanner(c.Events())
	scanner.Buffer(make([]byte, 4096), 64<<10)
	for scanner.Scan() {
		line := scanner.Text()
		at := strings.Index(line, ">>")
		if at < 1 || at > 80 {
			break
		}
		o.mu.Lock()
		if generation != o.generation {
			o.mu.Unlock()
			return
		}
		o.sequence++
		o.status.Fresh = false
		if o.dirtyAt.IsZero() {
			o.dirtyAt = time.Now()
		}
		o.lastEvent = time.Now()
		o.mu.Unlock()
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	if generation != o.generation {
		return
	}
	o.broken = true
	o.status.Fresh = false
	o.status.Issue = "event_gap"
	o.status.Gaps++
}
func (o *Observer) fail(err error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.status.Fresh = false
	o.status.Issue = err.Error()
}
func (o *Observer) connect(ctx context.Context) error {
	o.mu.Lock()
	s := o.source
	if o.conn != nil && !o.broken {
		o.mu.Unlock()
		return nil
	}
	if o.conn != nil {
		o.conn.Close()
		o.conn = nil
	}
	o.generation++
	generation := o.generation
	o.mu.Unlock()
	if !s.Active {
		return fmt.Errorf("source_not_selected")
	}
	connector := o.Connector
	if connector == nil {
		connector = Connect
	}
	c, err := connector(ctx, s.SocketDir)
	if err != nil {
		return err
	}
	source := c.Source()
	if (s.Epoch != "" && source.Epoch != s.Epoch) || (s.Host != "" && source.Host != s.Host) {
		c.Close()
		return fmt.Errorf("source_changed_reselect_required")
	}
	o.mu.Lock()
	o.conn = c
	o.broken = false
	o.mu.Unlock()
	go o.frames(c, generation)
	var version struct {
		Version string
		Dirty   bool
	}
	b, err := c.Read(ctx, "version")
	if err != nil || json.Unmarshal(b, &version) != nil || version.Version != "0.56.2" || version.Dirty {
		o.mu.Lock()
		o.broken = true
		o.mu.Unlock()
		c.Close()
		return fmt.Errorf("unsupported_compositor_version")
	}
	return nil
}
func (o *Observer) capture(ctx context.Context, focus bool) (captureErr error) {
	o.refresh.Lock()
	defer o.refresh.Unlock()
	defer func() {
		if captureErr != nil {
			o.fail(captureErr)
		}
	}()
	o.mu.Lock()
	o.lastAttempt = time.Now()
	o.status.Fresh = false
	o.status.FocusKnown = false
	o.status.FocusedWindow = nil
	o.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := o.connect(ctx); err != nil {
		return err
	}
	for attempt := 0; attempt < 3; attempt++ {
		startedAt := time.Now().UTC()
		o.mu.Lock()
		seq, c := o.sequence, o.conn
		o.mu.Unlock()
		first, err := inventory(ctx, c)
		if err != nil {
			return err
		}
		var active1, active2 *model.WindowIdentity
		if focus {
			active1, err = activeWindow(ctx, c)
			if err != nil {
				return err
			}
		}
		second, err := inventory(ctx, c)
		if err != nil {
			return err
		}
		if focus {
			active2, err = activeWindow(ctx, c)
			if err != nil {
				return err
			}
		}
		o.mu.Lock()
		valid := !o.broken && seq == o.sequence && first.ID == second.ID && reflect.DeepEqual(active1, active2)
		if valid {
			second.StartedAt = startedAt
			second.CapturedAt = time.Now().UTC()
			o.status.Snapshot = &second
			o.status.FocusKnown, o.status.FocusedWindow = focus, active2
			o.status.Fresh = true
			o.status.Issue = ""
			o.status.Reconciliations++
			o.dirtyAt = time.Time{}
			o.mu.Unlock()
			return nil
		}
		o.mu.Unlock()
		if ctx.Err() != nil {
			return fmt.Errorf("inventory_timeout")
		}
	}
	return fmt.Errorf("inventory_changed_during_capture")
}
func (o *Observer) Read(ctx context.Context, fresh bool) (Status, error) {
	return o.read(ctx, fresh, false)
}

// ReadFocused checks activewindow twice alongside the reconciled inventory.
// Focus history rank is deliberately not a focus postcondition.
func (o *Observer) ReadFocused(ctx context.Context) (Status, error) {
	return o.read(ctx, true, true)
}
func (o *Observer) read(ctx context.Context, fresh, focus bool) (Status, error) {
	var err error
	if fresh {
		err = o.capture(ctx, focus)
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	v := o.status
	if v.FocusedWindow != nil {
		copy := *v.FocusedWindow
		v.FocusedWindow = &copy
	}
	if v.Snapshot == nil || time.Since(v.Snapshot.CapturedAt) > 10*time.Second {
		v.Fresh = false
		if v.Issue == "" {
			v.Issue = "inventory_expired"
		}
	}
	// No caller may mutate the cache or race a later publication through a slice.
	if v.Snapshot != nil {
		b, _ := json.Marshal(v.Snapshot)
		var copy model.DesktopSnapshot
		_ = json.Unmarshal(b, &copy)
		v.Snapshot = &copy
	}
	return v, err
}

func activeWindow(ctx context.Context, c Connection) (*model.WindowIdentity, error) {
	b, err := c.Read(ctx, "activewindow")
	if err != nil {
		return nil, err
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(b, &fields) != nil || fields == nil {
		return nil, fmt.Errorf("invalid_activewindow_json")
	}
	if len(fields) == 0 {
		return nil, nil
	}
	var id string
	if json.Unmarshal(fields["stableId"], &id) != nil {
		return nil, fmt.Errorf("active_window_identity_unavailable")
	}
	w := model.WindowIdentity{SourceEpoch: c.Source().Epoch, StableID: id}
	if w.Validate() != nil {
		return nil, fmt.Errorf("invalid_active_window_identity")
	}
	return &w, nil
}

// CheckCapture also rejects an intervening capture, including one with unchanged
// inventory but different focus. No socket reads occur under a store writer.
func (o *Observer) CheckCapture(id string, at time.Time) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.status.Fresh || o.broken || o.status.Snapshot == nil || o.status.Snapshot.ID != id || !o.status.Snapshot.CapturedAt.Equal(at) || time.Since(at) > 5*time.Second {
		return fmt.Errorf("native capture changed or expired")
	}
	return nil
}
func (o *Observer) Check(id string) error {
	o.mu.Lock()
	defer o.mu.Unlock()
	if !o.status.Fresh || o.broken || o.status.Snapshot == nil || o.status.Snapshot.ID != id || time.Since(o.status.Snapshot.CapturedAt) > 10*time.Second {
		return fmt.Errorf("live observation changed or expired")
	}
	return nil
}
func Probe(ctx context.Context, dir string, connector Connector) (Status, error) {
	o := New()
	if connector != nil {
		o.Connector = connector
	}
	defer o.Close()
	o.Configure(model.DesktopSource{ID: model.NewID(), Active: true, SocketDir: dir})
	return o.Read(ctx, true)
}
