//go:build !linux

package hyprland

import (
	"context"
	"fmt"
	"net"
)

func dial(context.Context, string) (net.Conn, string, string, string, error) {
	return nil, "", "", "", fmt.Errorf("Hyprland observation requires Linux")
}
func validDir(string) error { return fmt.Errorf("Hyprland observation requires Linux") }
