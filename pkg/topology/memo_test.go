package topology

import (
	"testing"

	"styx-mcp/pkg/protocol"
)

func TestUpdateMemo(t *testing.T) {
	topo := NewTopology()
	go topo.Run()

	add := topo.Do(&Task{
		Mode:       AddNode,
		Target:     NewNode("nodeaaaaaa", "10.0.0.1"),
		ParentUUID: protocol.ControllerUUID,
		IsFirst:    true,
	})
	if add.IDNum < 0 {
		t.Fatal("add failed")
	}

	topo.Do(&Task{Mode: UpdateMemo, UUID: "nodeaaaaaa", Memo: "foothold-web"})

	got := topo.Do(&Task{Mode: GetNode, UUID: "nodeaaaaaa"})
	if !got.IsExist || got.Node == nil {
		t.Fatal("node missing after memo")
	}
	if got.Node.Memo != "foothold-web" {
		t.Fatalf("memo=%q want foothold-web", got.Node.Memo)
	}

	list := topo.Do(&Task{Mode: ListAll})
	if len(list.Nodes) != 1 || list.Nodes[0].Node.Memo != "foothold-web" {
		t.Fatalf("list memo mismatch: %+v", list.Nodes)
	}
}
