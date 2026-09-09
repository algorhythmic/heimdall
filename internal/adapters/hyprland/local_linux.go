//go:build linux

package hyprland

import (
	"context"

	"fmt"
	"golang.org/x/sys/unix"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func socketIdentity(stat unix.Stat_t) string {
	// A same-process listener replacement can immediately reuse the pathname's
	// inode. Include inode change time at its full precision to distinguish that
	// generation, both across RPCs and across the connect/stat race below.
	return fmt.Sprintf("%d/%d/%d/%d", stat.Dev, stat.Ino, stat.Ctim.Sec, stat.Ctim.Nsec)
}

func processStart(pid int) (string, error) {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "", fmt.Errorf("process identity unavailable")
	}
	i := strings.LastIndexByte(string(b), ')')
	if i < 0 {
		return "", fmt.Errorf("invalid process identity")
	}
	fields := strings.Fields(string(b[i+1:]))
	if len(fields) < 20 {
		return "", fmt.Errorf("short process identity")
	}
	if n, err := strconv.ParseUint(fields[19], 10, 64); err != nil || n == 0 {
		return "", fmt.Errorf("invalid process start time")
	}
	return fields[19], nil
}

func dial(ctx context.Context, path string) (net.Conn, string, string, string, error) {
	bad := func(err error) (net.Conn, string, string, string, error) { return nil, "", "", "", err }
	if !filepath.IsAbs(path) || len(path) > 4096 {
		return bad(fmt.Errorf("explicit absolute socket path required"))
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil || canonical != path {
		return bad(fmt.Errorf("selected Hyprland socket unavailable"))
	}
	var before unix.Stat_t
	if err := unix.Stat(path, &before); err != nil || before.Mode&unix.S_IFMT != unix.S_IFSOCK || before.Uid != uint32(os.Geteuid()) || before.Mode&0022 != 0 {
		return bad(fmt.Errorf("Hyprland socket must be user-owned and not writable by group or others"))
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
	if err != nil {
		return bad(fmt.Errorf("selected Hyprland server unavailable"))
	}
	failed := func(err error) (net.Conn, string, string, string, error) { conn.Close(); return bad(err) }
	raw, err := conn.(*net.UnixConn).SyscallConn()
	if err != nil {
		return failed(err)
	}
	var cred *unix.Ucred
	var sockErr error
	err = raw.Control(func(fd uintptr) { cred, sockErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED) })
	if err != nil || sockErr != nil || cred == nil || cred.Uid != uint32(os.Geteuid()) {
		return failed(fmt.Errorf("Hyprland peer must be the same local user"))
	}
	start, err := processStart(int(cred.Pid))
	if err != nil {
		return failed(err)
	}
	boot, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err != nil {
		return failed(err)
	}
	machine, err := os.ReadFile("/etc/machine-id")
	if err != nil || strings.TrimSpace(string(machine)) == "" {
		return failed(fmt.Errorf("local machine identity unavailable"))
	}
	var after unix.Stat_t
	if err := unix.Stat(path, &after); err != nil || socketIdentity(before) != socketIdentity(after) {
		return failed(fmt.Errorf("Hyprland socket replaced during connection"))
	}
	host := hash(strings.TrimSpace(string(machine)))
	peer := hash(fmt.Sprintf("hyprland-peer-v1/%s/%s/%d/%s", host, strings.TrimSpace(string(boot)), cred.Pid, start))
	epoch := hash(peer + "/" + socketIdentity(after))
	return conn, epoch, peer, host, nil
}

func validDir(path string) error {
	if !filepath.IsAbs(path) || filepath.Clean(path) != path || len(path) > 4096 {
		return fmt.Errorf("explicit canonical absolute socket directory required")
	}
	canonical, err := filepath.EvalSymlinks(path)
	if err != nil || canonical != path {
		return fmt.Errorf("socket directory unavailable or not canonical")
	}
	var st unix.Stat_t
	if err := unix.Lstat(path, &st); err != nil || st.Mode&unix.S_IFMT != unix.S_IFDIR || st.Uid != uint32(os.Geteuid()) || st.Mode&0022 != 0 {
		return fmt.Errorf("socket directory must be user-owned and not writable by group or others")
	}
	return nil
}
