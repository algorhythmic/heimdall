package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const serviceUnit = `[Unit]
Description=Heimdall continuity daemon
After=graphical-session.target
PartOf=graphical-session.target

[Service]
Type=exec
ExecStart=%h/.local/bin/heimdall start --data-dir %h/.local/share/heimdall
Restart=on-failure
RestartSec=3
ExecStopPost=/bin/sh -c 'rm -f %h/.local/share/heimdall/endpoint.json %h/.local/share/heimdall/client-endpoint.json %h/.local/share/heimdall/browser-endpoint.json'

[Install]
WantedBy=default.target
`

// serviceCLI manages the optional user-level systemd unit. The daemon's
// writer.lock already suppresses duplicate starts; the unit exists so a login
// session restores the daemon without a manual `heimdall` invocation.
func serviceCLI(ctx context.Context, o options, args []string, out io.Writer) error {
	if len(args) != 1 {
		return fmt.Errorf("service install|uninstall|status")
	}
	unitDir := filepath.Join(os.Getenv("HOME"), ".config", "systemd", "user")
	unitPath := filepath.Join(unitDir, "heimdall.service")
	systemctl := func(args ...string) error {
		cmd := exec.CommandContext(ctx, "systemctl", append([]string{"--user"}, args...)...)
		cmd.Stdout, cmd.Stderr = out, os.Stderr
		return cmd.Run()
	}
	switch args[0] {
	case "install":
		if _, err := exec.LookPath("systemctl"); err != nil {
			return fmt.Errorf("systemctl unavailable; `heimdall` auto-starts the daemon without systemd")
		}
		if err := os.MkdirAll(unitDir, 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(unitPath, []byte(serviceUnit), 0o644); err != nil {
			return err
		}
		if err := systemctl("daemon-reload"); err != nil {
			return err
		}
		if err := systemctl("enable", "--now", "heimdall.service"); err != nil {
			return err
		}
		fmt.Fprintln(out, "Installed and started user unit heimdall.service.")
	case "uninstall":
		_ = systemctl("disable", "--now", "heimdall.service")
		if err := os.Remove(unitPath); err != nil && !os.IsNotExist(err) {
			return err
		}
		_ = systemctl("daemon-reload")
		fmt.Fprintln(out, "Removed user unit heimdall.service.")
	case "status":
		return systemctl("status", "heimdall.service", "--no-pager")
	default:
		return fmt.Errorf("service install|uninstall|status")
	}
	_ = strings.TrimSpace
	return nil
}
