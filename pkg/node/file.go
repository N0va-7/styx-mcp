package node

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"styx-mcp/pkg/protocol"
)

const maxDownloadBytes = 32 << 20 // 32 MiB single-slice cap
const maxUploadBytes = 32 << 20   // 32 MiB single-slice cap

// handleFileStat prepares for an incoming file upload.
func (n *Node) handleFileStat(req *protocol.FileStatReq) {
	safeName, err := sanitizeUploadPath(req.Filename)
	if err != nil {
		slog.Error("invalid upload filename", "filename", req.Filename, "error", err)
		n.pendingFile.filename = ""
		return
	}
	if req.FileSize > maxUploadBytes {
		slog.Error("upload too large", "filename", safeName, "size", req.FileSize, "max", maxUploadBytes)
		n.pendingFile.filename = ""
		return
	}

	n.pendingFile.filename = safeName
	n.pendingFile.fileSize = req.FileSize
	n.pendingFile.sliceNum = req.SliceNum

	slog.Info("file upload prepared", "filename", safeName, "size", req.FileSize, "slices", req.SliceNum)
}

// handleFileData writes an incoming file slice to disk.
func (n *Node) handleFileData(req *protocol.FileData) {
	filename := n.pendingFile.filename
	if filename == "" {
		slog.Error("no pending file upload")
		return
	}
	if uint64(len(req.Data)) > maxUploadBytes {
		slog.Error("upload chunk too large", "filename", filename, "bytes", len(req.Data))
		n.pendingFile.filename = ""
		return
	}

	dir := filepath.Dir(filename)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			slog.Error("create directory failed", "dir", dir, "error", err)
			n.pendingFile.filename = ""
			return
		}
	}

	if err := os.WriteFile(filename, req.Data, 0o644); err != nil {
		slog.Error("write file failed", "filename", filename, "error", err)
		n.pendingFile.filename = ""
		return
	}

	slog.Info("file received", "filename", filename, "bytes", len(req.Data))
	n.pendingFile.filename = ""
}

// handleFileDownReq reads a remote file and streams it to the controller.
func (n *Node) handleFileDownReq(req *protocol.FileDownReq) {
	go n.runFileDownload(req)
}

func (n *Node) runFileDownload(req *protocol.FileDownReq) {
	sendRes := func(ok bool) {
		header := &protocol.Header{
			Version:     1,
			Sender:      n.UUID,
			Accepter:    protocol.ControllerUUID,
			MessageType: protocol.FILEDOWNRES,
			RouteLen:    uint32(len(protocol.NoRoute)),
			Route:       protocol.NoRoute,
		}
		res := &protocol.FileDownRes{OK: 0}
		if ok {
			res.OK = 1
		}
		if err := n.sendToParent(header, res); err != nil {
			slog.Error("send filedown res failed", "error", err)
		}
	}

	path := strings.TrimSpace(req.FilePath)
	if path == "" || strings.ContainsRune(path, 0) {
		sendRes(false)
		return
	}
	path = filepath.Clean(path)

	data, err := os.ReadFile(path)
	if err != nil {
		slog.Warn("download read failed", "path", path, "error", err)
		sendRes(false)
		return
	}
	if len(data) > maxDownloadBytes {
		slog.Warn("download too large", "path", path, "size", len(data))
		sendRes(false)
		return
	}

	sendRes(true)

	statHeader := &protocol.Header{
		Version:     1,
		Sender:      n.UUID,
		Accepter:    protocol.ControllerUUID,
		MessageType: protocol.FILESTATREQ,
		RouteLen:    uint32(len(protocol.NoRoute)),
		Route:       protocol.NoRoute,
	}
	stat := &protocol.FileStatReq{
		FilenameLen: uint32(len(path)),
		Filename:    path,
		FileSize:    uint64(len(data)),
		SliceNum:    1,
	}
	if err := n.sendToParent(statHeader, stat); err != nil {
		slog.Error("send download filestat failed", "error", err)
		return
	}

	dataHeader := &protocol.Header{
		Version:     1,
		Sender:      n.UUID,
		Accepter:    protocol.ControllerUUID,
		MessageType: protocol.FILEDATA,
		RouteLen:    uint32(len(protocol.NoRoute)),
		Route:       protocol.NoRoute,
	}
	chunk := &protocol.FileData{DataLen: uint64(len(data)), Data: data}
	if err := n.sendToParent(dataHeader, chunk); err != nil {
		slog.Error("send download filedata failed", "error", err)
		return
	}
	slog.Info("file download sent", "path", path, "bytes", len(data))
}

// sanitizeUploadPath cleans a destination path and rejects traversal.
// Absolute paths are allowed (agents often write to /tmp/…); ".." components are not.
func sanitizeUploadPath(filename string) (string, error) {
	if filename == "" {
		return "", fmt.Errorf("empty filename")
	}
	if strings.ContainsRune(filename, 0) {
		return "", fmt.Errorf("invalid null byte in filename")
	}

	cleaned := filepath.Clean(filename)
	if cleaned == "" || cleaned == "." {
		return "", fmt.Errorf("empty filename")
	}

	// After Clean, any ".." that remains means the path walks above the start.
	// Reject pure ".." and any remaining ".." segment.
	if cleaned == ".." {
		return "", fmt.Errorf("path traversal detected")
	}
	for _, part := range strings.Split(cleaned, string(filepath.Separator)) {
		if part == ".." {
			return "", fmt.Errorf("path traversal detected")
		}
	}

	return cleaned, nil
}
