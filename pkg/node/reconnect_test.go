package node

import (
	"testing"
	"time"
)

func TestReconnectDelayBounds(t *testing.T) {
	base := 10
	// attempt 1: base * 2^0 = 10, jitter [0,10] → [10,20]s
	// attempt 2: 20 + [0,10] → [20,30]s
	// attempt 3: 40 + [0,10] → [40,50]s
	for attempt := 1; attempt <= 3; attempt++ {
		minExp := base
		for i := 1; i < attempt; i++ {
			minExp *= 2
		}
		lo := time.Duration(minExp) * time.Second
		hi := time.Duration(minExp+base) * time.Second
		for i := 0; i < 50; i++ {
			d := reconnectDelay(base, attempt)
			if d < lo || d > hi {
				t.Fatalf("attempt=%d delay=%v outside [%v,%v]", attempt, d, lo, hi)
			}
		}
	}
}

func TestReconnectDelayDisabled(t *testing.T) {
	if d := reconnectDelay(0, 1); d != 0 {
		t.Fatalf("base 0: got %v", d)
	}
	if d := reconnectDelay(10, 0); d != 0 {
		t.Fatalf("attempt 0: got %v", d)
	}
}

func TestReconnectDelayClamp(t *testing.T) {
	// Large attempt should clamp exponential part at maxBackoffSec.
	d := reconnectDelay(10, 20)
	// max exp 300 + jitter [0,10] → [300,310]s
	if d < 300*time.Second || d > 310*time.Second {
		t.Fatalf("clamped delay=%v want [300s,310s]", d)
	}
}

func TestShouldAttemptReconnect(t *testing.T) {
	cases := []struct {
		name            string
		doNotReconnect  bool
		base, max       int
		active          bool
		want            bool
	}{
		{"default enabled", false, 10, 3, true, true},
		{"shutdown", true, 10, 3, true, false},
		{"base zero", false, 0, 3, true, false},
		{"max zero", false, 10, 0, true, false},
		{"passive", false, 10, 3, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldAttemptReconnect(tc.doNotReconnect, tc.base, tc.max, tc.active)
			if got != tc.want {
				t.Fatalf("got %v want %v", got, tc.want)
			}
		})
	}
}
