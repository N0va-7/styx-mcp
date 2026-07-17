package controller

import (
	"net"
	"testing"
	"time"

	"styx-mcp/pkg/topology"
)

// Scenario: Offline generation guard — stale readLoop must not DelNode a new session.
func TestNodeOfflineGenerationGuard(t *testing.T) {
	c := NewController(&Options{Listen: "127.0.0.1:0", Secret: "test"})
	go c.Topology.Run()

	// Pipe pair: old and new "connections".
	oldA, oldB := net.Pipe()
	defer oldA.Close()
	defer oldB.Close()
	newA, newB := net.Pipe()
	defer newA.Close()
	defer newB.Close()

	uuid := "nodeaaaaaa"
	c.Topology.Do(&topology.Task{
		Mode:    topology.AddNode,
		Target:  topology.NewNode(uuid, "10.0.0.1"),
		IsFirst: true,
	})
	c.Topology.Do(&topology.Task{Mode: topology.Calculate})

	c.connsMu.Lock()
	c.conns[uuid] = oldA
	c.connsMu.Unlock()

	// Simulate reconnect replacing conn.
	c.connsMu.Lock()
	oldA.Close()
	c.conns[uuid] = newA
	c.connsMu.Unlock()

	// Stale offline from old generation must be ignored.
	c.nodeOffline(uuid, oldA)

	list := c.Topology.Do(&topology.Task{Mode: topology.ListAll})
	if len(list.Nodes) != 1 {
		t.Fatalf("stale offline removed node; ListAll=%d", len(list.Nodes))
	}

	// Offline from current conn removes the node.
	c.nodeOffline(uuid, newA)
	list = c.Topology.Do(&topology.Task{Mode: topology.ListAll})
	if len(list.Nodes) != 0 {
		t.Fatalf("current offline did not remove; ListAll=%d", len(list.Nodes))
	}

	// Drain pipe ends so no goroutine leaks from closes.
	_ = oldB
	_ = newB
}

// Scenario: Controller active dial gives up after max.
func TestActiveDialGivesUpAfterMax(t *testing.T) {
	// Unreachable port on loopback (connection refused quickly).
	c := NewController(&Options{
		Connect:      "127.0.0.1:1",
		Secret:       "test",
		ReconnectMax: 2,
	})

	start := time.Now()
	_, err := c.activeConnect()
	if err == nil {
		t.Fatal("expected dial failure")
	}
	elapsed := time.Since(start)
	// 2 attempts + 1s sleep between → roughly >= 1s, well under infinite hang.
	if elapsed < 500*time.Millisecond {
		t.Fatalf("expected backoff between attempts, elapsed=%v", elapsed)
	}
	if elapsed > 10*time.Second {
		t.Fatalf("dial loop too long: %v", elapsed)
	}
}

func TestActiveDialMaxZeroIsSingleTry(t *testing.T) {
	c := NewController(&Options{
		Connect:      "127.0.0.1:1",
		Secret:       "test",
		ReconnectMax: 0,
	})
	start := time.Now()
	_, err := c.activeConnect()
	if err == nil {
		t.Fatal("expected dial failure")
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("single try should be quick, elapsed=%v", time.Since(start))
	}
}
