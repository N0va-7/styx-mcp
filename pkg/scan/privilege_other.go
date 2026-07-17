//go:build !linux

package scan

// CanRawTCP is false on non-Linux (no SYN engine in v1).
func CanRawTCP() bool { return false }
