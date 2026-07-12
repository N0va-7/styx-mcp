package controller

import (
	"io"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"styx-mcp/internal/utils"
	"styx-mcp/pkg/protocol"
)

// SocksService is a local SOCKS5 listener that tunnels through an agent node.
type SocksService struct {
	ctrl     *Controller
	nodeUUID string
	address  string
	listener net.Listener
	seqConn  map[uint64]net.Conn
	mu       sync.RWMutex
	seqGen   uint64
	readyCh  chan bool
	stopCh   chan struct{}
}

// StartSocks listens on the controller and forwards SOCKS5 traffic via the node.
func (c *Controller) StartSocks(nodeUUID, localAddr string) error {
	addr, _, err := utils.CheckIPPort(localAddr)
	if err != nil {
		return err
	}

	c.socksServicesMu.Lock()
	if existing, ok := c.socksServices[nodeUUID]; ok {
		c.socksServicesMu.Unlock()
		existing.Stop()
		c.socksServicesMu.Lock()
		delete(c.socksServices, nodeUUID)
	}
	c.socksServicesMu.Unlock()

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}

	svc := &SocksService{
		ctrl:     c,
		nodeUUID: nodeUUID,
		address:  addr,
		listener: listener,
		seqConn:  make(map[uint64]net.Conn),
		readyCh:  make(chan bool, 1),
		stopCh:   make(chan struct{}),
	}

	c.socksServicesMu.Lock()
	c.socksServices[nodeUUID] = svc
	c.socksServicesMu.Unlock()

	// Ask the agent to prepare for SOCKS streams (no listen on agent).
	header := &protocol.Header{
		Version:     1,
		Sender:      protocol.ADMIN_UUID,
		Accepter:    nodeUUID,
		MessageType: protocol.SOCKSSTART,
	}
	req := &protocol.SocksStart{
		AddrLen: uint16(len(addr)),
		Addr:    addr,
	}
	if err := c.SendToNode(nodeUUID, header, req); err != nil {
		listener.Close()
		c.socksServicesMu.Lock()
		delete(c.socksServices, nodeUUID)
		c.socksServicesMu.Unlock()
		return err
	}

	select {
	case ok := <-svc.readyCh:
		if !ok {
			listener.Close()
			c.socksServicesMu.Lock()
			delete(c.socksServices, nodeUUID)
			c.socksServicesMu.Unlock()
			return errSocksNotReady
		}
	case <-time.After(10 * time.Second):
		listener.Close()
		c.socksServicesMu.Lock()
		delete(c.socksServices, nodeUUID)
		c.socksServicesMu.Unlock()
		return errSocksTimeout
	}

	go svc.Run()
	slog.Info("socks5 listening on controller", "addr", addr, "node", nodeUUID)
	return nil
}

var (
	errSocksNotReady = &socksError{"agent rejected socks start"}
	errSocksTimeout  = &socksError{"timeout waiting for socks ready"}
)

type socksError struct{ msg string }

func (e *socksError) Error() string { return e.msg }

// Run accepts local SOCKS clients and tunnels them through the node.
func (s *SocksService) Run() {
	defer s.listener.Close()

	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.stopCh:
				return
			default:
			}
			slog.Warn("socks accept failed", "error", err)
			continue
		}
		go s.handleLocalConn(conn)
	}
}

// Stop shuts down the local SOCKS listener and sessions.
func (s *SocksService) Stop() {
	select {
	case <-s.stopCh:
	default:
		close(s.stopCh)
	}
	s.listener.Close()
	s.mu.Lock()
	for _, conn := range s.seqConn {
		conn.Close()
	}
	s.seqConn = make(map[uint64]net.Conn)
	s.mu.Unlock()
}

func (s *SocksService) handleLocalConn(conn net.Conn) {
	seq := atomic.AddUint64(&s.seqGen, 1)
	s.mu.Lock()
	s.seqConn[seq] = conn
	s.mu.Unlock()

	defer func() {
		s.sendSocksFin(seq)
		s.removeConn(seq)
	}()

	buf := make([]byte, 32*1024)
	for {
		select {
		case <-s.stopCh:
			return
		default:
		}

		nr, err := conn.Read(buf)
		if nr > 0 {
			s.sendSocksData(seq, buf[:nr])
		}
		if err != nil {
			if err != io.EOF {
				slog.Debug("socks local read closed", "seq", seq, "error", err)
			}
			return
		}
	}
}

func (s *SocksService) handleReady(ok bool) {
	select {
	case s.readyCh <- ok:
	default:
	}
}

func (s *SocksService) handleData(seq uint64, data []byte) {
	s.mu.RLock()
	conn, ok := s.seqConn[seq]
	s.mu.RUnlock()
	if !ok {
		slog.Warn("socks data for unknown seq", "seq", seq)
		return
	}
	if _, err := conn.Write(data); err != nil {
		slog.Warn("socks local write failed", "seq", seq, "error", err)
		conn.Close()
		s.removeConn(seq)
	}
}

func (s *SocksService) handleFin(seq uint64) {
	s.removeConn(seq)
}

func (s *SocksService) removeConn(seq uint64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if conn, ok := s.seqConn[seq]; ok {
		conn.Close()
		delete(s.seqConn, seq)
	}
}

func (s *SocksService) sendSocksData(seq uint64, data []byte) {
	header := &protocol.Header{
		Version:     1,
		Sender:      protocol.ADMIN_UUID,
		Accepter:    s.nodeUUID,
		MessageType: protocol.SOCKSTCPDATA,
	}
	msg := &protocol.SocksTCPData{Seq: seq, DataLen: uint64(len(data)), Data: data}
	if err := s.ctrl.SendToNode(s.nodeUUID, header, msg); err != nil {
		slog.Error("send socks data failed", "seq", seq, "error", err)
		s.removeConn(seq)
	}
}

func (s *SocksService) sendSocksFin(seq uint64) {
	header := &protocol.Header{
		Version:     1,
		Sender:      protocol.ADMIN_UUID,
		Accepter:    s.nodeUUID,
		MessageType: protocol.SOCKSTCPFIN,
	}
	if err := s.ctrl.SendToNode(s.nodeUUID, header, &protocol.SocksTCPFin{Seq: seq}); err != nil {
		slog.Error("send socks fin failed", "seq", seq, "error", err)
	}
}
