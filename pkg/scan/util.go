package scan

import (
	"fmt"
	"net"
)

func errf(format string, args ...interface{}) error {
	return fmt.Errorf(format, args...)
}

func joinHostPort(ip string, port uint16) string {
	return net.JoinHostPort(ip, fmt.Sprintf("%d", port))
}
