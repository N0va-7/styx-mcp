package controller

import (
	"net"
	"strings"
	"testing"
)

func TestStartFreePortBinds(t *testing.T) {
	c := NewController(&Options{
		Secret:     "unit-start-bind",
		Listen:     "127.0.0.1:0",
		Downstream: "raw",
	})
	if err := c.Start(); err != nil {
		t.Fatalf("Start free port: %v", err)
	}
	// Topology loop is running; empty list is fine without agents.
	if n := len(c.ListNodes()); n != 0 {
		t.Fatalf("want empty topology after Start, got %d", n)
	}
}

func TestStartPortAlreadyInUse(t *testing.T) {
	holder, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	addr := holder.Addr().String()

	c := NewController(&Options{
		Secret:     "unit-start-bind",
		Listen:     addr,
		Downstream: "raw",
	})
	err = c.Start()
	if err == nil {
		t.Fatal("expected Start to fail when listen address is in use")
	}
	msg := err.Error()
	if !strings.Contains(msg, addr) {
		t.Fatalf("error should mention address %s: %s", addr, msg)
	}
	// Prefer the actionable port-conflict path (EADDRINUSE).
	lower := strings.ToLower(msg)
	if !strings.Contains(lower, "already in use") && !strings.Contains(lower, "address already") {
		// Still hard-fail even if OS message differs; require non-nil was enough,
		// but flag soft wording so we notice regression to generic errors.
		if !strings.Contains(msg, "listen on") {
			t.Fatalf("unexpected error shape: %s", msg)
		}
	}
	if strings.Contains(lower, "already in use") && !strings.Contains(msg, "STYX_LISTEN") {
		t.Fatalf("addr-in-use path should hint STYX_LISTEN: %s", msg)
	}
}

func TestStartInvalidListenAddress(t *testing.T) {
	c := NewController(&Options{
		Secret: "unit-start-bind",
		Listen: "not-an-address",
	})
	err := c.Start()
	if err == nil {
		t.Fatal("expected invalid listen address to fail Start")
	}
}
