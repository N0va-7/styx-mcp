package node

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"styx-mcp/pkg/protocol"
)

const maxDownloadBytes = 64 << 20 // 64 MiB total cap
const maxUploadBytes = 64 << 20   // 64 MiB total cap
const fileChunkSize = 512 << 10   // 512 KiB wire chunks

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
	n.pendingFile.nextSlice = 0
	n.pendingFile.received = 0

	// Truncate / create destination for multi-slice write.
	dir := filepath.Dir(safeName)
	if dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			slog.Error("create directory failed", "dir", dir, "error", err)
			n.pendingFile.filename = ""
			return
		}
	}
	if err := os.WriteFile(safeName, nil, 0o644); err != nil {
		slog.Error("truncate upload target failed", "filename", safeName, "error", err)
		n.pendingFile.filename = ""
		return
	}

	slog.Info("file upload prepared", "filename", safeName, "size", req.FileSize, "slices", req.SliceNum)
}

// handleFileData writes an incoming file slice to disk (append in order).
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

	// Default for legacy single-slice messages (SliceTotal 0 treated as 1).
	total := req.SliceTotal
	if total == 0 {
		total = 1
	}
	if req.SliceIndex != n.pendingFile.nextSlice {
		slog.Error("out-of-order slice", "filename", filename, "got", req.SliceIndex, "want", n.pendingFile.nextSlice)
		n.pendingFile.filename = ""
		return
	}

	f, err := os.OpenFile(filename, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		slog.Error("open upload target failed", "filename", filename, "error", err)
		n.pendingFile.filename = ""
		return
	}
	if _, err := f.Write(req.Data); err != nil {
		f.Close()
		slog.Error("write file failed", "filename", filename, "error", err)
		n.pendingFile.filename = ""
		return
	}
	f.Close()

	n.pendingFile.nextSlice++
	n.pendingFile.received += uint64(len(req.Data))

	if uint32(n.pendingFile.nextSlice) >= total {
		slog.Info("file received", "filename", filename, "bytes", n.pendingFile.received, "slices", total)
		n.pendingFile.filename = ""
	}
}

// handleFileDownReq reads a remote file and streams it to the controller in chunks.
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

	total := len(data)
	slices := total / fileChunkSize
	if total%fileChunkSize != 0 || total == 0 {
		slices++
	}
	if total == 0 {
		slices = 1
	}

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
		FileSize:    uint64(total),
		SliceNum:    uint64(slices),
	}
	if err := n.sendToParent(statHeader, stat); err != nil {
		slog.Error("send download filestat failed", "error", err)
		return
	}

	for i := 0; i < slices; i++ {
		start := i * fileChunkSize
		end := start + fileChunkSize
		if end > total {
			end = total
		}
		chunk := data[start:end]
		dataHeader := &protocol.Header{
			Version:     1,
			Sender:      n.UUID,
			Accepter:    protocol.ControllerUUID,
			MessageType: protocol.FILEDATA,
			RouteLen:    uint32(len(protocol.NoRoute)),
			Route:       protocol.NoRoute,
		}
		msg := &protocol.FileData{
			SliceIndex: uint32(i),
			SliceTotal: uint32(slices),
			DataLen:    uint64(len(chunk)),
			Data:       chunk,
		}
		if err := n.sendToParent(dataHeader, msg); err != nil {
			slog.Error("send download filedata failed", "error", err, "slice", i)
			return
		}
	}
	slog.Info("file download sent", "path", path, "bytes", total, "slices", slices)
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
