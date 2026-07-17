package scan

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Dialer abstracts TCP connect for unit tests / fingerprint.
type Dialer interface {
	DialContext(ctx context.Context, network, address string) (net.Conn, error)
}

// NetDialer is a Dialer backed by net.Dialer with per-dial timeout.
type NetDialer struct {
	Timeout time.Duration
}

// DialContext implements Dialer.
func (d NetDialer) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	nd := &net.Dialer{Timeout: d.Timeout}
	return nd.DialContext(ctx, network, address)
}

// Config drives a single port-scan job (hosts already filtered if discover ran).
type Config struct {
	Hosts       []string
	Ports       []uint16
	Concurrency int
	Timeout     time.Duration
	// MaxDuration is wall-clock budget; 0 means no extra cap beyond context.
	MaxDuration time.Duration
	// MaxDials hard-stops after this many attempts (0 = unlimited).
	MaxDials int
	// Checker: if set, used for open/closed tests (SYN or connect).
	// If nil, builds connect checker (or uses Dialer only for connect).
	Checker PortChecker
	// Dialer kept for tests that inject fake dialer without checker.
	Dialer Dialer
}

// Defaults and ceilings.
const (
	DefaultConcurrency = 200
	MaxConcurrency     = 500
	DefaultTimeoutMs   = 500
	MaxTimeoutMs       = 5000
	DefaultMaxDuration = 5 * time.Minute
	MaxMaxDuration     = 30 * time.Minute
	// Full mode still gets a wall-clock ceiling unless caller sets lower.
	FullDefaultMaxDuration = 15 * time.Minute
)

// OpenPort is one successful open port.
type OpenPort struct {
	IP    string `json:"ip"`
	Port  uint16 `json:"port"`
	Proto string `json:"proto"`
	State string `json:"state"`
}

// Stats summarizes dial/probe work.
type Stats struct {
	HostsTotal    int    `json:"hosts_total"`
	HostsAlive    int    `json:"hosts_alive,omitempty"`
	HostsWithOpen int    `json:"hosts_with_open"`
	PortsTried    int64  `json:"ports_tried"`
	Open          int    `json:"open"`
	DurationMs    int64  `json:"duration_ms"`
	DiscoverMs    int64  `json:"discover_ms,omitempty"`
	Method        string `json:"method,omitempty"`
}

// Result is the port-scan phase output (no fingerprint).
type Result struct {
	Open  []OpenPort `json:"open"`
	Stats Stats      `json:"stats"`
}

// ProgressFn is optional; called with partial open list and stats.
type ProgressFn func(open []OpenPort, stats Stats)

// NormalizeConfig clamps knobs.
func NormalizeConfig(c Config) Config {
	if c.Concurrency <= 0 {
		c.Concurrency = DefaultConcurrency
	}
	if c.Concurrency > MaxConcurrency {
		c.Concurrency = MaxConcurrency
	}
	if c.Timeout <= 0 {
		c.Timeout = time.Duration(DefaultTimeoutMs) * time.Millisecond
	}
	maxTO := time.Duration(MaxTimeoutMs) * time.Millisecond
	if c.Timeout > maxTO {
		c.Timeout = maxTO
	}
	if c.MaxDuration < 0 {
		c.MaxDuration = 0
	}
	if c.MaxDuration > MaxMaxDuration {
		c.MaxDuration = MaxMaxDuration
	}
	if c.Dialer == nil {
		c.Dialer = NetDialer{Timeout: c.Timeout}
	}
	return c
}

// Run performs port open checks on cfg.Hosts × cfg.Ports.
// Prefer Discover first so Hosts is the alive set.
func Run(ctx context.Context, cfg Config, onProgress ProgressFn) (*Result, error) {
	cfg = NormalizeConfig(cfg)
	if len(cfg.Hosts) == 0 {
		return &Result{Open: nil, Stats: Stats{HostsTotal: 0, Method: methodOf(cfg)}}, nil
	}
	if len(cfg.Ports) == 0 {
		return nil, fmt.Errorf("no ports")
	}

	if cfg.MaxDuration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, cfg.MaxDuration)
		defer cancel()
	}

	checker := cfg.Checker
	ownedChecker := false
	if checker == nil {
		// Prefer explicit Dialer (tests) via adapter; else connect checker.
		if cfg.Dialer != nil {
			if _, ok := cfg.Dialer.(NetDialer); !ok {
				// custom dialer (e.g. fake) — wrap as checker
				checker = &dialerChecker{d: cfg.Dialer, timeout: cfg.Timeout}
			}
		}
		if checker == nil {
			var err error
			checker, err = NewPortChecker(MethodConnect, cfg.Timeout)
			if err != nil {
				return nil, err
			}
			ownedChecker = true
		}
	}
	if ownedChecker {
		defer checker.Close()
	}

	start := time.Now()
	type job struct {
		ip   string
		port uint16
	}

	jobs := make(chan job, cfg.Concurrency*2)
	var (
		mu         sync.Mutex
		open       []OpenPort
		hostsOpen  = make(map[string]struct{})
		portsTried int64
		dialBudget int64
		budgetOK   = cfg.MaxDials <= 0
	)
	if cfg.MaxDials > 0 {
		dialBudget = int64(cfg.MaxDials)
	}

	var wg sync.WaitGroup
	worker := func() {
		defer wg.Done()
		for j := range jobs {
			if ctx.Err() != nil {
				continue
			}
			if !budgetOK {
				if atomic.AddInt64(&dialBudget, -1) < 0 {
					continue
				}
			}
			atomic.AddInt64(&portsTried, 1)
			ok, _ := checker.Open(ctx, j.ip, j.port)
			if !ok {
				continue
			}
			mu.Lock()
			open = append(open, OpenPort{
				IP:    j.ip,
				Port:  j.port,
				Proto: "tcp",
				State: "open",
			})
			hostsOpen[j.ip] = struct{}{}
			if onProgress != nil {
				stats := Stats{
					HostsTotal:    len(cfg.Hosts),
					HostsWithOpen: len(hostsOpen),
					PortsTried:    atomic.LoadInt64(&portsTried),
					Open:          len(open),
					DurationMs:    time.Since(start).Milliseconds(),
					Method:        checker.Method(),
				}
				snap := append([]OpenPort(nil), open...)
				mu.Unlock()
				onProgress(snap, stats)
			} else {
				mu.Unlock()
			}
		}
	}

	nWorkers := cfg.Concurrency
	wg.Add(nWorkers)
	for i := 0; i < nWorkers; i++ {
		go worker()
	}

feed:
	for _, host := range cfg.Hosts {
		for _, port := range cfg.Ports {
			if ctx.Err() != nil {
				break feed
			}
			select {
			case <-ctx.Done():
				break feed
			case jobs <- job{ip: host, port: port}:
			}
		}
	}
	close(jobs)
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	return &Result{
		Open: open,
		Stats: Stats{
			HostsTotal:    len(cfg.Hosts),
			HostsWithOpen: len(hostsOpen),
			PortsTried:    atomic.LoadInt64(&portsTried),
			Open:          len(open),
			DurationMs:    time.Since(start).Milliseconds(),
			Method:        checker.Method(),
		},
	}, nil
}

func methodOf(cfg Config) string {
	if cfg.Checker != nil {
		return cfg.Checker.Method()
	}
	return MethodConnect
}

// dialerChecker adapts Dialer to PortChecker (tests).
type dialerChecker struct {
	d       Dialer
	timeout time.Duration
}

func (c *dialerChecker) Method() string { return MethodConnect }
func (c *dialerChecker) Close() error   { return nil }

func (c *dialerChecker) Open(ctx context.Context, ip string, port uint16) (bool, error) {
	dctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	conn, err := c.d.DialContext(dctx, "tcp", joinHostPort(ip, port))
	if err != nil {
		return false, nil
	}
	_ = conn.Close()
	return true, nil
}
