package topology

import (
	"testing"

	"styx-mcp/pkg/protocol"
)

func TestListAllSkipsSparseIDs(t *testing.T) {
	topo := NewTopology()
	go topo.Run()

	// Add three nodes under admin.
	for i, uuid := range []string{"nodeaaaaaa", "nodebbbbbb", "nodecccccc"} {
		_ = i
		topo.TaskChan <- &Task{
			Mode:       AddNode,
			Target:     NewNode(uuid, "10.0.0."+uuid[4:5]),
			ParentUUID: protocol.ADMIN_UUID,
			IsFirst:    true,
		}
		res := <-topo.ResultChan
		if res.IDNum < 0 {
			t.Fatalf("add node failed for %s", uuid)
		}
	}

	// Delete middle ID (1).
	topo.TaskChan <- &Task{Mode: DelNode, UUID: "nodebbbbbb"}
	<-topo.ResultChan

	topo.TaskChan <- &Task{Mode: ListAll}
	res := <-topo.ResultChan
	if len(res.Nodes) != 2 {
		t.Fatalf("ListAll returned %d nodes, want 2", len(res.Nodes))
	}
	if res.Nodes[0].ID != 0 || res.Nodes[0].Node.UUID != "nodeaaaaaa" {
		t.Fatalf("first entry = %+v", res.Nodes[0])
	}
	if res.Nodes[1].ID != 2 || res.Nodes[1].Node.UUID != "nodecccccc" {
		t.Fatalf("second entry = %+v", res.Nodes[1])
	}

	// Legacy GetUUID(1) is empty (sparse), but ListAll still sees ID 2.
	topo.TaskChan <- &Task{Mode: GetUUID, UUIDNum: 1}
	miss := <-topo.ResultChan
	if miss.UUID != "" {
		t.Fatalf("expected gap at id 1, got %q", miss.UUID)
	}
}
