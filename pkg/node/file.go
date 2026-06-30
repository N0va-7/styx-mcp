package node

import (
	"log/slog"
	"os"
	"path/filepath"

	"mcp-stowaway/pkg/protocol"
)

var pendingFile struct {
	filename string
	fileSize uint64
	sliceNum uint64
}

// handleFileStat prepares for an incoming file upload.
func (n *Node) handleFileStat(req *protocol.FileStatReq) {
	pendingFile.filename = req.Filename
	pendingFile.fileSize = req.FileSize
	pendingFile.sliceNum = req.SliceNum

	slog.Info("file upload prepared", "filename", req.Filename, "size", req.FileSize, "slices", req.SliceNum)
}

// handleFileData writes an incoming file slice to disk.
func (n *Node) handleFileData(req *protocol.FileData) {
	filename := pendingFile.filename
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
	pendingFile.filename = ""
}
