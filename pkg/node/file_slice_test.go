package node

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"styx-mcp/pkg/protocol"
)

func TestMultiSliceUploadReassembly(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "out.bin")

	// Three slices that rebuild to a known payload.
	part0 := bytes.Repeat([]byte("A"), 100)
	part1 := bytes.Repeat([]byte("B"), 100)
	part2 := bytes.Repeat([]byte("C"), 50)
	want := append(append(append([]byte{}, part0...), part1...), part2...)

	n := NewNode(&Options{Secret: "t"})
	n.handleFileStat(&protocol.FileStatReq{
		FilenameLen: uint32(len(dest)),
		Filename:    dest,
		FileSize:    uint64(len(want)),
		SliceNum:    3,
	})
	if n.pendingFile.filename == "" {
		t.Fatal("pending file not prepared")
	}

	parts := [][]byte{part0, part1, part2}
	for i, p := range parts {
		n.handleFileData(&protocol.FileData{
			SliceIndex: uint32(i),
			SliceTotal: 3,
			DataLen:    uint64(len(p)),
			Data:       p,
		})
	}
	if n.pendingFile.filename != "" {
		t.Fatal("pending file should clear after last slice")
	}

	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("reassembled len=%d want=%d mismatch", len(got), len(want))
	}
}

func TestMultiSliceOutOfOrderRejected(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "bad.bin")

	n := NewNode(&Options{Secret: "t"})
	n.handleFileStat(&protocol.FileStatReq{
		FilenameLen: uint32(len(dest)),
		Filename:    dest,
		FileSize:    2,
		SliceNum:    2,
	})
	// Skip index 0 — should abort pending upload.
	n.handleFileData(&protocol.FileData{
		SliceIndex: 1,
		SliceTotal: 2,
		DataLen:    1,
		Data:       []byte{0x01},
	})
	if n.pendingFile.filename != "" {
		t.Fatal("pending should clear on out-of-order")
	}
}

func TestLegacySingleSliceZeroTotal(t *testing.T) {
	dir := t.TempDir()
	dest := filepath.Join(dir, "legacy.bin")
	payload := []byte("hello-legacy")

	n := NewNode(&Options{Secret: "t"})
	n.handleFileStat(&protocol.FileStatReq{
		FilenameLen: uint32(len(dest)),
		Filename:    dest,
		FileSize:    uint64(len(payload)),
		SliceNum:    1,
	})
	n.handleFileData(&protocol.FileData{
		// SliceTotal 0 → treated as 1 for backward compatibility
		SliceIndex: 0,
		SliceTotal: 0,
		DataLen:    uint64(len(payload)),
		Data:       payload,
	})
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, payload) {
		t.Fatalf("got %q want %q", got, payload)
	}
}
