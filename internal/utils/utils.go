package utils

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net"
	"os"
	"os/user"
	"runtime"
	"strings"
)

// GenerateUUID returns a 10-character hex string for node IDs (wire field width).
func GenerateUUID() string {
	b := make([]byte, 5)
	if _, err := rand.Read(b); err != nil {
		return ""
	}
	return hex.EncodeToString(b)
}

// CheckIPPort parses an address string like "127.0.0.1:8080" or "8080".
// If only a port is given, it defaults to "0.0.0.0:port".
func CheckIPPort(addr string) (string, string, error) {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return "", "", fmt.Errorf("empty address")
	}

	if !strings.Contains(addr, ":") {
		return fmt.Sprintf("0.0.0.0:%s", addr), addr, nil
	}

	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return "", "", err
	}

	if host == "" {
		host = "0.0.0.0"
	}

	return net.JoinHostPort(host, port), port, nil
}

// GetSystemInfo returns hostname and username of the current system.
func GetSystemInfo() (hostname, username string) {
	hostname, _ = os.Hostname()
	if hostname == "" {
		hostname = "unknown"
	}

	u, err := user.Current()
	if err != nil {
		username = "unknown"
	} else {
		username = u.Username
	}

	if runtime.GOOS == "windows" && strings.Contains(username, "\\") {
		parts := strings.Split(username, "\\")
		username = parts[len(parts)-1]
	}

	return hostname, username
}

// StringSliceReverse reverses a string slice in place.
func StringSliceReverse(s []string) {
	for i, j := 0, len(s)-1; i < j; i, j = i+1, j-1 {
		s[i], s[j] = s[j], s[i]
	}
}

// LocalIPv4Addrs returns non-loopback IPv4 addresses on up interfaces.
// Order is stable (interface then addr order from the OS). Errors yield nil.
func LocalIPv4Addrs() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []string
	seen := make(map[string]struct{})
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil {
				continue
			}
			ip4 := ip.To4()
			if ip4 == nil || ip4.IsLoopback() || ip4.IsLinkLocalUnicast() {
				continue
			}
			s := ip4.String()
			if _, ok := seen[s]; ok {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out
}

// JoinAddrs joins addresses with commas for wire encoding.
func JoinAddrs(addrs []string) string {
	return strings.Join(addrs, ",")
}

// SplitAddrs splits a comma-separated address list, dropping empties.
func SplitAddrs(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
