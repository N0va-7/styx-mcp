package utils

import (
	"strings"
	"testing"
)

func TestJoinSplitAddrs(t *testing.T) {
	in := []string{"10.0.0.1", "192.168.1.2"}
	s := JoinAddrs(in)
	if s != "10.0.0.1,192.168.1.2" {
		t.Fatalf("join=%q", s)
	}
	out := SplitAddrs(s)
	if len(out) != 2 || out[0] != "10.0.0.1" || out[1] != "192.168.1.2" {
		t.Fatalf("split=%v", out)
	}
	if SplitAddrs("") != nil && len(SplitAddrs("")) != 0 {
		t.Fatalf("empty split")
	}
	if got := SplitAddrs(" a , ,b "); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("trim=%v", got)
	}
}

func TestLocalIPv4AddrsNoLoopback(t *testing.T) {
	addrs := LocalIPv4Addrs()
	for _, a := range addrs {
		if a == "127.0.0.1" || strings.HasPrefix(a, "127.") {
			t.Fatalf("loopback leaked: %v", addrs)
		}
	}
}
