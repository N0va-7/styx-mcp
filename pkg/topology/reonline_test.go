package topology

import (
	"testing"

	"styx-mcp/pkg/protocol"
)

// Scenario: Same node_id after reconnect (topology delta).
func TestReonlinePreservesNumericID(t *testing.T) {
	topo := NewTopology()
	go topo.Run()

	add := topo.Do(&Task{
		Mode:       AddNode,
		Target:     NewNode("nodeaaaaaa", "10.0.0.1"),
		ParentUUID: protocol.ControllerUUID,
		IsFirst:    true,
	})
	if add.IDNum != 0 {
		t.Fatalf("first id=%d want 0", add.IDNum)
	}

	topo.Do(&Task{Mode: DelNode, UUID: "nodeaaaaaa"})

	list := topo.Do(&Task{Mode: ListAll})
	if len(list.Nodes) != 0 {
		t.Fatalf("after offline ListAll=%d want 0", len(list.Nodes))
	}

	topo.Do(&Task{
		Mode:       ReonlineNode,
		Target:     NewNode("nodeaaaaaa", "10.0.0.1"),
		ParentUUID: protocol.ControllerUUID,
		IsFirst:    true,
	})
	topo.Do(&Task{Mode: Calculate})

	list = topo.Do(&Task{Mode: ListAll})
	if len(list.Nodes) != 1 {
		t.Fatalf("after reonline ListAll=%d want 1", len(list.Nodes))
	}
	if list.Nodes[0].ID != 0 || list.Nodes[0].Node.UUID != "nodeaaaaaa" {
		t.Fatalf("reonline entry=%+v want id=0 uuid=nodeaaaaaa", list.Nodes[0])
	}

	// A distinct new agent still gets a new id (not reusing history of another UUID).
	add2 := topo.Do(&Task{
		Mode:       AddNode,
		Target:     NewNode("nodebbbbbb", "10.0.0.2"),
		ParentUUID: protocol.ControllerUUID,
		IsFirst:    true,
	})
	if add2.IDNum != 1 {
		t.Fatalf("new agent id=%d want 1", add2.IDNum)
	}
}

// Scenario: Drop then empty then reappear.
func TestOfflineRemovesFromOnlineSet(t *testing.T) {
	topo := NewTopology()
	go topo.Run()

	topo.Do(&Task{
		Mode:    AddNode,
		Target:  NewNode("nodeaaaaaa", "10.0.0.1"),
		IsFirst: true,
	})
	topo.Do(&Task{Mode: DelNode, UUID: "nodeaaaaaa"})
	if n := len(topo.Do(&Task{Mode: ListAll}).Nodes); n != 0 {
		t.Fatalf("online after drop=%d", n)
	}
	topo.Do(&Task{
		Mode:    ReonlineNode,
		Target:  NewNode("nodeaaaaaa", "10.0.0.1"),
		IsFirst: true,
	})
	list := topo.Do(&Task{Mode: ListAll})
	if len(list.Nodes) != 1 || list.Nodes[0].ID != 0 {
		t.Fatalf("reappear=%+v", list.Nodes)
	}
}
