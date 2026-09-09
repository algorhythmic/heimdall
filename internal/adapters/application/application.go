// Package application executes explicitly reviewed recipes. It does not infer
// commands from terminal history, window titles, or captured process arguments.
package application

import (
	"context"
	"crypto/sha256"
	"fmt"
	"heimdall/internal/adapters/herdr"
	"heimdall/internal/adapters/hyprland"
	"heimdall/internal/model"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"sync"
	"time"
)

func Digest(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0111 == 0 || info.Size() > 256<<20 {
		return "", fmt.Errorf("bounded executable file required")
	}
	h := sha256.New()
	if _, err := io.Copy(h, io.LimitReader(f, (256<<20)+1)); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

func CheckFiles(s model.ApplicationSpec) error {
	if err := s.Validate(); err != nil {
		return err
	}
	if s.Adapter == "browser" {
		return nil
	}
	for _, pin := range [][2]string{{s.Executable, s.ExecutableDigest}, {s.Command, s.CommandDigest}} {
		d, err := Digest(pin[0])
		if err != nil || d != pin[1] {
			return fmt.Errorf("reviewed executable missing or changed: %s", pin[0])
		}
	}
	p, err := filepath.EvalSymlinks(s.Cwd)
	if err != nil || p != filepath.Clean(s.Cwd) {
		return fmt.Errorf("reviewed cwd must exist and be canonical")
	}
	info, err := os.Stat(p)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("reviewed cwd unavailable")
	}
	if s.Editor != nil {
		for _, file := range s.Editor.Files {
			info, err := os.Stat(file)
			if err != nil || !info.Mode().IsRegular() {
				return fmt.Errorf("saved editor file unavailable: %s", file)
			}
		}
	}
	return nil
}

type Adapter struct{ Observer *hyprland.Observer }
type prepared struct {
	adapter  Adapter
	spec     model.ApplicationSpec
	session  *model.SessionBinding
	attempt  string
	observed hyprland.Status
	started  time.Time
	mu       sync.Mutex
	used     bool
}

func (a Adapter) Prepare(ctx context.Context, source model.DesktopSource, spec model.ApplicationSpec, attempt string, session *model.SessionBinding) (hyprland.PreparedCommand, error) {
	if a.Observer == nil || !model.OpaqueID.MatchString(attempt) {
		return nil, fmt.Errorf("selected observer and attempt required")
	}
	if err := supported(); err != nil {
		return nil, err
	}
	if err := CheckFiles(spec); err != nil {
		return nil, err
	}
	if err := CheckSession(ctx, spec, session); err != nil {
		return nil, err
	}
	o, err := a.Observer.Read(ctx, true)
	if err != nil || !o.Fresh || o.Snapshot == nil || o.Snapshot.SourceEpoch != source.Epoch {
		return nil, fmt.Errorf("fresh selected compositor required")
	}
	for _, w := range o.Snapshot.Windows {
		if w.Class == model.ApplicationClass(attempt) {
			return nil, fmt.Errorf("attempt already has a candidate window")
		}
	}
	return &prepared{adapter: a, spec: spec, session: session, attempt: attempt, observed: o, started: time.Now()}, nil
}
func (p *prepared) Observation() hyprland.Status { return model.Clone(p.observed) }
func (p *prepared) Close() error                 { return nil }
func (p *prepared) Check() error {
	if time.Since(p.started) < 0 || time.Since(p.started) >= 2*time.Second {
		return fmt.Errorf("application preparation expired")
	}
	return p.adapter.Observer.CheckCapture(p.observed.Snapshot.ID, p.observed.Snapshot.CapturedAt)
}
func (p *prepared) Send(ctx context.Context, commit func() error) hyprland.DispatchReceipt {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.used {
		return hyprland.DispatchReceipt{Detail: "application preparation already consumed"}
	}
	p.used = true
	for _, check := range []func() error{ctx.Err, p.Check, func() error { return CheckFiles(p.spec) }, func() error { return CheckSession(ctx, p.spec, p.session) }} {
		if err := check(); err != nil {
			return hyprland.DispatchReceipt{Detail: err.Error()}
		}
	}
	if commit == nil {
		return hyprland.DispatchReceipt{Detail: "durable dispatch required"}
	}
	if err := commit(); err != nil {
		return hyprland.DispatchReceipt{Detail: err.Error()}
	}
	if err := ctx.Err(); err != nil {
		return hyprland.DispatchReceipt{Detail: err.Error()}
	}
	if err := p.Check(); err != nil {
		return hyprland.DispatchReceipt{Detail: err.Error()}
	}
	return launch(p.spec, p.attempt, p.session)
}

func CheckSession(ctx context.Context, s model.ApplicationSpec, b *model.SessionBinding) error {
	if s.SessionBindingID == "" {
		if b != nil {
			return fmt.Errorf("unexpected session")
		}
		return nil
	}
	if b == nil || b.ID != s.SessionBindingID || !b.Active || b.Locator == nil || b.Herdr == nil || b.Locator.Cwd != s.Cwd {
		return fmt.Errorf("typed current Herdr binding required")
	}
	o, err := (herdr.Adapter{}).Observe(ctx, b.Locator.SessionID, b.Locator.PaneID, b.Locator.SourceEpoch)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(o.Locator, *b.Locator) || !reflect.DeepEqual(o.Herdr, *b.Herdr) {
		return fmt.Errorf("Herdr pane, process or workspace changed")
	}
	return nil
}

func (a Adapter) Process(pid int) (model.ApplicationProcess, error) { return process(pid) }
