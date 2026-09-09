//go:build linux

package herdr

import (
	"context"
	"crypto/sha256"
	"fmt"
	"golang.org/x/sys/unix"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func digest(s string) string { return fmt.Sprintf("%x", sha256.Sum256([]byte(s))) }

func socketIdentity(stat unix.Stat_t) string {
	// A same-process listener replacement can immediately reuse the pathname's
	// inode. Include inode change time at its full precision to distinguish that
	// generation, both across RPCs and across the connect/stat race below.
	return fmt.Sprintf("%d/%d/%d/%d", stat.Dev, stat.Ino, stat.Ctim.Sec, stat.Ctim.Nsec)
}

func processStart(pid int) (string, error) {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return "", fail("process_unavailable", "process identity unavailable")
	}
	i := strings.LastIndexByte(string(b), ')')
	if i < 0 {
		return "", fail("process_unavailable", "invalid process identity")
	}
	fields := strings.Fields(string(b[i+1:]))
	if len(fields) < 20 {
		return "", fail("process_unavailable", "short process identity")
	}
	if n, err := strconv.ParseUint(fields[19], 10, 64); err != nil || n == 0 {
		return "", fail("process_unavailable", "invalid process start time")
	}
	return fields[19], nil
}

func processInfo(pid int) (string, string, error) {
	start, err := processStart(pid)
	if err != nil {
		return "", "", err
	}
	cwd, err := canonicalDir(fmt.Sprintf("/proc/%d/cwd", pid))
	return start, cwd, err
}

func dial(ctx context.Context, path string) (net.Conn, string, string, string, error) {
	bad := func(err error) (net.Conn, string, string, string, error) { return nil, "", "", "", err }
	if !filepath.IsAbs(path) || len(path) > 4096 {
		return bad(fail("invalid_socket", "explicit absolute socket path required"))
	}
	path, err := filepath.EvalSymlinks(path)
	if err != nil {
		return bad(fail("disconnected", "selected Herdr socket unavailable"))
	}
	var before unix.Stat_t
	if err := unix.Stat(path, &before); err != nil || before.Mode&unix.S_IFMT != unix.S_IFSOCK || before.Uid != uint32(os.Geteuid()) || before.Mode&0022 != 0 {
		return bad(fail("invalid_socket", "Herdr socket must be user-owned and not writable by group or others"))
	}
	conn, err := (&net.Dialer{}).DialContext(ctx, "unix", path)
	if err != nil {
		return bad(fail("disconnected", "selected Herdr server unavailable"))
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
		return failed(fail("invalid_socket", "Herdr peer must be the same local user"))
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
		return failed(fail("host_unavailable", "local machine identity unavailable"))
	}
	var after unix.Stat_t
	if err := unix.Stat(path, &after); err != nil || socketIdentity(before) != socketIdentity(after) {
		return failed(fail("source_changed", "Herdr socket replaced during connection"))
	}
	host := digest(strings.TrimSpace(string(machine)))
	epoch := digest(fmt.Sprintf("herdr-source-v2/%s/%s/%d/%s/%s", host, strings.TrimSpace(string(boot)), cred.Pid, start, socketIdentity(after)))
	return conn, path, epoch, host, nil
}
