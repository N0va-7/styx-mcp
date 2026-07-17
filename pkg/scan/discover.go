package scan

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// DefaultProbePorts: thicker high-value set for TCP alive discovery.
var DefaultProbePorts = []uint16{
	21, 22, 23, 25, 80, 81, 110, 135, 139, 143,
	443, 445, 993, 995, 1433, 1521, 2181, 3306, 3389, 5432,
	5900, 6379, 7001, 8000, 8080, 8443, 8888, 9000, 9200, 11211,
	27017,
}

// DiscoverConfig controls host alive probes.
type DiscoverConfig struct {
	Hosts       []string
	ProbePorts  []uint16
	Concurrency int
	Timeout     time.Duration
	Checker     PortChecker
	// SkipICMP disables the ICMP phase (unit tests).
	SkipICMP bool
	// OnProgress receives throttled discover snapshots (icmp/tcp stages).
	OnProgress DiscoverProgressFn
	// ProgressEvery: emit every N ICMP completions / TCP probes (default 16).
	ProgressEvery int
}

// DiscoverResult is the output of host discovery.
type DiscoverResult struct {
	Alive       []string
	HostsTotal  int
	Probes      int64
	ICMPTried   int64
	ICMPAlive   int
	TCPNewAlive int
	DurationMs  int64
	Method      string // connect|syn
	Mode        string // hybrid|tcp-only
}

// Discover finds hosts that answer ICMP and/or have an open probe port.
//
//	alive = ICMP success OR any TCP/SYN probe open
func Discover(ctx context.Context, cfg DiscoverConfig) (*DiscoverResult, error) {
	if len(cfg.Hosts) == 0 {
		return nil, errf("no hosts")
	}
	ports := cfg.ProbePorts
	if len(ports) == 0 {
		ports = append([]uint16(nil), DefaultProbePorts...)
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = 400 * time.Millisecond
	}
	concurrency := cfg.Concurrency
	if concurrency <= 0 {
		concurrency = DefaultConcurrency
	}
	if concurrency > MaxConcurrency {
		concurrency = MaxConcurrency
	}
	every := cfg.ProgressEvery
	if every <= 0 {
		every = 16
	}
	checker := cfg.Checker
	owned := false
	if checker == nil {
		var err error
		checker, err = NewPortChecker(MethodAuto, timeout)
		if err != nil {
			return nil, err
		}
		owned = true
	}
	if owned {
		defer checker.Close()
	}

	start := time.Now()
	aliveFlag := make(map[string]*atomic.Bool, len(cfg.Hosts))
	for _, h := range cfg.Hosts {
		aliveFlag[h] = &atomic.Bool{}
	}

	countAlive := func() int {
		n := 0
		for _, f := range aliveFlag {
			if f.Load() {
				n++
			}
		}
		return n
	}

	var icmpTried int64
	icmpAliveN := 0
	mode := "tcp-only"

	// Phase 1: best-effort ICMP.
	if !cfg.SkipICMP {
		mode = "hybrid"
		icmpHits, tried := ICMPProbeOpt(ctx, ICMPProbeOptions{
			Hosts:         cfg.Hosts,
			Concurrency:   concurrency,
			Timeout:       timeout,
			ProgressEvery: every,
			OnProgress:    cfg.OnProgress,
		})
		icmpTried = tried
		icmpAliveN = len(icmpHits)
		for _, ip := range icmpHits {
			if f := aliveFlag[ip]; f != nil {
				f.Store(true)
			}
		}
	}

	// Phase 2: TCP/SYN only for hosts not yet alive.
	type job struct {
		ip   string
		port uint16
	}
	jobs := make(chan job, concurrency*2)
	var (
		probes      int64
		tcpNewAlive int64
		wg          sync.WaitGroup
	)

	emitTCP := func(stage string) {
		if cfg.OnProgress == nil {
			return
		}
		cfg.OnProgress(DiscoverProgress{
			Stage:      stage,
			HostsTotal: len(cfg.Hosts),
			ICMPDone:   icmpTried,
			ICMPTotal:  len(cfg.Hosts),
			ICMPAlive:  icmpAliveN,
			TCPProbes:  atomic.LoadInt64(&probes),
			AliveN:     countAlive(),
			Method:     checker.Method(),
		})
	}

	worker := func() {
		defer wg.Done()
		for j := range jobs {
			if ctx.Err() != nil {
				continue
			}
			if aliveFlag[j.ip].Load() {
				continue
			}
			n := atomic.AddInt64(&probes, 1)
			ok, _ := checker.Open(ctx, j.ip, j.port)
			if ok {
				if aliveFlag[j.ip].CompareAndSwap(false, true) {
					atomic.AddInt64(&tcpNewAlive, 1)
				}
			}
			if n == 1 || n%int64(every) == 0 {
				emitTCP("tcp")
			}
		}
	}

	nWorkers := concurrency
	wg.Add(nWorkers)
	for i := 0; i < nWorkers; i++ {
		go worker()
	}

feed:
	for _, host := range cfg.Hosts {
		if aliveFlag[host].Load() {
			continue
		}
		for _, port := range ports {
			if ctx.Err() != nil {
				break feed
			}
			if aliveFlag[host].Load() {
				break
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
	emitTCP("tcp_done")

	alive := make([]string, 0)
	for _, h := range cfg.Hosts {
		if aliveFlag[h].Load() {
			alive = append(alive, h)
		}
	}

	return &DiscoverResult{
		Alive:       alive,
		HostsTotal:  len(cfg.Hosts),
		Probes:      atomic.LoadInt64(&probes),
		ICMPTried:   icmpTried,
		ICMPAlive:   icmpAliveN,
		TCPNewAlive: int(atomic.LoadInt64(&tcpNewAlive)),
		DurationMs:  time.Since(start).Milliseconds(),
		Method:      checker.Method(),
		Mode:        mode,
	}, nil
}

// DiscoverTimeout returns a short timeout for alive probes.
func DiscoverTimeout(scanTimeout time.Duration) time.Duration {
	d := 400 * time.Millisecond
	if scanTimeout > 0 && scanTimeout < d {
		return scanTimeout
	}
	return d
}
