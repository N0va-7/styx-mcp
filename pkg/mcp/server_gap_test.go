package mcp

import (
	"context"
	"encoding/json"
	"net"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"

	"styx-mcp/pkg/controller"
)

func toolText(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	if res == nil || len(res.Content) == 0 {
		t.Fatal("empty tool result")
	}
	tc, ok := res.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("content type %T", res.Content[0])
	}
	return tc.Text
}

func TestHandleListNodesEmpty(t *testing.T) {
	c := controller.NewController(&controller.Options{Secret: "t", Listen: "127.0.0.1:0"})
	go c.Topology.Run()
	s := NewServer(c)

	res, err := s.handleListNodes(context.Background(), mcp.CallToolRequest{})
	if err != nil {
		t.Fatal(err)
	}
	var body map[string]interface{}
	if err := json.Unmarshal([]byte(toolText(t, res)), &body); err != nil {
		t.Fatal(err)
	}
	if body["success"] != true {
		t.Fatalf("success=%v body=%v", body["success"], body)
	}
	nodes, ok := body["nodes"].([]interface{})
	if !ok {
		t.Fatalf("nodes type %T body=%v", body["nodes"], body)
	}
	if len(nodes) != 0 {
		t.Fatalf("want empty nodes, got %v", nodes)
	}
}

func TestHandleStartSocksUnknownNode(t *testing.T) {
	c := controller.NewController(&controller.Options{Secret: "t", Listen: "127.0.0.1:0"})
	go c.Topology.Run()
	s := NewServer(c)

	// Pick a free port so we can prove start_socks never binds it on reject.
	holder, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := holder.Addr().String()
	holder.Close()

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"node_id": float64(42),
		"address": addr,
	}
	res, err := s.handleStartSocks(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	text := toolText(t, res)
	var body map[string]interface{}
	if err := json.Unmarshal([]byte(text), &body); err != nil {
		t.Fatal(err)
	}
	if body["success"] != false {
		t.Fatalf("want failure, body=%v", body)
	}
	errMsg, _ := body["error"].(string)
	if !strings.Contains(errMsg, "not found") {
		t.Fatalf("error=%q want not found", errMsg)
	}

	// Address must still be free (no bind on unknown node).
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatalf("address should be free after unknown-node reject: %v", err)
	}
	ln.Close()
}
