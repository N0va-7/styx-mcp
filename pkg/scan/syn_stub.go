//go:build !linux

package scan

import (
	"time"
)

func newSynChecker(timeout time.Duration) (PortChecker, error) {
	return nil, errf("SYN scan is only supported on Linux")
}
