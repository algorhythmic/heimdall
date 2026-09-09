//go:build !linux

package herdr

import (
	"context"
	"net"
)

func dial(context.Context, string) (net.Conn, string, string, string, error) {
	return nil, "", "", "", fail("unsupported_platform", "live Herdr binding currently requires Linux")
}
func processInfo(int) (string, string, error) {
	return "", "", fail("unsupported_platform", "live Herdr binding currently requires Linux")
}
