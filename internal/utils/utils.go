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
