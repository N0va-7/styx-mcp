package node

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"time"

	"styx-mcp/pkg/fingerprint"
	"styx-mcp/pkg/protocol"
	"styx-mcp/pkg/scan"
)

const (
	scanDefaultConcurrency = 200
	scanDefaultTimeoutMs   = 500
	scanDefaultMaxHosts    = 1024
	scanDefaultMaxDuration = 300 // seconds
	scanFullMaxDuration    = 900
)

func (n *Node) handleScanReq(req *protocol.ScanReq) {
	go n.runScan(req)
}

func (n *Node) runScan(req *protocol.ScanReq) {
	taskID := req.TaskID
	start := time.Now()

	sendFail := func(msg string) {
		n.sendScanRes(taskID, false, msg, nil)
	}

	mode := req.Mode
	if mode == "" {
		mode = scan.ModeFast
	}
	ports, err := scan.ResolvePorts(mode, req.Ports)
	if err != nil {
		sendFail(err.Error())
		return
	}

	maxHosts := int(req.MaxHosts)
	if maxHosts <= 0 {
		maxHosts = scanDefaultMaxHosts
	}
	hosts, err := scan.ParseTargets(req.Targets, maxHosts)
	if err != nil {
		sendFail(err.Error())
		return
	}

	concurrency := int(req.Concurrency)
	if concurrency <= 0 {
		concurrency = scanDefaultConcurrency
	}
	timeoutMs := int(req.TimeoutMs)
	if timeoutMs <= 0 {
		timeoutMs = scanDefaultTimeoutMs
	}
	maxDurSec := int(req.MaxDurationSec)
	if maxDurSec <= 0 {
		if mode == scan.ModeFull {
			maxDurSec = scanFullMaxDuration
		} else {
			maxDurSec = scanDefaultMaxDuration
		}
	}

	method := req.Method
	if method == "" {
		method = scan.MethodAuto
	}
	doDiscover := req.Discover != 2 // 0 default on, 1 on, 2 off

	timeout := time.Duration(timeoutMs) * time.Millisecond
	checker, err := scan.NewPortChecker(method, timeout)
	if err != nil {
		sendFail(err.Error())
		return
	}
	defer checker.Close()

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(maxDurSec)*time.Second)
	defer cancel()

	var (
		discoverMs   int64
		hostsAlive   int
		scanHosts    = hosts
		methodUsed   = checker.Method()
		discoverMode string
		icmpAlive    int
		tcpNewAlive  int
		fallback     bool
		warnings     []string
	)

	if doDiscover && len(hosts) > 0 {
		n.sendScanProg(taskID, "discovering", map[string]interface{}{
			"hosts_total":  len(hosts),
			"method":       methodUsed,
			"probe_ports":  len(scan.DefaultProbePorts),
			"strategy":     "icmp+tcp",
			"stage":        "start",
			"icmp_done":    0,
			"icmp_total":   len(hosts),
			"icmp_alive":   0,
			"alive_n":      0,
			"tcp_probes":   0,
		})
		// Throttle SCANPROG: at most ~2/s so we don't flood the wire.
		var progMu sync.Mutex
		var lastProg time.Time
		onProg := func(p scan.DiscoverProgress) {
			progMu.Lock()
			now := time.Now()
			// Always emit terminal stage markers; throttle intermediate ticks.
			force := p.Stage == "icmp_done" || p.Stage == "tcp_done"
			if !force && !lastProg.IsZero() && now.Sub(lastProg) < 500*time.Millisecond {
				progMu.Unlock()
				return
			}
			lastProg = now
			progMu.Unlock()
			n.sendScanProg(taskID, "discovering", map[string]interface{}{
				"hosts_total": p.HostsTotal,
				"stage":       p.Stage,
				"icmp_done":   p.ICMPDone,
				"icmp_total":  p.ICMPTotal,
				"icmp_alive":  p.ICMPAlive,
				"alive_n":     p.AliveN,
				"tcp_probes":  p.TCPProbes,
				"method":      p.Method,
				"probe_ports": len(scan.DefaultProbePorts),
				"strategy":    "icmp+tcp",
			})
		}
		disc, err := scan.Discover(ctx, scan.DiscoverConfig{
			Hosts:         hosts,
			ProbePorts:    scan.DefaultProbePorts,
			Concurrency:   concurrency,
			Timeout:       scan.DiscoverTimeout(timeout),
			Checker:       checker,
			OnProgress:    onProg,
			ProgressEvery: 16,
		})
		if err != nil {
			sendFail(err.Error())
			return
		}
		discoverMs = disc.DurationMs
		hostsAlive = len(disc.Alive)
		scanHosts = disc.Alive
		discoverMode = disc.Mode
		icmpAlive = disc.ICMPAlive
		tcpNewAlive = disc.TCPNewAlive
		if disc.Method != "" {
			methodUsed = disc.Method
		}
		// Zero alive → fallback full target list (avoid silent empty sweep).
		if len(scanHosts) == 0 && len(hosts) > 0 {
			fallback = true
			scanHosts = hosts
			warnings = append(warnings,
				"discover found 0 alive hosts (ICMP+TCP probes); fell back to scanning all targets")
			slog.Info("scan discover fallback", "task", taskID, "hosts", len(hosts))
		}
		n.sendScanProg(taskID, "scanning", map[string]interface{}{
			"hosts_total":   len(hosts),
			"hosts_alive":   hostsAlive,
			"ports":         len(ports),
			"mode":          mode,
			"method":        methodUsed,
			"discover_ms":   discoverMs,
			"discover_mode": discoverMode,
			"icmp_alive":    icmpAlive,
			"tcp_new_alive": tcpNewAlive,
			"fallback":      fallback,
		})
	} else {
		hostsAlive = len(hosts)
		n.sendScanProg(taskID, "scanning", map[string]interface{}{
			"hosts_total": len(hosts),
			"hosts_alive": hostsAlive,
			"ports":       len(ports),
			"mode":        mode,
			"method":      methodUsed,
			"discover":    false,
		})
	}

	// Cap total dials for non-full modes.
	maxDials := 0
	if mode != scan.ModeFull {
		maxDials = len(scanHosts) * len(ports)
	}

	var scanRes *scan.Result
	if len(scanHosts) == 0 {
		scanRes = &scan.Result{
			Open: nil,
			Stats: scan.Stats{
				HostsTotal: len(hosts),
				HostsAlive: 0,
				Method:     methodUsed,
			},
		}
	} else {
		var err error
		scanRes, err = scan.Run(ctx, scan.Config{
			Hosts:       scanHosts,
			Ports:       ports,
			Concurrency: concurrency,
			Timeout:     timeout,
			MaxDuration: time.Duration(maxDurSec) * time.Second,
			MaxDials:    maxDials,
			Checker:     checker,
		}, nil)
		if err != nil {
			sendFail(err.Error())
			return
		}
	}

	doFP := req.Fingerprint != 0
	var findings []fingerprint.Finding
	if doFP && len(scanRes.Open) > 0 {
		n.sendScanProg(taskID, "fingerprinting", map[string]interface{}{
			"open": scanRes.Stats.Open,
		})
		open := make([]struct {
			IP   string
			Port uint16
		}, len(scanRes.Open))
		for i, o := range scanRes.Open {
			open[i].IP = o.IP
			open[i].Port = o.Port
		}
		findings = fingerprint.ProbeOpen(ctx, open, fingerprint.Config{
			Timeout:     time.Duration(timeoutMs+400) * time.Millisecond,
			Concurrency: min(concurrency, 40),
		})
	} else {
		for _, o := range scanRes.Open {
			findings = append(findings, fingerprint.Finding{
				IP: o.IP, Port: o.Port, Proto: o.Proto, State: o.State,
			})
		}
	}

	fingerprint.AttachRefs(findings)

	// After fallback, hosts_alive is pre-fallback count; also report scan set size.
	stats := map[string]interface{}{
		"hosts_total":     len(hosts),
		"hosts_alive":     hostsAlive,
		"hosts_scanned":   len(scanHosts),
		"hosts_with_open": scanRes.Stats.HostsWithOpen,
		"ports_tried":     scanRes.Stats.PortsTried,
		"open":            scanRes.Stats.Open,
		"duration_ms":     time.Since(start).Milliseconds(),
		"discover_ms":     discoverMs,
		"discover_mode":   discoverMode,
		"icmp_alive":      icmpAlive,
		"tcp_new_alive":   tcpNewAlive,
		"fallback":        fallback,
		"method":          methodUsed,
	}
	result := map[string]interface{}{
		"mode":  mode,
		"stats": stats,
		"open":  findings,
		"summary": map[string]interface{}{
			"interesting": fingerprint.BuildInteresting(findings),
		},
	}
	if len(warnings) > 0 {
		result["warnings"] = warnings
	}
	payload, err := json.Marshal(result)
	if err != nil {
		sendFail("marshal result: " + err.Error())
		return
	}
	n.sendScanRes(taskID, true, "", payload)
}

func (n *Node) sendScanProg(taskID, phase string, payload map[string]interface{}) {
	var raw []byte
	if payload != nil {
		raw, _ = json.Marshal(payload)
	}
	header := &protocol.Header{
		Version:     1,
		Sender:      n.UUID,
		Accepter:    protocol.ControllerUUID,
		MessageType: protocol.SCANPROG,
		RouteLen:    uint32(len(protocol.NoRoute)),
		Route:       protocol.NoRoute,
	}
	prog := &protocol.ScanProg{
		TaskIDLen:  uint16(len(taskID)),
		TaskID:     taskID,
		PhaseLen:   uint16(len(phase)),
		Phase:      phase,
		PayloadLen: uint32(len(raw)),
		Payload:    raw,
	}
	if err := n.sendToParent(header, prog); err != nil {
		slog.Warn("send scan progress failed", "task", taskID, "error", err)
	}
}

func (n *Node) sendScanRes(taskID string, ok bool, errMsg string, payload []byte) {
	header := &protocol.Header{
		Version:     1,
		Sender:      n.UUID,
		Accepter:    protocol.ControllerUUID,
		MessageType: protocol.SCANRES,
		RouteLen:    uint32(len(protocol.NoRoute)),
		Route:       protocol.NoRoute,
	}
	res := &protocol.ScanRes{
		TaskIDLen:  uint16(len(taskID)),
		TaskID:     taskID,
		ErrorLen:   uint16(len(errMsg)),
		Error:      errMsg,
		PayloadLen: uint32(len(payload)),
		Payload:    payload,
	}
	if ok {
		res.OK = 1
	}
	if err := n.sendToParent(header, res); err != nil {
		slog.Error("send scan result failed", "task", taskID, "error", err)
	}
}
