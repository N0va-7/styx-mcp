package scan

import (
	"testing"
)

func TestParseTargetsIP(t *testing.T) {
	hosts, err := ParseTargets("192.168.1.10", 0)
	if err != nil || len(hosts) != 1 || hosts[0] != "192.168.1.10" {
		t.Fatalf("got %v err=%v", hosts, err)
	}
}

func TestParseTargetsList(t *testing.T) {
	hosts, err := ParseTargets("10.0.0.1,10.0.0.2,10.0.0.1", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 2 {
		t.Fatalf("want 2 unique, got %v", hosts)
	}
}

func TestParseTargetsCIDR(t *testing.T) {
	// /30 → 4 addresses; skip network+broadcast → 2 hosts.
	hosts, err := ParseTargets("192.168.1.0/30", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(hosts) != 2 {
		t.Fatalf("want 2 hosts for /30, got %v", hosts)
	}
}

func TestParseTargetsMaxHosts(t *testing.T) {
	_, err := ParseTargets("10.0.0.0/24", 10)
	if err == nil {
		t.Fatal("expected max hosts error")
	}
}

func TestParseTargetsEmptyInvalid(t *testing.T) {
	if _, err := ParseTargets("", 0); err == nil {
		t.Fatal("empty")
	}
	if _, err := ParseTargets("not-an-ip", 0); err == nil {
		t.Fatal("invalid")
	}
	if _, err := ParseTargets("::1", 0); err == nil {
		t.Fatal("ipv6 should fail")
	}
}
