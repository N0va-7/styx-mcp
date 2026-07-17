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
	"styx-mcp/pkg/socksflow"
)

// SocksService is a local SOCKS5 listener that tunnels through an agent node.
type SocksService struct {
	ctrl     *Controller
	nodeUUID string
	address  string
	listener net.Listener
	seqConn  map[uint64]*socksStream
	mu       sync.RWMutex
	seqGen   uint64
	readyCh  chan bool
	stopCh   chan struct{}
}

type socksStream struct {
	conn       net.Conn
	send       *socksflow.Window
	inbox      *socksflow.ByteQueue
	once       sync.Once
	remoteOnce sync.Once
	remoteDone chan struct{} // closed when agent signals FIN
}

func (ss *socksStream) close() {
	ss.once.Do(func() {
		ss.send.Close()
		ss.inbox.Close()
		ss.conn.Close()
		ss.remoteOnce.Do(func() { close(ss.remoteDone) })
	})
}

func (ss *socksStream) markRemoteDone() {
	ss.remoteOnce.Do(func() { close(ss.remoteDone) })
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
		seqConn:  make(map[uint64]*socksStream),
		readyCh:  make(chan bool, 1),
		stopCh:   make(chan struct{}),
	}

	c.socksServicesMu.Lock()
	c.socksServices[nodeUUID] = svc
	c.socksServicesMu.Unlock()

	header := &protocol.Header{
		Version:     1,
		Sender:      protocol.ControllerUUID,
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
	for _, ss := range s.seqConn {
		ss.close()
	}
	s.seqConn = make(map[uint64]*socksStream)
	s.mu.Unlock()
}

func (s *SocksService) handleLocalConn(conn net.Conn) {
	seq := atomic.AddUint64(&s.seqGen, 1)
	ss := &socksStream{
		conn:       conn,
		send:       socksflow.NewWindow(socksflow.InitialWindow),
		inbox:      socksflow.NewByteQueue(socksflow.InitialWindow),
		remoteDone: make(chan struct{}),
	}

	s.mu.Lock()
	s.seqConn[seq] = ss
	s.mu.Unlock()

	// Drain remote→local for the lifetime of the stream (including after local EOF).
	drainDone := make(chan struct{})
	go func() {
		defer close(drainDone)
		s.drainInbox(seq, ss)
	}()

	// Local → remote: on EOF do not FIN yet — keep reverse path open so HTTP
	// responses can finish after the client half-closes its write side.
	buf := make([]byte, 32*1024)
	for {
		select {
		case <-s.stopCh:
			ss.inbox.Close()
			<-drainDone
			s.removeConn(seq)
			return
		default:
		}

		nr, err := conn.Read(buf)
		if nr > 0 {
			if err := ss.send.Acquire(uint64(nr)); err != nil {
				ss.inbox.Close()
				<-drainDone
				s.removeConn(seq)
				return
			}
			if err := s.sendSocksData(seq, buf[:nr]); err != nil {
				ss.inbox.Close()
				<-drainDone
				s.removeConn(seq)
				return
			}
		}
		if err != nil {
			if err != io.EOF {
				slog.Debug("socks local read closed", "seq", seq, "error", err)
			}
			break
		}
	}

	// Wait until agent FINs (or timeout/stop), then let drain finish remaining bytes.
	// Closing the inbox only after remoteDone ensures in-flight response is written
	// before we tear down; never CloseWrite before drain completes.
	select {
	case <-ss.remoteDone:
	case <-time.After(5 * time.Minute):
		slog.Debug("socks wait remote done timeout", "seq", seq)
		ss.inbox.Close()
	case <-s.stopCh:
		ss.inbox.Close()
	}

	<-drainDone
	s.removeConn(seq)
}

func (s *SocksService) drainInbox(seq uint64, ss *socksStream) {
	for {
		chunk, err := ss.inbox.Pop()
		if err != nil {
			return
		}
		if _, err := ss.conn.Write(chunk); err != nil {
			slog.Debug("socks local write closed", "seq", seq, "error", err)
			// Unblock handleLocalConn if it is still waiting on remote FIN.
			ss.markRemoteDone()
			return
		}
		s.sendSocksAck(seq, uint64(len(chunk)))
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
	ss, ok := s.seqConn[seq]
	s.mu.RUnlock()
	if !ok {
		slog.Warn("socks data for unknown seq", "seq", seq)
		return
	}
	if err := ss.inbox.Push(data); err != nil {
		slog.Warn("socks inbox push failed", "seq", seq, "error", err)
		s.removeConn(seq)
	}
}

func (s *SocksService) handleAck(seq uint64, credit uint64) {
	s.mu.RLock()
	ss, ok := s.seqConn[seq]
	s.mu.RUnlock()
	if !ok {
		return
	}
	ss.send.Release(credit)
}

func (s *SocksService) handleFin(seq uint64) {
	s.mu.RLock()
	ss, ok := s.seqConn[seq]
	s.mu.RUnlock()
	if !ok {
		return
	}
	// Signal remote completion and close inbox so drainInbox can finish any
	// already-buffered chunks, then exit. Do not CloseWrite here: that races
	// with drain and truncates the HTTP response (empty reply / broken pipe).
	ss.markRemoteDone()
	ss.inbox.Close()
}

func (s *SocksService) removeConn(seq uint64) {
	s.mu.Lock()
	ss, ok := s.seqConn[seq]
	if ok {
		delete(s.seqConn, seq)
	}
	s.mu.Unlock()
	if ok {
		// Tell agent we are done (idempotent if already closed).
		s.sendSocksFin(seq)
		ss.close()
	}
}

func (s *SocksService) sendSocksData(seq uint64, data []byte) error {
	header := &protocol.Header{
		Version:     1,
		Sender:      protocol.ControllerUUID,
		Accepter:    s.nodeUUID,
		MessageType: protocol.SOCKSTCPDATA,
	}
	msg := &protocol.SocksTCPData{Seq: seq, DataLen: uint64(len(data)), Data: data}
	if err := s.ctrl.SendToNode(s.nodeUUID, header, msg); err != nil {
		slog.Error("send socks data failed", "seq", seq, "error", err)
		s.removeConn(seq)
		return err
	}
	return nil
}

func (s *SocksService) sendSocksAck(seq uint64, credit uint64) {
	header := &protocol.Header{
		Version:     1,
		Sender:      protocol.ControllerUUID,
		Accepter:    s.nodeUUID,
		MessageType: protocol.SOCKSTCPACK,
	}
	msg := &protocol.SocksTCPAck{Seq: seq, Credit: credit}
	if err := s.ctrl.SendToNode(s.nodeUUID, header, msg); err != nil {
		slog.Error("send socks ack failed", "seq", seq, "error", err)
	}
}

func (s *SocksService) sendSocksFin(seq uint64) {
	header := &protocol.Header{
		Version:     1,
		Sender:      protocol.ControllerUUID,
		Accepter:    s.nodeUUID,
		MessageType: protocol.SOCKSTCPFIN,
	}
	if err := s.ctrl.SendToNode(s.nodeUUID, header, &protocol.SocksTCPFin{Seq: seq}); err != nil {
		slog.Error("send socks fin failed", "seq", seq, "error", err)
	}
}
