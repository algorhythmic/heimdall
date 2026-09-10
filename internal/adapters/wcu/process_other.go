//go:build !linux

package wcu

import (
	"fmt"
	"os"
	"os/exec"
)

func configureProcess(*exec.Cmd) error { return fmt.Errorf("WCU observer requires Linux") }
func killProcess(*exec.Cmd)            {}

func openReport(root *os.Root, name string) (*os.File, error) { return root.Open(name) }
