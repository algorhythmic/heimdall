//go:build windows

package main

import (
	"os/exec"
	"syscall"
)

// DETACHED_PROCESS keeps the daemon running after the launching console exits.
func detachDaemon(c *exec.Cmd) {
	c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP | 0x00000008}
}
