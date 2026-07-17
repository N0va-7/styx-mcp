package controller

import (
	"encoding/json"
	"fmt"
	"log/slog"

	"styx-mcp/pkg/fingerprint"
	"styx-mcp/pkg/protocol"
)

// StartScan sends SCANREQ to the agent for nodeUUID.
func (c *Controller) StartScan(nodeUUID string, req *protocol.ScanReq) error {
	header := &protocol.Header{
		Version:     1,
		Sender:      protocol.ControllerUUID,
		Accepter:    nodeUUID,
		MessageType: protocol.SCANREQ,
	}
	return c.SendToNode(nodeUUID, header, req)
}

func (c *Controller) handleScanProg(prog *protocol.ScanProg) {
	if prog == nil || prog.TaskID == "" {
		return
	}
	if prog.Phase != "" {
		c.TaskManager.SetPhase(prog.TaskID, prog.Phase)
	}
	// Progressive payload → task.result so get_task_status shows discover progress.
	if len(prog.Payload) == 0 {
		return
	}
	var partial map[string]interface{}
	if err := json.Unmarshal(prog.Payload, &partial); err != nil || partial == nil {
		return
	}
	// Nest under progress to avoid clobbering final open/summary keys later.
	c.TaskManager.MergeResult(prog.TaskID, map[string]interface{}{
		"progress": partial,
	})
}

func (c *Controller) handleScanRes(res *protocol.ScanRes, fromUUID string) {
	if res == nil || res.TaskID == "" {
		return
	}
	if res.OK != 1 {
		msg := res.Error
		if msg == "" {
			msg = "scan failed"
		}
		c.TaskManager.SetPhase(res.TaskID, "failed")
		c.TaskManager.SetError(res.TaskID, fmt.Errorf("%s", msg))
		return
	}

	var result map[string]interface{}
	if len(res.Payload) > 0 {
		if err := json.Unmarshal(res.Payload, &result); err != nil {
			c.TaskManager.SetPhase(res.TaskID, "result-error")
			c.TaskManager.SetError(res.TaskID, fmt.Errorf("invalid scan payload: %w", err))
			return
		}
	} else {
		result = map[string]interface{}{}
	}
	if id, ok := c.nodeIDByUUID(fromUUID); ok {
		result["via_node_id"] = id
	}

	// Controller-side ref enrichment (also safe if agent already attached).
	enrichScanRefs(result)

	c.TaskManager.SetPhase(res.TaskID, "done")
	c.TaskManager.SetResult(res.TaskID, result)
	slog.Info("scan complete", "task", res.TaskID, "from", fromUUID)
}

func (c *Controller) nodeIDByUUID(uuid string) (int, bool) {
	for _, e := range c.ListNodes() {
		if e.Node != nil && e.Node.UUID == uuid {
			return e.ID, true
		}
	}
	return -1, false
}

func enrichScanRefs(result map[string]interface{}) {
	rawOpen, ok := result["open"]
	if !ok {
		return
	}
	b, err := json.Marshal(rawOpen)
	if err != nil {
		return
	}
	var findings []fingerprint.Finding
	if err := json.Unmarshal(b, &findings); err != nil {
		return
	}
	fingerprint.AttachRefs(findings)
	result["open"] = findings
	result["summary"] = map[string]interface{}{
		"interesting": fingerprint.BuildInteresting(findings),
	}
}
