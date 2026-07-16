package controller

import (
	"testing"

	"styx-mcp/pkg/protocol"
)

func TestAsMsgOK(t *testing.T) {
	in := &protocol.SocksReady{OK: 1}
	got, ok := asMsg[*protocol.SocksReady](in, "SOCKSREADY", "node")
	if !ok || got == nil || got.OK != 1 {
		t.Fatalf("asMsg ok failed: ok=%v got=%v", ok, got)
	}
}

func TestAsMsgWrongType(t *testing.T) {
	got, ok := asMsg[*protocol.SocksReady](&protocol.ListenRes{OK: 1}, "SOCKSREADY", "node")
	if ok || got != nil {
		t.Fatalf("expected fail, ok=%v got=%v", ok, got)
	}
}
