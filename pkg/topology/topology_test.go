package topology

import (
	"fmt"
	"sync"
	"testing"

	"styx-mcp/pkg/protocol"
)

func TestListAllSkipsSparseIDs(t *testing.T) {
	topo := NewTopology()
	go topo.Run()

	for _, uuid := range []string{"nodeaaaaaa", "nodebbbbbb", "nodecccccc"} {
		res := topo.Do(&Task{
			Mode:       AddNode,
			Target:     NewNode(uuid, "10.0.0.1"),
			ParentUUID: protocol.ControllerUUID,
			IsFirst:    true,
		})
		if res.IDNum < 0 {
			t.Fatalf("add node failed for %s", uuid)
		}
	}

	topo.Do(&Task{Mode: DelNode, UUID: "nodebbbbbb"})

	res := topo.Do(&Task{Mode: ListAll})
	if len(res.Nodes) != 2 {
		t.Fatalf("ListAll returned %d nodes, want 2", len(res.Nodes))
	}
	if res.Nodes[0].ID != 0 || res.Nodes[0].Node.UUID != "nodeaaaaaa" {
		t.Fatalf("first entry = %+v", res.Nodes[0])
	}
	if res.Nodes[1].ID != 2 || res.Nodes[1].Node.UUID != "nodecccccc" {
		t.Fatalf("second entry = %+v", res.Nodes[1])
	}

	miss := topo.Do(&Task{Mode: GetUUID, UUIDNum: 1})
	if miss.UUID != "" {
		t.Fatalf("expected gap at id 1, got %q", miss.UUID)
	}
}

func TestDoCorrelatesConcurrentResults(t *testing.T) {
	topo := NewTopology()
	go topo.Run()

	const n = 32
	for i := 0; i < n; i++ {
		uuid := formatID(i)
		res := topo.Do(&Task{
			Mode:    AddNode,
			Target:  NewNode(uuid, "10.0.0.1"),
			IsFirst: true,
		})
		if res.IDNum != i {
			t.Fatalf("add %d: id=%d", i, res.IDNum)
		}
	}

	var wg sync.WaitGroup
	errs := make(chan string, n*2)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			want := formatID(id)
			res := topo.Do(&Task{Mode: GetUUID, UUIDNum: id})
			if res.UUID != want {
				errs <- fmt.Sprintf("GetUUID(%d)=%q want %q", id, res.UUID, want)
				return
			}
			nodeRes := topo.Do(&Task{Mode: GetNode, UUID: want})
			if !nodeRes.IsExist || nodeRes.Node == nil || nodeRes.Node.UUID != want {
				errs <- fmt.Sprintf("GetNode(%s) mismatch", want)
			}
		}(i)
	}
	wg.Wait()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
}

func formatID(i int) string {
	// Exactly 10 chars for wire-style UUIDs.
	return fmt.Sprintf("n%08x", i)
}
