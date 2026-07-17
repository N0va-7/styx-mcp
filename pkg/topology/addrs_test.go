package topology

import (
	"testing"

	"styx-mcp/pkg/protocol"
)

func TestUpdateDetailLocalAddrs(t *testing.T) {
	topo := NewTopology()
	go topo.Run()

	add := topo.Do(&Task{
		Mode:       AddNode,
		Target:     NewNode("nodeaaaaaa", "10.7.11.116:1234"),
		ParentUUID: protocol.ControllerUUID,
		IsFirst:    true,
	})
	if add.IDNum < 0 {
		t.Fatal("add failed")
	}

	topo.Do(&Task{
		Mode:       UpdateDetail,
		UUID:       "nodeaaaaaa",
		UserName:   "oracle",
		HostName:   "wl",
		LocalAddrs: []string{"172.16.23.20", "10.10.5.20"},
	})

	got := topo.Do(&Task{Mode: GetNode, UUID: "nodeaaaaaa"})
	if !got.IsExist || got.Node == nil {
		t.Fatal("missing node")
	}
	if got.Node.CurrentIP != "10.7.11.116:1234" {
		t.Fatalf("peer ip=%q", got.Node.CurrentIP)
	}
	if len(got.Node.LocalAddrs) != 2 || got.Node.LocalAddrs[0] != "172.16.23.20" {
		t.Fatalf("local=%v", got.Node.LocalAddrs)
	}
}
