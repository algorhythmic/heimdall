//go:build linux || darwin

package main

import (
	"os/exec"
	"syscall"
)

func detachDaemon(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
