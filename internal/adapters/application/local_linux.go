//go:build linux

package application

import (
	"fmt"
	"heimdall/internal/adapters/hyprland"
	"heimdall/internal/model"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

func supported() error { return nil }
func process(pid int) (model.ApplicationProcess, error) {
	r := model.ApplicationProcess{PID: pid}
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return r, err
	}
	i := strings.LastIndexByte(string(b), ')')
	if i < 0 {
		return r, fmt.Errorf("invalid process identity")
	}
	f := strings.Fields(string(b[i+1:]))
	if len(f) < 20 || f[0] == "Z" {
		return r, fmt.Errorf("process unavailable")
	}
	r.Start = f[19]
	if !r.Valid() {
		return r, fmt.Errorf("invalid process start identity")
	}
	return r, nil
}

func launch(s model.ApplicationSpec, attempt string, session *model.SessionBinding) hyprland.DispatchReceipt {
	// A dedicated foot process supplies one Wayland window and an exact PID.
	// -- ends emulator options before the reviewed executable and literal argv.
	argv := []string{"--app-id=" + model.ApplicationClass(attempt), "--working-directory=" + s.Cwd, "--", s.Command}
	argv = append(argv, s.Argv...)
	if session != nil {
		argv = append(argv, "terminal", "attach", session.Herdr.TerminalID)
	}
	if s.Editor != nil {
		e := s.Editor
		argv = append(argv, "-u", "NONE", "-i", "NONE", "--noplugin", "-c", fmt.Sprintf("buffer %d", e.Active+1), "-c", fmt.Sprintf("call cursor(%d,%d)", e.Line, e.Column), "--")
		argv = append(argv, e.Files...)
	}
	cmd := exec.Command(s.Executable, argv...)
	cmd.Dir = s.Cwd
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	// Deliberately exclude inherited caller pane/session identity and launch hooks.
	for _, key := range []string{"HOME", "USER", "LOGNAME", "PATH", "LANG", "LC_ALL", "XDG_RUNTIME_DIR", "WAYLAND_DISPLAY", "DBUS_SESSION_BUS_ADDRESS", "XDG_CONFIG_HOME", "XDG_DATA_HOME"} {
		if value, ok := os.LookupEnv(key); ok {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	if session != nil {
		cmd.Env = append(cmd.Env, "HERDR_SOCKET_PATH="+session.Locator.SessionID)
	}
	if err := cmd.Start(); err != nil {
		return hyprland.DispatchReceipt{Detail: "Reviewed application could not start"}
	}
	r := hyprland.DispatchReceipt{Submitted: true, Detail: "Process started; window association pending"}
	p, err := process(cmd.Process.Pid)
	if err == nil {
		r.Process, r.Acknowledged = &p, true
	}
	// The child lifetime belongs to the user, not the request context. Reap it
	// without killing it on daemon/request cancellation.
	go func() { _ = cmd.Wait() }()
	return r
}
