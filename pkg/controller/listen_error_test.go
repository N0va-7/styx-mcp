package controller

import (
	"errors"
	"net"
	"strings"
	"syscall"
	"testing"
)

func TestListenBindErrorAddrInUse(t *testing.T) {
	err := listenBindError("127.0.0.1:19137", &net.OpError{
		Op:  "listen",
		Net: "tcp",
		Err: syscall.EADDRINUSE,
	})
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	for _, part := range []string{"127.0.0.1:19137", "already in use", "STYX_LISTEN"} {
		if !strings.Contains(msg, part) {
			t.Fatalf("missing %q in: %s", part, msg)
		}
	}
}

func TestListenBindErrorOther(t *testing.T) {
	err := listenBindError("127.0.0.1:1", errors.New("permission denied"))
	if err == nil {
		t.Fatal("expected error")
	}
	msg := err.Error()
	if strings.Contains(msg, "STYX_LISTEN") {
		t.Fatalf("should not add port-conflict hint for other errors: %s", msg)
	}
	if !strings.Contains(msg, "127.0.0.1:1") || !strings.Contains(msg, "permission denied") {
		t.Fatalf("unexpected message: %s", msg)
	}
}

func TestIsAddrInUse(t *testing.T) {
	if !isAddrInUse(syscall.EADDRINUSE) {
		t.Fatal("expected EADDRINUSE true")
	}
	if !isAddrInUse(&net.OpError{Err: syscall.EADDRINUSE}) {
		t.Fatal("expected wrapped EADDRINUSE true")
	}
	if isAddrInUse(errors.New("nope")) {
		t.Fatal("expected false")
	}
}
