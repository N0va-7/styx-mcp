package node

import (
	"strings"
	"testing"
)

func TestValidateTransport(t *testing.T) {
	if err := validateTransport("raw"); err != nil {
		t.Fatalf("raw: %v", err)
	}
	if err := validateTransport(""); err != nil {
		t.Fatalf("empty: %v", err)
	}
	err := validateTransport("ws")
	if err == nil {
		t.Fatal("expected ws error")
	}
	if !strings.Contains(err.Error(), "not implemented") {
		t.Fatalf("msg=%q", err.Error())
	}
	if err := validateTransport("quic"); err == nil {
		t.Fatal("expected unknown transport error")
	}
}
