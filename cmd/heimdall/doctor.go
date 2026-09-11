package main

import (
	"context"
	"encoding/json"
	"fmt"
	"heimdall/internal/model"
	"io"
	"net"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type doctorCheck struct {
	Name   string `json:"name"`
	Status string `json:"status"` // ok | degraded | failed | skipped
	Detail string `json:"detail,omitempty"`
}

// doctorStartup runs bounded readiness checks against the daemon's durable
// state plus the local sensors a session needs to observe. It reports
// coverage honestly: an unreachable source is degraded, never fabricated.
func doctorStartup(ctx context.Context, o options, out io.Writer) error {
	checks := []doctorCheck{}
	add := func(name, status, detail string) { checks = append(checks, doctorCheck{name, status, detail}) }

	// Daemon endpoint liveness, bounded.
	hctx, stop := context.WithTimeout(ctx, 2*time.Second)
	_, err := call(hctx, o, "GET", "/health", nil)
	stop()
	if err != nil {
		add("daemon", "failed", "endpoint unreachable; run `heimdall start` or `heimdall`")
		reportDoctor(o, out, checks)
		return fmt.Errorf("daemon unavailable")
	}
	add("daemon", "ok", "endpoint healthy")

	// Durable state coverage.
	raw, err := call(ctx, o, "GET", "/state", nil)
	if err != nil {
		add("state", "failed", err.Error())
		reportDoctor(o, out, checks)
		return fmt.Errorf("state unavailable")
	}
	var st model.State
	if err := model.StrictJSON(raw, &st); err != nil {
		add("state", "failed", err.Error())
		reportDoctor(o, out, checks)
		return fmt.Errorf("state undecodable")
	}
	add("state", "ok", fmt.Sprintf("revision %d · %d tasks", st.Revision, len(st.Tasks)))

	// Session source roots: reachable directory or explicit coverage gap.
	roots, missing := 0, 0
	for _, r := range st.SourceRoots {
		if !r.Active {
			continue
		}
		roots++
		if _, err := os.Stat(r.Root); err != nil {
			missing++
		}
	}
	switch {
	case roots == 0:
		add("session_sources", "skipped", "no configured roots (`heimdall init --hooks` or `source add`)")
	case missing > 0:
		add("session_sources", "degraded", fmt.Sprintf("%d of %d roots unreachable", missing, roots))
	default:
		add("session_sources", "ok", fmt.Sprintf("%d roots reachable", roots))
	}
	degraded := []string{}
	for name, h := range st.SensorHealth {
		if h.Status == "degraded" {
			degraded = append(degraded, name+": "+h.Reason)
		}
	}
	sort.Strings(degraded)
	if len(degraded) > 0 {
		add("sensors", "degraded", strings.Join(degraded, "; "))
	} else {
		add("sensors", "ok", fmt.Sprintf("%d healthy", len(st.SensorHealth)))
	}

	// Compositor readiness: Hyprland socket dir under XDG_RUNTIME_DIR.
	hypr := filepath.Join(os.Getenv("XDG_RUNTIME_DIR"), "hypr")
	if entries, err := os.ReadDir(hypr); err == nil && len(entries) > 0 {
		add("compositor", "ok", "hyprland socket present")
	} else {
		add("compositor", "skipped", "no hyprland instance (non-Hyprland session)")
	}

	// Herdr: reach the socket of every active binding, bounded dial.
	bound, failed := 0, 0
	for _, head := range st.SessionHeads {
		b := st.SessionBindings[head]
		if !b.Active || b.Locator == nil || b.Locator.Adapter != "herdr" || b.Locator.SessionID == "" {
			continue
		}
		bound++
		dctx, cancel := context.WithTimeout(ctx, 1500*time.Millisecond)
		conn, err := (&net.Dialer{}).DialContext(dctx, "unix", b.Locator.SessionID)
		cancel()
		if err != nil {
			failed++
		} else {
			conn.Close()
		}
	}
	switch {
	case bound == 0:
		add("herdr", "skipped", "no active herdr session bindings")
	case failed > 0:
		add("herdr", "degraded", fmt.Sprintf("%d of %d bound sockets unreachable", failed, bound))
	default:
		add("herdr", "ok", fmt.Sprintf("%d bound sockets reachable", bound))
	}

	// Browser pairing coverage.
	paired := 0
	for _, p := range st.Browsers {
		if p.Paired {
			paired++
		}
	}
	if paired == 0 {
		add("browser", "skipped", "no paired browser profiles")
	} else {
		add("browser", "ok", fmt.Sprintf("%d paired profiles", paired))
	}

	reportDoctor(o, out, checks)
	for _, c := range checks {
		if c.Status == "failed" {
			return fmt.Errorf("startup checks failed")
		}
	}
	return nil
}

func reportDoctor(o options, out io.Writer, checks []doctorCheck) {
	if o.json {
		_ = json.NewEncoder(out).Encode(map[string]any{"checks": checks})
		return
	}
	for _, c := range checks {
		detail := ""
		if c.Detail != "" {
			detail = " — " + c.Detail
		}
		fmt.Fprintf(out, "%-16s %-8s%s\n", c.Name, c.Status, detail)
	}
}
