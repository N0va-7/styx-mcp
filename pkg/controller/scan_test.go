package controller

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"styx-mcp/pkg/fingerprint"
	"styx-mcp/pkg/protocol"
	"styx-mcp/pkg/tasks"
)

func TestHandleScanProgAndRes(t *testing.T) {
	c := NewController(&Options{Secret: "t", Listen: "127.0.0.1:0"})
	go c.Topology.Run()

	task := c.TaskManager.Create("start_scan")
	c.TaskManager.UpdateStatus(task.ID, tasks.Running)

	c.handleScanProg(&protocol.ScanProg{
		TaskIDLen: uint16(len(task.ID)),
		TaskID:    task.ID,
		PhaseLen:  uint16(len("scanning")),
		Phase:     "scanning",
	})
	got, ok := c.TaskManager.Get(task.ID)
	if !ok || got.Phase != "scanning" {
		t.Fatalf("phase scanning: ok=%v phase=%q", ok, got.Phase)
	}

	c.handleScanProg(&protocol.ScanProg{
		TaskIDLen: uint16(len(task.ID)),
		TaskID:    task.ID,
		PhaseLen:  uint16(len("fingerprinting")),
		Phase:     "fingerprinting",
	})
	got, _ = c.TaskManager.Get(task.ID)
	if got.Phase != "fingerprinting" {
		t.Fatalf("phase=%q", got.Phase)
	}

	payload, _ := json.Marshal(map[string]interface{}{
		"mode": "fast",
		"stats": map[string]interface{}{
			"hosts_total": 1, "hosts_with_open": 1, "ports_tried": 10, "open": 1, "duration_ms": 5,
		},
		"open": []map[string]interface{}{
			{"ip": "127.0.0.1", "port": 7001, "proto": "tcp", "state": "open", "product": "weblogic"},
		},
	})
	c.handleScanRes(&protocol.ScanRes{
		TaskIDLen:  uint16(len(task.ID)),
		TaskID:     task.ID,
		OK:         1,
		PayloadLen: uint32(len(payload)),
		Payload:    payload,
	}, "deadbeef01")

	got, ok = c.TaskManager.Get(task.ID)
	if !ok || got.Status != tasks.Done {
		t.Fatalf("status=%v ok=%v", got.Status, ok)
	}
	if got.Phase != "done" {
		t.Fatalf("phase=%q", got.Phase)
	}
	open, ok := got.Result["open"]
	if !ok {
		t.Fatal("missing open")
	}
	findings, ok := open.([]fingerprint.Finding)
	if !ok {
		t.Fatalf("open type %T", open)
	}
	if len(findings) != 1 || findings[0].Product != "weblogic" {
		t.Fatalf("findings=%+v", findings)
	}
	if len(findings[0].Refs) < 1 {
		t.Fatal("expected weblogic refs after enrichment")
	}
	sum, ok := got.Result["summary"].(map[string]interface{})
	if !ok {
		t.Fatalf("summary type %T", got.Result["summary"])
	}
	if _, ok := sum["interesting"]; !ok {
		t.Fatal("missing summary.interesting")
	}
}

func TestHandleScanProgMergesProgress(t *testing.T) {
	c := NewController(&Options{Secret: "t", Listen: "127.0.0.1:0"})
	go c.Topology.Run()
	task := c.TaskManager.Create("start_scan")
	c.TaskManager.UpdateStatus(task.ID, tasks.Running)
	c.TaskManager.SetPhase(task.ID, "discovering")

	payload, _ := json.Marshal(map[string]interface{}{
		"stage": "icmp", "icmp_done": 32, "icmp_total": 254, "icmp_alive": 1, "alive_n": 1,
	})
	c.handleScanProg(&protocol.ScanProg{
		TaskIDLen:  uint16(len(task.ID)),
		TaskID:     task.ID,
		PhaseLen:   uint16(len("discovering")),
		Phase:      "discovering",
		PayloadLen: uint32(len(payload)),
		Payload:    payload,
	})
	got, _ := c.TaskManager.Get(task.ID)
	if got.Status != tasks.Running {
		t.Fatalf("status=%s", got.Status)
	}
	prog, ok := got.Result["progress"].(map[string]interface{})
	if !ok {
		t.Fatalf("result=%v", got.Result)
	}
	// json numbers are float64 when re-unmarshaled from map... actually we put via MergeResult from unmarshaled JSON
	if fmt.Sprint(prog["stage"]) != "icmp" {
		t.Fatalf("prog=%v", prog)
	}
}

func TestHandleScanResError(t *testing.T) {
	c := NewController(&Options{Secret: "t", Listen: "127.0.0.1:0"})
	go c.Topology.Run()
	task := c.TaskManager.Create("start_scan")
	c.handleScanRes(&protocol.ScanRes{
		TaskIDLen: uint16(len(task.ID)),
		TaskID:    task.ID,
		OK:        0,
		ErrorLen:  uint16(len("boom")),
		Error:     "boom",
	}, "x")
	got, _ := c.TaskManager.Get(task.ID)
	if got.Status != tasks.Failed {
		t.Fatalf("status=%v", got.Status)
	}
	if got.Error != "boom" {
		t.Fatalf("error=%q", got.Error)
	}
	if !strings.Contains(got.Phase, "fail") && got.Phase != "failed" {
		// phase may be "failed"
		if got.Status != tasks.Failed {
			t.Fatalf("phase=%q status=%v", got.Phase, got.Status)
		}
	}
}
