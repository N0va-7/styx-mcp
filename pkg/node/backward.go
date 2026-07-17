package node

import (
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"

	"styx-mcp/internal/utils"
	"styx-mcp/pkg/protocol"
)

// BackwardManager tracks active reverse-forward connections on a node.
type BackwardManager struct {
	n       *Node
	seqConn map[uint64]net.Conn
	mu      sync.RWMutex
	seqGen  uint64
}

// NewBackwardManager creates a manager for reverse port forwards.
func NewBackwardManager(n *Node) *BackwardManager {
	return &BackwardManager{
		n:       n,
		seqConn: make(map[uint64]net.Conn),
	}
}

// closeAll tears down all reverse-forward connections after upstream loss (no resume).
func (bm *BackwardManager) closeAll() {
	bm.mu.Lock()
	for seq, conn := range bm.seqConn {
		conn.Close()
		delete(bm.seqConn, seq)
	}
	bm.mu.Unlock()
}

func (bm *BackwardManager) handleBackwardStart(req *protocol.BackwardStart) {
	targetAddr, _, err := utils.CheckIPPort(req.TargetAddr)
	if err != nil {
		slog.Error("invalid backward target address", "error", err)
		bm.sendBackwardReady(req.Seq, false)
		return
	}

	conn, err := net.Dial("tcp", targetAddr)
	if err != nil {
		slog.Error("backward target connect failed", "target", targetAddr, "error", err)
		bm.sendBackwardReady(req.Seq, false)
		return
	}

	bm.mu.Lock()
	bm.seqConn[req.Seq] = conn
	bm.mu.Unlock()

	bm.sendBackwardReady(req.Seq, true)

	// Forward target -> upstream.
	go func(seq uint64, c net.Conn) {
		defer c.Close()
		buf := make([]byte, 32*1024)
		for {
			nr, err := c.Read(buf)
			if nr > 0 {
				bm.sendBackwardData(seq, buf[:nr])
			}
			if err != nil {
				if err != io.EOF {
					slog.Warn("backward target read failed", "seq", seq, "error", err)
				}
				bm.sendBackwardFin(seq)
				bm.removeConn(seq)
				return
			}
		}
	}(req.Seq, conn)
}

func (bm *BackwardManager) handleBackwardData(req *protocol.BackwardData) {
	bm.mu.RLock()
	conn, ok := bm.seqConn[req.Seq]
	bm.mu.RUnlock()
	if !ok {
		slog.Warn("backward data for unknown seq", "seq", req.Seq)
		return
	}

	if _, err := conn.Write(req.Data); err != nil {
		slog.Warn("backward target write failed", "seq", req.Seq, "error", err)
		conn.Close()
		bm.removeConn(req.Seq)
	}
}

func (bm *BackwardManager) handleBackwardFin(req *protocol.BackWardFin) {
	bm.mu.Lock()
	conn, ok := bm.seqConn[req.Seq]
	if ok {
		conn.Close()
		delete(bm.seqConn, req.Seq)
	}
	bm.mu.Unlock()
}

func (bm *BackwardManager) removeConn(seq uint64) {
	bm.mu.Lock()
	defer bm.mu.Unlock()
	if conn, ok := bm.seqConn[seq]; ok {
		conn.Close()
		delete(bm.seqConn, seq)
	}
}

func (bm *BackwardManager) nextSeq() uint64 {
	return atomic.AddUint64(&bm.seqGen, 1)
}

func (bm *BackwardManager) sendBackwardReady(seq uint64, ok bool) {
	res := &protocol.BackwardReady{Seq: seq, OK: 0}
	if ok {
		res.OK = 1
	}
	header := &protocol.Header{
		Version:     1,
		Sender:      bm.n.UUID,
		Accepter:    protocol.ControllerUUID,
		MessageType: protocol.BACKWARDREADY,
		RouteLen:    uint32(len(protocol.NoRoute)),
		Route:       protocol.NoRoute,
	}
	bm.sendToUpstream(header, res)
}

func (bm *BackwardManager) sendBackwardData(seq uint64, data []byte) {
	header := &protocol.Header{
		Version:     1,
		Sender:      bm.n.UUID,
		Accepter:    protocol.ControllerUUID,
		MessageType: protocol.BACKWARDDATA,
		RouteLen:    uint32(len(protocol.NoRoute)),
		Route:       protocol.NoRoute,
	}
	msg := &protocol.BackwardData{Seq: seq, DataLen: uint64(len(data)), Data: data}
	bm.sendToUpstream(header, msg)
}

func (bm *BackwardManager) sendBackwardFin(seq uint64) {
	header := &protocol.Header{
		Version:     1,
		Sender:      bm.n.UUID,
		Accepter:    protocol.ControllerUUID,
		MessageType: protocol.BACKWARDFIN,
		RouteLen:    uint32(len(protocol.NoRoute)),
		Route:       protocol.NoRoute,
	}
	bm.sendToUpstream(header, &protocol.BackWardFin{Seq: seq})
}

func (bm *BackwardManager) sendToUpstream(header *protocol.Header, payload interface{}) {
	sMessage := protocol.NewUpMsg(bm.n.ParentConn, bm.n.Options.Secret, bm.n.UUID)
	if err := protocol.ConstructMessage(sMessage, header, payload, false); err != nil {
		slog.Error("send backward message failed", "error", err)
		return
	}
	if err := sMessage.SendMessage(); err != nil {
		slog.Error("send backward message failed", "error", err)
	}
}
