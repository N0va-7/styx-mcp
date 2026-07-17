package scan

import (
	"testing"
)

func TestFastPortsFrozen(t *testing.T) {
	if len(FastPorts) < 20 || len(FastPorts) > 50 {
		t.Fatalf("fast set size %d outside 20–50", len(FastPorts))
	}
	// Spot-check high-value ports.
	need := map[uint16]bool{22: false, 80: false, 443: false, 445: false, 3389: false, 6379: false, 7001: false, 8080: false}
	for _, p := range FastPorts {
		if _, ok := need[p]; ok {
			need[p] = true
		}
	}
	for p, ok := range need {
		if !ok {
			t.Errorf("fast set missing port %d", p)
		}
	}
}

func TestNormalPortsLargerThanFast(t *testing.T) {
	if len(NormalPorts) < 80 || len(NormalPorts) > 160 {
		t.Fatalf("normal set size %d outside 80–160", len(NormalPorts))
	}
	if len(NormalPorts) <= len(FastPorts) {
		t.Fatalf("normal (%d) should exceed fast (%d)", len(NormalPorts), len(FastPorts))
	}
}

func TestResolvePortsModes(t *testing.T) {
	fast, err := ResolvePorts(ModeFast, "")
	if err != nil || len(fast) != len(FastPorts) {
		t.Fatalf("fast: err=%v len=%d", err, len(fast))
	}
	normal, err := ResolvePorts(ModeNormal, "")
	if err != nil || len(normal) != len(NormalPorts) {
		t.Fatalf("normal: err=%v len=%d", err, len(normal))
	}
	full, err := ResolvePorts(ModeFull, "")
	if err != nil || len(full) != 65535 {
		t.Fatalf("full: err=%v len=%d", err, len(full))
	}
	if full[0] != 1 || full[65534] != 65535 {
		t.Fatalf("full range bounds: first=%d last=%d", full[0], full[65534])
	}
	custom, err := ResolvePorts(ModeCustom, "22,80,8000-8002")
	if err != nil {
		t.Fatal(err)
	}
	if len(custom) != 5 {
		t.Fatalf("custom len=%d want 5", len(custom))
	}
	_, err = ResolvePorts(ModeCustom, "")
	if err == nil {
		t.Fatal("custom empty should fail")
	}
	_, err = ResolvePorts("bogus", "")
	if err == nil {
		t.Fatal("unknown mode should fail")
	}
}

func TestParsePorts(t *testing.T) {
	ports, err := ParsePorts("22, 80, 443, 8000-8002, 22")
	if err != nil {
		t.Fatal(err)
	}
	want := []uint16{22, 80, 443, 8000, 8001, 8002}
	if len(ports) != len(want) {
		t.Fatalf("got %v", ports)
	}
	for i := range want {
		if ports[i] != want[i] {
			t.Fatalf("got %v want %v", ports, want)
		}
	}
	if _, err := ParsePorts(""); err == nil {
		t.Fatal("empty should fail")
	}
	if _, err := ParsePorts("0"); err == nil {
		t.Fatal("port 0 should fail")
	}
	if _, err := ParsePorts("abc"); err == nil {
		t.Fatal("non-numeric should fail")
	}
}
