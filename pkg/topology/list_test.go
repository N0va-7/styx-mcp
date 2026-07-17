package topology

import "testing"

func TestListAllEmpty(t *testing.T) {
	topo := NewTopology()
	go topo.Run()

	res := topo.Do(&Task{Mode: ListAll})
	if res.Nodes == nil {
		// empty slice preferred; nil also means no nodes for callers that range
		return
	}
	if len(res.Nodes) != 0 {
		t.Fatalf("want empty list, got %d nodes", len(res.Nodes))
	}
}
