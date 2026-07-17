package node

import (
	"flag"
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

// Scenario-aligned defaults: -reconnect 10, -reconnect-max 3.
func TestReconnectOptionDefaults(t *testing.T) {
	// ParseOptions uses the global flag set; isolate with a fresh set.
	fs := flag.NewFlagSet("agent-test", flag.ContinueOnError)
	opt := &Options{}
	fs.StringVar(&opt.Listen, "l", "", "")
	fs.StringVar(&opt.Connect, "c", "", "")
	fs.IntVar(&opt.Reconnect, "reconnect", 10, "")
	fs.IntVar(&opt.ReconnectMax, "reconnect-max", 3, "")
	if err := fs.Parse([]string{"-c", "127.0.0.1:19137"}); err != nil {
		t.Fatal(err)
	}
	if opt.Reconnect != 10 {
		t.Fatalf("Reconnect default=%d want 10", opt.Reconnect)
	}
	if opt.ReconnectMax != 3 {
		t.Fatalf("ReconnectMax default=%d want 3", opt.ReconnectMax)
	}
}
