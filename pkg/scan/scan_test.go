package scan

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"time"
)

type fakeDialer struct {
	open map[string]bool // "ip:port"
	hits int64
}

func (f *fakeDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	atomic.AddInt64(&f.hits, 1)
	if f.open[address] {
		// Pipe ends act as successful "connections".
		c1, c2 := net.Pipe()
		_ = c2.Close()
		return c1, nil
	}
	return nil, context.DeadlineExceeded
}

func TestRunFakeDialer(t *testing.T) {
	fd := &fakeDialer{open: map[string]bool{
		"10.0.0.1:80":  true,
		"10.0.0.1:443": true,
		"10.0.0.2:22":  true,
	}}
	res, err := Run(context.Background(), Config{
		Hosts:       []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"},
		Ports:       []uint16{22, 80, 443},
		Concurrency: 4,
		Timeout:     50 * time.Millisecond,
		Dialer:      fd,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stats.Open != 3 {
		t.Fatalf("open=%d want 3; open=%v", res.Stats.Open, res.Open)
	}
	if res.Stats.HostsWithOpen != 2 {
		t.Fatalf("hosts_with_open=%d", res.Stats.HostsWithOpen)
	}
	if res.Stats.PortsTried != 9 {
		t.Fatalf("ports_tried=%d want 9", res.Stats.PortsTried)
	}
	if res.Stats.HostsTotal != 3 {
		t.Fatalf("hosts_total=%d", res.Stats.HostsTotal)
	}
	if res.Stats.DurationMs < 0 {
		t.Fatal("duration")
	}
}

func TestRunMaxDials(t *testing.T) {
	fd := &fakeDialer{open: map[string]bool{}}
	res, err := Run(context.Background(), Config{
		Hosts:       []string{"10.0.0.1"},
		Ports:       []uint16{1, 2, 3, 4, 5},
		Concurrency: 2,
		Timeout:     20 * time.Millisecond,
		MaxDials:    2,
		Dialer:      fd,
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if res.Stats.PortsTried > 2 {
		t.Fatalf("ports_tried=%d want <=2", res.Stats.PortsTried)
	}
}

func TestNormalizeConfigCeilings(t *testing.T) {
	c := NormalizeConfig(Config{Concurrency: 9999, Timeout: time.Hour, MaxDuration: time.Hour})
	if c.Concurrency != MaxConcurrency {
		t.Fatalf("concurrency %d", c.Concurrency)
	}
	if c.Timeout != time.Duration(MaxTimeoutMs)*time.Millisecond {
		t.Fatalf("timeout %v", c.Timeout)
	}
	if c.MaxDuration != MaxMaxDuration {
		t.Fatalf("max duration %v", c.MaxDuration)
	}
}
