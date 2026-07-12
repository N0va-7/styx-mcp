package node

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"styx-mcp/pkg/protocol"
	"styx-mcp/pkg/socksflow"
)

// SocksManager handles SOCKS5 streams tunneled from the controller.
type SocksManager struct {
	n        *Node
	sessions map[uint64]*socksSession
	mu       sync.RWMutex
}

type socksSession struct {
	inbox *socksflow.ByteQueue
	send  *socksflow.Window
	once  sync.Once
}

func (s *socksSession) close() {
	s.once.Do(func() {
		s.send.Close()
		s.inbox.Close()
	})
}

// NewSocksManager creates a SOCKS stream manager for a node.
func NewSocksManager(n *Node) *SocksManager {
	return &SocksManager{
		n:        n,
		sessions: make(map[uint64]*socksSession),
	}
}

// handleSocksStart prepares the agent to accept tunneled SOCKS streams.
func (sm *SocksManager) handleSocksStart(_ *protocol.SocksStart) {
	sm.mu.Lock()
	for _, sess := range sm.sessions {
		sess.close()
	}
	sm.sessions = make(map[uint64]*socksSession)
	sm.mu.Unlock()

	sm.sendSocksReady(true)
	slog.Info("socks ready (controller-side listener)")
}

func (sm *SocksManager) getOrCreateSession(seq uint64) *socksSession {
	sm.mu.Lock()
	defer sm.mu.Unlock()
	sess, ok := sm.sessions[seq]
	if !ok {
		sess = &socksSession{
			inbox: socksflow.NewByteQueue(socksflow.InitialWindow),
			send:  socksflow.NewWindow(socksflow.InitialWindow),
		}
		sm.sessions[seq] = sess
		go sm.runSession(seq, sess)
	}
	return sess
}

func (sm *SocksManager) handleSocksData(req *protocol.SocksTCPData) {
	sess := sm.getOrCreateSession(req.Seq)
	if err := sess.inbox.Push(req.Data); err != nil {
		slog.Warn("socks inbox push failed", "seq", req.Seq, "error", err)
		sm.sendSocksFin(req.Seq)
		sm.removeSession(req.Seq)
	}
}

func (sm *SocksManager) handleSocksAck(req *protocol.SocksTCPAck) {
	sm.mu.RLock()
	sess, ok := sm.sessions[req.Seq]
	sm.mu.RUnlock()
	if !ok {
		return
	}
	sess.send.Release(req.Credit)
}

func (sm *SocksManager) handleSocksFin(req *protocol.SocksTCPFin) {
	sm.removeSession(req.Seq)
}

func (sm *SocksManager) removeSession(seq uint64) {
	sm.mu.Lock()
	sess, ok := sm.sessions[seq]
	if ok {
		delete(sm.sessions, seq)
	}
	sm.mu.Unlock()
	if ok {
		sess.close()
	}
}

func (sm *SocksManager) runSession(seq uint64, sess *socksSession) {
	defer func() {
		sm.sendSocksFin(seq)
		sm.removeSession(seq)
	}()

	greeting, err := sess.inbox.Pop()
	if err != nil || len(greeting) < 2 || greeting[0] != 0x05 {
		return
	}
	sm.sendSocksAck(seq, uint64(len(greeting)))
	if err := sess.send.Acquire(2); err != nil {
		return
	}
	if err := sm.sendSocksData(seq, []byte{0x05, 0x00}); err != nil {
		return
	}

	req, err := sess.inbox.Pop()
	if err != nil || len(req) < 4 || req[0] != 0x05 || req[1] != 0x01 {
		return
	}
	sm.sendSocksAck(seq, uint64(len(req)))

	targetAddr, err := parseSocksAddr(req)
	if err != nil {
		_ = sess.send.Acquire(10)
		_ = sm.sendSocksData(seq, []byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	target, err := net.Dial("tcp", targetAddr)
	if err != nil {
		slog.Warn("socks dial failed", "seq", seq, "target", targetAddr, "error", err)
		_ = sess.send.Acquire(10)
		_ = sm.sendSocksData(seq, []byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}
	defer target.Close()

	localAddr := target.LocalAddr().(*net.TCPAddr)
	ip4 := localAddr.IP.To4()
	if ip4 == nil {
		ip4 = net.IPv4zero
	}
	resp := append([]byte{0x05, 0x00, 0x00, 0x01}, ip4...)
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(localAddr.Port))
	resp = append(resp, portBytes...)
	if err := sess.send.Acquire(uint64(len(resp))); err != nil {
		return
	}
	if err := sm.sendSocksData(seq, resp); err != nil {
		return
	}

	slog.Info("socks connected", "seq", seq, "target", targetAddr)

	done := make(chan struct{}, 2)

	// controller -> target
	go func() {
		defer func() { done <- struct{}{} }()
		for {
			chunk, err := sess.inbox.Pop()
			if err != nil {
				return
			}
			if _, err := target.Write(chunk); err != nil {
				return
			}
			sm.sendSocksAck(seq, uint64(len(chunk)))
		}
	}()

	// target -> controller
	go func() {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, 32*1024)
		for {
			nr, err := target.Read(buf)
			if nr > 0 {
				if err := sess.send.Acquire(uint64(nr)); err != nil {
					return
				}
				if err := sm.sendSocksData(seq, buf[:nr]); err != nil {
					return
				}
			}
			if err != nil {
				return
			}
		}
	}()

	<-done
}

func parseSocksAddr(req []byte) (string, error) {
	if len(req) < 4 {
		return "", fmt.Errorf("short request")
	}
	off := 4
	switch req[3] {
	case 0x01: // IPv4
		if len(req) < off+6 {
			return "", fmt.Errorf("short ipv4")
		}
		ip := net.IP(req[off : off+4])
		port := binary.BigEndian.Uint16(req[off+4 : off+6])
		return fmt.Sprintf("%s:%d", ip.String(), port), nil
	case 0x03: // Domain
		if len(req) < off+1 {
			return "", fmt.Errorf("short domain len")
		}
		dlen := int(req[off])
		off++
		if len(req) < off+dlen+2 {
			return "", fmt.Errorf("short domain")
		}
		domain := string(req[off : off+dlen])
		port := binary.BigEndian.Uint16(req[off+dlen : off+dlen+2])
		return fmt.Sprintf("%s:%d", domain, port), nil
	case 0x04: // IPv6
		if len(req) < off+18 {
			return "", fmt.Errorf("short ipv6")
		}
		ip := net.IP(req[off : off+16])
		port := binary.BigEndian.Uint16(req[off+16 : off+18])
		return fmt.Sprintf("[%s]:%d", ip.String(), port), nil
	default:
		return "", fmt.Errorf("unsupported atyp %d", req[3])
	}
}

func (sm *SocksManager) sendSocksReady(ok bool) {
	res := &protocol.SocksReady{OK: 0}
	if ok {
		res.OK = 1
	}
	header := &protocol.Header{
		Version:     1,
		Sender:      sm.n.UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.SOCKSREADY,
		RouteLen:    uint32(len(protocol.TEMP_ROUTE)),
		Route:       protocol.TEMP_ROUTE,
	}
	sm.sendToUpstream(header, res)
}

func (sm *SocksManager) sendSocksData(seq uint64, data []byte) error {
	header := &protocol.Header{
		Version:     1,
		Sender:      sm.n.UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.SOCKSTCPDATA,
		RouteLen:    uint32(len(protocol.TEMP_ROUTE)),
		Route:       protocol.TEMP_ROUTE,
	}
	msg := &protocol.SocksTCPData{Seq: seq, DataLen: uint64(len(data)), Data: data}
	return sm.sendToUpstreamErr(header, msg)
}

func (sm *SocksManager) sendSocksAck(seq uint64, credit uint64) {
	header := &protocol.Header{
		Version:     1,
		Sender:      sm.n.UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.SOCKSTCPACK,
		RouteLen:    uint32(len(protocol.TEMP_ROUTE)),
		Route:       protocol.TEMP_ROUTE,
	}
	sm.sendToUpstream(header, &protocol.SocksTCPAck{Seq: seq, Credit: credit})
}

func (sm *SocksManager) sendSocksFin(seq uint64) {
	header := &protocol.Header{
		Version:     1,
		Sender:      sm.n.UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.SOCKSTCPFIN,
		RouteLen:    uint32(len(protocol.TEMP_ROUTE)),
		Route:       protocol.TEMP_ROUTE,
	}
	sm.sendToUpstream(header, &protocol.SocksTCPFin{Seq: seq})
}

func (sm *SocksManager) sendToUpstream(header *protocol.Header, payload interface{}) {
	_ = sm.sendToUpstreamErr(header, payload)
}

func (sm *SocksManager) sendToUpstreamErr(header *protocol.Header, payload interface{}) error {
	sMessage := protocol.NewUpMsg(sm.n.ParentConn, sm.n.Options.Secret, sm.n.UUID)
	if err := protocol.ConstructMessage(sMessage, header, payload, false); err != nil {
		slog.Error("send socks message failed", "error", err)
		return err
	}
	if err := sMessage.SendMessage(); err != nil {
		slog.Error("send socks message failed", "error", err)
		return err
	}
	return nil
}
