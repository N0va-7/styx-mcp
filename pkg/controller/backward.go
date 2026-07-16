package controller

import (
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"

	"styx-mcp/internal/utils"
	"styx-mcp/pkg/protocol"
)

// BackwardListener tracks a reverse-forward listener on the controller.
type BackwardListener struct {
	ctrl       *Controller
	nodeUUID   string
	targetAddr string
	listener   net.Listener
	seqConn    map[uint64]net.Conn
	mu         sync.RWMutex
	seqGen     uint64
	stopCh     chan struct{}
}

// NewBackwardListener creates a listener for reverse port forwarding.
func NewBackwardListener(ctrl *Controller, nodeUUID, localAddr, targetAddr string) (*BackwardListener, error) {
	addr, _, err := utils.CheckIPPort(localAddr)
	if err != nil {
		return nil, err
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}

	return &BackwardListener{
		ctrl:       ctrl,
		nodeUUID:   nodeUUID,
		targetAddr: targetAddr,
		listener:   listener,
		seqConn:    make(map[uint64]net.Conn),
		stopCh:     make(chan struct{}),
	}, nil
}

// Run accepts local connections and forwards them through the node.
func (bl *BackwardListener) Run() {
	defer bl.listener.Close()

	for {
		select {
		case <-bl.stopCh:
			return
		default:
		}

		conn, err := bl.listener.Accept()
		if err != nil {
			slog.Warn("backward accept failed", "error", err)
			continue
		}
		go bl.handleLocalConn(conn)
	}
}

// Stop closes the listener and active connections.
func (bl *BackwardListener) Stop() {
	close(bl.stopCh)
	bl.listener.Close()
	bl.mu.Lock()
	for _, conn := range bl.seqConn {
		conn.Close()
	}
	bl.mu.Unlock()
}

func (bl *BackwardListener) handleLocalConn(conn net.Conn) {
	defer conn.Close()

	seq := atomic.AddUint64(&bl.seqGen, 1)
	bl.mu.Lock()
	bl.seqConn[seq] = conn
	bl.mu.Unlock()

	// Tell the node to connect to the target.
	start := &protocol.BackwardStart{
		Seq:           seq,
		TargetAddrLen: uint16(len(bl.targetAddr)),
		TargetAddr:    bl.targetAddr,
	}
	header := &protocol.Header{
		Version:     1,
		Sender:      protocol.ControllerUUID,
		Accepter:    bl.nodeUUID,
		MessageType: protocol.BACKWARDSTART,
	}
	if err := bl.ctrl.SendToNode(bl.nodeUUID, header, start); err != nil {
		slog.Error("send backward start failed", "seq", seq, "error", err)
		bl.removeConn(seq)
		return
	}

	// Forward local -> node.
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-bl.stopCh:
			return
		default:
		}

		nr, err := conn.Read(buf)
		if nr > 0 {
			bl.sendBackwardData(seq, buf[:nr])
		}
		if err != nil {
			if err != io.EOF {
				slog.Warn("backward local read failed", "seq", seq, "error", err)
			}
			bl.sendBackwardFin(seq)
			bl.removeConn(seq)
			return
		}
	}
}

func (bl *BackwardListener) handleReady(seq uint64, ok bool) {
	if !ok {
		bl.removeConn(seq)
	}
}

func (bl *BackwardListener) handleData(seq uint64, data []byte) {
	bl.mu.RLock()
	conn, found := bl.seqConn[seq]
	bl.mu.RUnlock()
	if !found {
		slog.Warn("backward data for unknown seq", "seq", seq)
		return
	}

	if _, err := conn.Write(data); err != nil {
		slog.Warn("backward local write failed", "seq", seq, "error", err)
		conn.Close()
		bl.removeConn(seq)
	}
}

func (bl *BackwardListener) handleFin(seq uint64) {
	bl.mu.Lock()
	conn, ok := bl.seqConn[seq]
	if ok {
		conn.Close()
		delete(bl.seqConn, seq)
	}
	bl.mu.Unlock()
}

func (bl *BackwardListener) removeConn(seq uint64) {
	bl.mu.Lock()
	defer bl.mu.Unlock()
	if conn, ok := bl.seqConn[seq]; ok {
		conn.Close()
		delete(bl.seqConn, seq)
	}
}

func (bl *BackwardListener) sendBackwardData(seq uint64, data []byte) {
	header := &protocol.Header{
		Version:     1,
		Sender:      protocol.ControllerUUID,
		Accepter:    bl.nodeUUID,
		MessageType: protocol.BACKWARDDATA,
	}
	msg := &protocol.BackwardData{Seq: seq, DataLen: uint64(len(data)), Data: data}
	if err := bl.ctrl.SendToNode(bl.nodeUUID, header, msg); err != nil {
		slog.Error("send backward data failed", "seq", seq, "error", err)
		bl.removeConn(seq)
	}
}

func (bl *BackwardListener) sendBackwardFin(seq uint64) {
	header := &protocol.Header{
		Version:     1,
		Sender:      protocol.ControllerUUID,
		Accepter:    bl.nodeUUID,
		MessageType: protocol.BACKWARDFIN,
	}
	if err := bl.ctrl.SendToNode(bl.nodeUUID, header, &protocol.BackWardFin{Seq: seq}); err != nil {
		slog.Error("send backward fin failed", "seq", seq, "error", err)
	}
}
