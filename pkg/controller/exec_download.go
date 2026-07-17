package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"log/slog"
	"os"
	"path/filepath"

	"styx-mcp/pkg/protocol"
)

type pendingDownload struct {
	taskID     string
	localPath  string
	fileSize   uint64
	sliceTotal uint32
	nextSlice  uint32
	received   uint64
	hash       hash.Hash
	file       *os.File
}

// StartExec asks a node to run a command asynchronously (result via EXECRES).
func (c *Controller) StartExec(nodeUUID, taskID, command, workdir string, timeoutSec uint32) error {
	header := &protocol.Header{
		Version:     1,
		Sender:      protocol.ControllerUUID,
		Accepter:    nodeUUID,
		MessageType: protocol.EXECREQ,
	}
	req := &protocol.ExecReq{
		TaskIDLen:  uint16(len(taskID)),
		TaskID:     taskID,
		TimeoutSec: timeoutSec,
		WorkdirLen: uint16(len(workdir)),
		Workdir:    workdir,
		CmdLen:     uint32(len(command)),
		Cmd:        command,
	}
	return c.SendToNode(nodeUUID, header, req)
}

func (c *Controller) handleExecRes(res *protocol.ExecRes) {
	c.TaskManager.SetPhase(res.TaskID, "done")
	c.TaskManager.SetResult(res.TaskID, map[string]interface{}{
		"exit_code":   res.ExitCode,
		"stdout":      string(res.Stdout),
		"stderr":      string(res.Stderr),
		"truncated":   res.Truncated == 1,
		"timed_out":   res.TimedOut == 1,
		"duration_ms": res.DurationMs,
	})
}

// StartDownload requests a file from the node to a local path.
func (c *Controller) StartDownload(nodeUUID, taskID, remotePath, localPath string) error {
	c.downloadsMu.Lock()
	c.pendingDownloads[nodeUUID] = &pendingDownload{
		taskID:    taskID,
		localPath: localPath,
		hash:      sha256.New(),
	}
	c.downloadsMu.Unlock()

	header := &protocol.Header{
		Version:     1,
		Sender:      protocol.ControllerUUID,
		Accepter:    nodeUUID,
		MessageType: protocol.FILEDOWNREQ,
	}
	req := &protocol.FileDownReq{
		FilePathLen: uint32(len(remotePath)),
		FilePath:    remotePath,
		FilenameLen: uint32(len(filepath.Base(localPath))),
		Filename:    filepath.Base(localPath),
	}
	if err := c.SendToNode(nodeUUID, header, req); err != nil {
		c.clearDownload(nodeUUID)
		return err
	}
	return nil
}

func (c *Controller) handleFileDownRes(uuid string, res *protocol.FileDownRes) {
	if res.OK == 1 {
		c.TaskManager.SetPhase(c.downloadTaskID(uuid), "receiving")
		return
	}
	c.downloadsMu.Lock()
	pd, ok := c.pendingDownloads[uuid]
	c.downloadsMu.Unlock()
	if !ok {
		return
	}
	c.TaskManager.SetPhase(pd.taskID, "rejected")
	c.TaskManager.SetError(pd.taskID, fmt.Errorf("agent rejected download"))
	c.clearDownload(uuid)
}

func (c *Controller) downloadTaskID(uuid string) string {
	c.downloadsMu.Lock()
	defer c.downloadsMu.Unlock()
	if pd, ok := c.pendingDownloads[uuid]; ok {
		return pd.taskID
	}
	return ""
}

func (c *Controller) handleDownloadFileStat(uuid string, req *protocol.FileStatReq) {
	c.downloadsMu.Lock()
	pd, ok := c.pendingDownloads[uuid]
	if ok {
		pd.fileSize = req.FileSize
		if req.SliceNum == 0 {
			pd.sliceTotal = 1
		} else {
			pd.sliceTotal = uint32(req.SliceNum)
		}
		pd.nextSlice = 0
		pd.received = 0
	}
	c.downloadsMu.Unlock()
	if !ok {
		slog.Warn("filestat for unknown download", "uuid", uuid)
		return
	}
	c.TaskManager.SetPhase(pd.taskID, "receiving")
}

func (c *Controller) handleDownloadFileData(uuid string, data *protocol.FileData) {
	c.downloadsMu.Lock()
	pd, ok := c.pendingDownloads[uuid]
	if !ok {
		c.downloadsMu.Unlock()
		slog.Warn("filedata for unknown download", "uuid", uuid)
		return
	}

	total := data.SliceTotal
	if total == 0 {
		total = 1
	}
	if pd.sliceTotal == 0 {
		pd.sliceTotal = total
	}
	if data.SliceIndex != pd.nextSlice {
		taskID := pd.taskID
		c.downloadsMu.Unlock()
		c.TaskManager.SetPhase(taskID, "slice-error")
		c.TaskManager.SetError(taskID, fmt.Errorf("out-of-order slice: got %d want %d", data.SliceIndex, pd.nextSlice))
		c.clearDownload(uuid)
		return
	}

	if pd.file == nil {
		dir := filepath.Dir(pd.localPath)
		if dir != "" && dir != "." {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				taskID := pd.taskID
				c.downloadsMu.Unlock()
				c.TaskManager.SetError(taskID, err)
				c.clearDownload(uuid)
				return
			}
		}
		f, err := os.OpenFile(pd.localPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
		if err != nil {
			taskID := pd.taskID
			c.downloadsMu.Unlock()
			c.TaskManager.SetError(taskID, err)
			c.clearDownload(uuid)
			return
		}
		pd.file = f
		if pd.hash == nil {
			pd.hash = sha256.New()
		}
	}

	if _, err := pd.file.Write(data.Data); err != nil {
		taskID := pd.taskID
		_ = pd.file.Close()
		pd.file = nil
		c.downloadsMu.Unlock()
		c.TaskManager.SetError(taskID, err)
		c.clearDownload(uuid)
		return
	}
	if pd.hash != nil {
		_, _ = pd.hash.Write(data.Data)
	}
	pd.nextSlice++
	pd.received += uint64(len(data.Data))
	done := pd.nextSlice >= pd.sliceTotal
	taskID := pd.taskID
	bytes := pd.received
	localPath := pd.localPath
	var sum []byte
	if done && pd.hash != nil {
		sum = pd.hash.Sum(nil)
	}
	if done {
		if pd.file != nil {
			_ = pd.file.Close()
			pd.file = nil
		}
	}
	c.downloadsMu.Unlock()

	if !done {
		return
	}

	result := map[string]interface{}{
		"local_path": localPath,
		"bytes":      bytes,
		"slices":     total,
	}
	if sum != nil {
		result["sha256"] = hex.EncodeToString(sum)
	}
	c.TaskManager.SetPhase(taskID, "done")
	c.TaskManager.SetResult(taskID, result)
	c.clearDownload(uuid)
	slog.Info("download complete", "task", taskID, "bytes", bytes, "slices", total)
}

func (c *Controller) clearDownload(uuid string) {
	c.downloadsMu.Lock()
	if pd, ok := c.pendingDownloads[uuid]; ok && pd.file != nil {
		_ = pd.file.Close()
	}
	delete(c.pendingDownloads, uuid)
	c.downloadsMu.Unlock()
}
