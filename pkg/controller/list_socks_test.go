package controller

import (
	"net"
	"strings"
	"testing"

	"styx-mcp/pkg/topology"
)

func TestListNodesEmpty(t *testing.T) {
	c := NewController(&Options{Secret: "t", Listen: "127.0.0.1:0"})
	go c.Topology.Run()

	entries := c.ListNodes()
	if len(entries) != 0 {
		t.Fatalf("want empty, got %d", len(entries))
	}
}

func TestStartSocksPortBusy(t *testing.T) {
	c := NewController(&Options{Secret: "t", Listen: "127.0.0.1:0"})
	go c.Topology.Run()

	holder, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Close()
	addr := holder.Addr().String()

	err = c.StartSocks("nodeaaaaaa", addr)
	if err == nil {
		t.Fatal("expected bind error when address already in use")
	}
	// listen errors vary by OS; must not succeed and must not hang
	if strings.Contains(strings.ToLower(err.Error()), "timeout") {
		t.Fatalf("unexpected timeout path for busy port: %v", err)
	}

	c.socksServicesMu.RLock()
	n := len(c.socksServices)
	c.socksServicesMu.RUnlock()
	if n != 0 {
		t.Fatalf("socks service leaked after bind failure: count=%d", n)
	}
}

// TestMissingNodeUUID matches the lookup start_socks does before binding.
func TestMissingNodeUUID(t *testing.T) {
	c := NewController(&Options{Secret: "t", Listen: "127.0.0.1:0"})
	go c.Topology.Run()

	res := c.Topology.Do(&topology.Task{Mode: topology.GetUUID, UUIDNum: 0})
	if res.UUID != "" {
		t.Fatalf("expected empty UUID for missing node, got %q", res.UUID)
	}
	res = c.Topology.Do(&topology.Task{Mode: topology.GetUUID, UUIDNum: 99})
	if res.UUID != "" {
		t.Fatalf("expected empty UUID for id 99, got %q", res.UUID)
	}
}
