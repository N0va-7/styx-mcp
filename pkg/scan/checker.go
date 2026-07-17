package scan

import (
	"context"
	"strings"
	"time"
)

// Method names for scan/discover probes.
const (
	MethodAuto    = "auto"
	MethodConnect = "connect"
	MethodSYN     = "syn"
)

// PortChecker tests whether a TCP port is open without full fingerprint I/O.
type PortChecker interface {
	// Open returns true if the port appears open.
	Open(ctx context.Context, ip string, port uint16) (bool, error)
	// Method returns "syn" or "connect".
	Method() string
	// Close releases resources (raw sockets, etc.).
	Close() error
}

// ResolveMethod normalizes method and picks an implementation.
// synRequired: error if SYN requested but unavailable.
func ResolveMethod(method string) (string, error) {
	m := strings.ToLower(strings.TrimSpace(method))
	if m == "" {
		m = MethodAuto
	}
	switch m {
	case MethodAuto, MethodConnect, MethodSYN:
		return m, nil
	default:
		return "", errf("unknown scan method %q (want auto|connect|syn)", m)
	}
}

// NewPortChecker builds a checker for the resolved method.
// auto → SYN if CanRawTCP, else connect. syn → error if !CanRawTCP.
func NewPortChecker(method string, timeout time.Duration) (PortChecker, error) {
	m, err := ResolveMethod(method)
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = time.Duration(DefaultTimeoutMs) * time.Millisecond
	}
	switch m {
	case MethodConnect:
		return newConnectChecker(timeout), nil
	case MethodSYN:
		if !CanRawTCP() {
			return nil, errf("method syn requires raw IPv4 TCP (root/CAP_NET_RAW on Linux)")
		}
		c, err := newSynChecker(timeout)
		if err != nil {
			return nil, errf("syn engine: %w", err)
		}
		return c, nil
	default: // auto
		if CanRawTCP() {
			c, err := newSynChecker(timeout)
			if err == nil {
				return c, nil
			}
			// fall through to connect
		}
		return newConnectChecker(timeout), nil
	}
}

type connectChecker struct {
	timeout time.Duration
	dialer  NetDialer
}

func newConnectChecker(timeout time.Duration) *connectChecker {
	return &connectChecker{
		timeout: timeout,
		dialer:  NetDialer{Timeout: timeout},
	}
}

func (c *connectChecker) Method() string { return MethodConnect }

func (c *connectChecker) Close() error { return nil }

func (c *connectChecker) Open(ctx context.Context, ip string, port uint16) (bool, error) {
	addr := joinHostPort(ip, port)
	dctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	conn, err := c.dialer.DialContext(dctx, "tcp", addr)
	if err != nil {
		return false, nil
	}
	_ = conn.Close()
	return true, nil
}
