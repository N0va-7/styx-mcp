package controller

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"styx-mcp/pkg/protocol"
)

type pendingDownload struct {
	taskID    string
	localPath string
	fileSize  uint64
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
		return
	}
	c.downloadsMu.Lock()
	pd, ok := c.pendingDownloads[uuid]
	c.downloadsMu.Unlock()
	if !ok {
		return
	}
	c.TaskManager.SetError(pd.taskID, fmt.Errorf("agent rejected download"))
	c.clearDownload(uuid)
}

func (c *Controller) handleDownloadFileStat(uuid string, req *protocol.FileStatReq) {
	c.downloadsMu.Lock()
	pd, ok := c.pendingDownloads[uuid]
	if ok {
		pd.fileSize = req.FileSize
	}
	c.downloadsMu.Unlock()
	if !ok {
		slog.Warn("filestat for unknown download", "uuid", uuid)
	}
}

func (c *Controller) handleDownloadFileData(uuid string, data *protocol.FileData) {
	c.downloadsMu.Lock()
	pd, ok := c.pendingDownloads[uuid]
	c.downloadsMu.Unlock()
	if !ok {
		slog.Warn("filedata for unknown download", "uuid", uuid)
		return
	}

	dir := filepath.Dir(pd.localPath)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			c.TaskManager.SetError(pd.taskID, err)
			c.clearDownload(uuid)
			return
		}
	}
	if err := os.WriteFile(pd.localPath, data.Data, 0o644); err != nil {
		c.TaskManager.SetError(pd.taskID, err)
		c.clearDownload(uuid)
		return
	}

	sum := sha256.Sum256(data.Data)
	c.TaskManager.SetResult(pd.taskID, map[string]interface{}{
		"local_path": pd.localPath,
		"bytes":      len(data.Data),
		"sha256":     hex.EncodeToString(sum[:]),
	})
	c.clearDownload(uuid)
	slog.Info("download complete", "task", pd.taskID, "bytes", len(data.Data))
}

func (c *Controller) clearDownload(uuid string) {
	c.downloadsMu.Lock()
	delete(c.pendingDownloads, uuid)
	c.downloadsMu.Unlock()
}
