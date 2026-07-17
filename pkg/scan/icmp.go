package scan

import (
	"context"
	"os/exec"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// ICMPProbeOptions configures concurrent ICMP alive checks.
type ICMPProbeOptions struct {
	Hosts       []string
	Concurrency int
	Timeout     time.Duration
	// OnProgress is called periodically (throttled by caller via Interval).
	OnProgress DiscoverProgressFn
	// ProgressEvery: emit progress every N completed pings (default 16).
	ProgressEvery int
}

// ICMPProbe pings hosts concurrently (best-effort). Missing ping binary or
// filtered ICMP simply yields no hits — never a hard error.
func ICMPProbe(ctx context.Context, hosts []string, concurrency int, timeout time.Duration) (alive []string, tried int64) {
	return ICMPProbeOpt(ctx, ICMPProbeOptions{
		Hosts: hosts, Concurrency: concurrency, Timeout: timeout,
	})
}

// ICMPProbeOpt is ICMPProbe with progress callbacks.
func ICMPProbeOpt(ctx context.Context, opt ICMPProbeOptions) (alive []string, tried int64) {
	hosts := opt.Hosts
	if len(hosts) == 0 {
		return nil, 0
	}
	concurrency := opt.Concurrency
	if concurrency <= 0 {
		concurrency = DefaultConcurrency
	}
	if concurrency > MaxConcurrency {
		concurrency = MaxConcurrency
	}
	timeout := opt.Timeout
	if timeout <= 0 {
		timeout = 400 * time.Millisecond
	}
	// System ping often has 1s minimum granularity on Linux (-W seconds).
	if runtime.GOOS == "linux" && timeout < time.Second {
		timeout = time.Second
	}
	every := opt.ProgressEvery
	if every <= 0 {
		every = 16
	}

	jobs := make(chan string, concurrency*2)
	var (
		mu       sync.Mutex
		aliveSet = make(map[string]struct{})
		nTried   int64
		nAlive   int64
		wg       sync.WaitGroup
	)

	emit := func() {
		if opt.OnProgress == nil {
			return
		}
		opt.OnProgress(DiscoverProgress{
			Stage:      "icmp",
			HostsTotal: len(hosts),
			ICMPDone:   atomic.LoadInt64(&nTried),
			ICMPTotal:  len(hosts),
			ICMPAlive:  int(atomic.LoadInt64(&nAlive)),
			AliveN:     int(atomic.LoadInt64(&nAlive)),
		})
	}

	worker := func() {
		defer wg.Done()
		for ip := range jobs {
			if ctx.Err() != nil {
				continue
			}
			done := atomic.AddInt64(&nTried, 1)
			if pingOnce(ctx, ip, timeout) {
				atomic.AddInt64(&nAlive, 1)
				mu.Lock()
				aliveSet[ip] = struct{}{}
				mu.Unlock()
			}
			if done == 1 || done%int64(every) == 0 || int(done) == len(hosts) {
				emit()
			}
		}
	}

	wg.Add(concurrency)
	for i := 0; i < concurrency; i++ {
		go worker()
	}
	for _, h := range hosts {
		if ctx.Err() != nil {
			break
		}
		select {
		case <-ctx.Done():
		case jobs <- h:
		}
	}
	close(jobs)
	wg.Wait()

	// Final icmp snapshot.
	if opt.OnProgress != nil {
		opt.OnProgress(DiscoverProgress{
			Stage:      "icmp_done",
			HostsTotal: len(hosts),
			ICMPDone:   atomic.LoadInt64(&nTried),
			ICMPTotal:  len(hosts),
			ICMPAlive:  int(atomic.LoadInt64(&nAlive)),
			AliveN:     int(atomic.LoadInt64(&nAlive)),
		})
	}

	for _, h := range hosts {
		if _, ok := aliveSet[h]; ok {
			alive = append(alive, h)
		}
	}
	return alive, atomic.LoadInt64(&nTried)
}

func pingOnce(ctx context.Context, ip string, timeout time.Duration) bool {
	args := pingArgs(ip, timeout)
	if len(args) == 0 {
		return false
	}
	pctx, cancel := context.WithTimeout(ctx, timeout+500*time.Millisecond)
	defer cancel()
	cmd := exec.CommandContext(pctx, args[0], args[1:]...)
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

func pingArgs(ip string, timeout time.Duration) []string {
	switch runtime.GOOS {
	case "windows":
		ms := int(timeout / time.Millisecond)
		if ms < 200 {
			ms = 200
		}
		return []string{"ping", "-n", "1", "-w", strconv.Itoa(ms), ip}
	case "darwin":
		ms := int(timeout / time.Millisecond)
		if ms < 200 {
			ms = 200
		}
		return []string{"ping", "-c", "1", "-W", strconv.Itoa(ms), ip}
	default:
		sec := int(timeout / time.Second)
		if sec < 1 {
			sec = 1
		}
		return []string{"ping", "-c", "1", "-n", "-W", strconv.Itoa(sec), ip}
	}
}
