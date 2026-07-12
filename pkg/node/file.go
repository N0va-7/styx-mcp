package node

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"styx-mcp/pkg/protocol"
)

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

// sanitizeUploadPath rejects absolute paths and path traversal attempts.
// It returns a cleaned relative path that is safe to write under the current
// working directory (or a configured upload directory).
func sanitizeUploadPath(filename string) (string, error) {
	if filename == "" {
		return "", fmt.Errorf("empty filename")
	}

	if filepath.IsAbs(filename) {
		return "", fmt.Errorf("absolute paths are not allowed")
	}

	cleaned := filepath.Clean(filename)

	// Reject any component that escapes the base directory.
	parts := strings.Split(cleaned, string(filepath.Separator))
	for _, part := range parts {
		if part == ".." {
			return "", fmt.Errorf("path traversal detected")
		}
	}

	return cleaned, nil
}
