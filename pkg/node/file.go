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

// handleFileStat prepares for an incoming file upload.
func (n *Node) handleFileStat(req *protocol.FileStatReq) {
	safeName, err := sanitizeUploadPath(req.Filename)
	if err != nil {
		slog.Error("invalid upload filename", "filename", req.Filename, "error", err)
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

	dir := filepath.Dir(filename)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			slog.Error("create directory failed", "dir", dir, "error", err)
			return
		}
	}

	if err := os.WriteFile(filename, req.Data, 0o644); err != nil {
		slog.Error("write file failed", "filename", filename, "error", err)
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
			Accepter:    protocol.ADMIN_UUID,
			MessageType: protocol.FILEDOWNRES,
			RouteLen:    uint32(len(protocol.TEMP_ROUTE)),
			Route:       protocol.TEMP_ROUTE,
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
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.FILESTATREQ,
		RouteLen:    uint32(len(protocol.TEMP_ROUTE)),
		Route:       protocol.TEMP_ROUTE,
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
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.FILEDATA,
		RouteLen:    uint32(len(protocol.TEMP_ROUTE)),
		Route:       protocol.TEMP_ROUTE,
	}
	chunk := &protocol.FileData{DataLen: uint64(len(data)), Data: data}
	if err := n.sendToParent(dataHeader, chunk); err != nil {
		slog.Error("send download filedata failed", "error", err)
		return
	}
	slog.Info("file download sent", "path", path, "bytes", len(data))
}

// sanitizeUploadPath rejects absolute paths and path traversal attempts.
func sanitizeUploadPath(filename string) (string, error) {
	if filename == "" {
		return "", fmt.Errorf("empty filename")
	}

	if filepath.IsAbs(filename) {
		return "", fmt.Errorf("absolute paths are not allowed")
	}

	cleaned := filepath.Clean(filename)

	parts := strings.Split(cleaned, string(filepath.Separator))
	for _, part := range parts {
		if part == ".." {
			return "", fmt.Errorf("path traversal detected")
		}
	}

	return cleaned, nil
}
