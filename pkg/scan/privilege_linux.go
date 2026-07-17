//go:build linux

package scan

import (
	"net"
	"os"
)

// CanRawTCP reports whether SYN-style probes are likely available.
func CanRawTCP() bool {
	if os.Geteuid() == 0 {
		return true
	}
	// CAP_NET_RAW may allow this without euid 0.
	c, err := net.ListenPacket("ip4:tcp", "0.0.0.0")
	if err != nil {
		return false
	}
	_ = c.Close()
	return true
}
