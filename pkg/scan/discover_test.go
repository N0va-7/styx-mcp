package scan

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

type mapDialer struct {
	open map[string]bool
	hits int64
}

func (m *mapDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	atomic.AddInt64(&m.hits, 1)
	if m.open[address] {
		c1, c2 := net.Pipe()
		_ = c2.Close()
		return c1, nil
	}
	return nil, context.DeadlineExceeded
}

func TestDiscoverOnlyAliveTCP(t *testing.T) {
	fd := &mapDialer{open: map[string]bool{
		"10.0.0.1:80": true,
		"10.0.0.3:22": true,
	}}
	checker := &dialerChecker{d: fd, timeout: 50 * time.Millisecond}
	res, err := Discover(context.Background(), DiscoverConfig{
		Hosts:       []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"},
		ProbePorts:  []uint16{22, 80},
		Concurrency: 4,
		Timeout:     50 * time.Millisecond,
		Checker:     checker,
		SkipICMP:    true, // deterministic unit test
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Alive) != 2 {
		t.Fatalf("alive=%v", res.Alive)
	}
	if res.TCPNewAlive != 2 {
		t.Fatalf("tcp_new=%d", res.TCPNewAlive)
	}
	if res.Mode != "tcp-only" {
		t.Fatalf("mode=%s", res.Mode)
	}
}

func TestDiscoverNoneAliveTCP(t *testing.T) {
	fd := &mapDialer{open: map[string]bool{}}
	checker := &dialerChecker{d: fd, timeout: 20 * time.Millisecond}
	res, err := Discover(context.Background(), DiscoverConfig{
		Hosts:       []string{"10.0.0.1", "10.0.0.2"},
		ProbePorts:  []uint16{80},
		Concurrency: 2,
		Timeout:     20 * time.Millisecond,
		Checker:     checker,
		SkipICMP:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Alive) != 0 {
		t.Fatalf("alive=%v", res.Alive)
	}
}

func TestDefaultProbePortsSize(t *testing.T) {
	if len(DefaultProbePorts) < 20 || len(DefaultProbePorts) > 50 {
		t.Fatalf("probe set size %d", len(DefaultProbePorts))
	}
	// spot-check
	need := map[uint16]bool{22: false, 80: false, 445: false, 7001: false, 27017: false}
	for _, p := range DefaultProbePorts {
		if _, ok := need[p]; ok {
			need[p] = true
		}
	}
	for p, ok := range need {
		if !ok {
			t.Errorf("missing probe port %d", p)
		}
	}
}

func TestResolveMethod(t *testing.T) {
	m, err := ResolveMethod("")
	if err != nil || m != MethodAuto {
		t.Fatalf("%q %v", m, err)
	}
	if _, err := ResolveMethod("bogus"); err == nil {
		t.Fatal("expected error")
	}
}

func TestNewPortCheckerConnect(t *testing.T) {
	c, err := NewPortChecker(MethodConnect, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if c.Method() != MethodConnect {
		t.Fatalf("method=%s", c.Method())
	}
}

func TestNewPortCheckerSynWithoutCap(t *testing.T) {
	if CanRawTCP() {
		t.Skip("raw TCP available; cannot assert failure")
	}
	_, err := NewPortChecker(MethodSYN, 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected syn unavailable error")
	}
}

func TestPingArgsLinuxShape(t *testing.T) {
	args := pingArgs("1.2.3.4", 400*time.Millisecond)
	if len(args) < 2 || args[0] != "ping" {
		t.Fatalf("args=%v", args)
	}
}

func TestDiscoverProgressCallback(t *testing.T) {
	fd := &mapDialer{open: map[string]bool{"10.0.0.1:80": true}}
	checker := &dialerChecker{d: fd, timeout: 30 * time.Millisecond}
	var stages []string
	var lastAlive int
	_, err := Discover(context.Background(), DiscoverConfig{
		Hosts:         []string{"10.0.0.1", "10.0.0.2"},
		ProbePorts:    []uint16{80},
		Concurrency:   2,
		Timeout:       30 * time.Millisecond,
		Checker:       checker,
		SkipICMP:      true,
		ProgressEvery: 1,
		OnProgress: func(p DiscoverProgress) {
			stages = append(stages, p.Stage)
			lastAlive = p.AliveN
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(stages) == 0 {
		t.Fatal("expected progress callbacks")
	}
	// Should end with tcp_done
	if stages[len(stages)-1] != "tcp_done" {
		t.Fatalf("stages=%v", stages)
	}
	if lastAlive < 1 {
		t.Fatalf("alive_n=%d", lastAlive)
	}
}
