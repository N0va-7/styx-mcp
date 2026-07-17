package node

import (
	"math/rand"
	"time"
)

// reconnectDelay returns the sleep before a post-drop reconnect attempt.
// attempt is 1-based. Formula (design D3):
//
//	sleep ≈ base * 2^(attempt-1) clamped to maxBackoffSec, plus uniform jitter in [0, base] seconds.
//
// When baseSec <= 0 the result is 0 (caller should not reconnect).
func reconnectDelay(baseSec, attempt int) time.Duration {
	if baseSec <= 0 || attempt < 1 {
		return 0
	}
	exp := baseSec
	for i := 1; i < attempt; i++ {
		if exp > maxBackoffSec/2 {
			exp = maxBackoffSec
			break
		}
		exp *= 2
	}
	if exp > maxBackoffSec {
		exp = maxBackoffSec
	}
	jitter := rand.Intn(baseSec + 1) // [0, base]
	return time.Duration(exp+jitter) * time.Second
}

// maxBackoffSec clamps exponential growth (5 minutes).
const maxBackoffSec = 300

// shouldAttemptReconnect reports whether the agent may enter the reconnect loop
// after an upstream drop (transport Scenario: shutdown / disabled / passive).
func shouldAttemptReconnect(doNotReconnect bool, reconnectBase, reconnectMax int, activeMode bool) bool {
	if doNotReconnect {
		return false
	}
	if !activeMode {
		return false
	}
	if reconnectBase <= 0 || reconnectMax <= 0 {
		return false
	}
	return true
}
