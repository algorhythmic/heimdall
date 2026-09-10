package main

import (
	"context"
	"fmt"
	"heimdall/internal/core"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"golang.org/x/term"
)

// interactive reports whether this invocation owns a real terminal on stdout.
// Piped output and test buffers keep the historical non-launching behavior.
func interactive(out io.Writer) bool {
	f, ok := out.(*os.File)
	return ok && f.Fd() == os.Stdout.Fd() && term.IsTerminal(int(f.Fd()))
}

// ensureDaemon reuses a healthy endpoint or launches a detached `heimdall
// start` child that owns the data directory and outlives the terminal that
// launched it. Interactive reads therefore never require a separate service
// manager; an explicit `heimdall start` remains available for supervision.
func ensureDaemon(ctx context.Context, o options) error {
	if _, err := call(ctx, o, "GET", "/health", nil); err == nil {
		return nil
	}
	if _, err := os.Stat(filepath.Join(o.dir, "heimdall.db")); os.IsNotExist(err) {
		e, err := core.Open(o.dir)
		if err != nil {
			return err
		}
		if err = e.Close(); err != nil {
			return err
		}
	}
	if err := os.MkdirAll(o.dir, 0700); err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	log, err := os.OpenFile(filepath.Join(o.dir, "daemon.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	defer log.Close()
	cmd := exec.Command(exe, "start", "--data-dir", o.dir)
	cmd.Stdin = nil
	cmd.Stdout = log
	cmd.Stderr = log
	detachDaemon(cmd)
	if err = cmd.Start(); err != nil {
		return err
	}
	// The daemon keeps its own lifetime; release the child handle so an early
	// exit cannot leave a zombie while this process continues.
	_ = cmd.Process.Release()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err = call(ctx, o, "GET", "/health", nil); err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("daemon did not become healthy; see %s", log.Name())
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
		}
	}
}
