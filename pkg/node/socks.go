package node

import (
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"styx-mcp/pkg/protocol"
)

// SocksManager handles SOCKS5 streams tunneled from the controller.
type SocksManager struct {
	n        *Node
	sessions map[uint64]*socksSession
	mu       sync.RWMutex
}

type socksSession struct {
	dataChan chan []byte
	once     sync.Once
}

// NewSocksManager creates a SOCKS stream manager for a node.
func NewSocksManager(n *Node) *SocksManager {
	return &SocksManager{
		n:        n,
		sessions: make(map[uint64]*socksSession),
	}
}

// handleSocksStart prepares the agent to accept tunneled SOCKS streams.
// Unlike the old behavior, it does NOT listen locally.
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

func (sm *SocksManager) handleSocksData(req *protocol.SocksTCPData) {
	sm.mu.Lock()
	sess, ok := sm.sessions[req.Seq]
	if !ok {
		sess = &socksSession{dataChan: make(chan []byte, 32)}
		sm.sessions[req.Seq] = sess
		go sm.runSession(req.Seq, sess)
	}
	sm.mu.Unlock()

	select {
	case sess.dataChan <- append([]byte(nil), req.Data...):
	default:
		slog.Warn("socks data chan full", "seq", req.Seq)
	}
}

func (sm *SocksManager) handleSocksFin(req *protocol.SocksTCPFin) {
	sm.mu.Lock()
	sess, ok := sm.sessions[req.Seq]
	if ok {
		delete(sm.sessions, req.Seq)
	}
	sm.mu.Unlock()
	if ok {
		sess.close()
	}
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

func (s *socksSession) close() {
	s.once.Do(func() {
		close(s.dataChan)
	})
}

func (sm *SocksManager) runSession(seq uint64, sess *socksSession) {
	defer func() {
		sm.sendSocksFin(seq)
		sm.removeSession(seq)
	}()

	// 1) SOCKS greeting
	greeting, ok := <-sess.dataChan
	if !ok || len(greeting) < 2 || greeting[0] != 0x05 {
		return
	}
	sm.sendSocksData(seq, []byte{0x05, 0x00})

	// 2) CONNECT request
	req, ok := <-sess.dataChan
	if !ok || len(req) < 4 || req[0] != 0x05 || req[1] != 0x01 {
		return
	}

	targetAddr, err := parseSocksAddr(req)
	if err != nil {
		sm.sendSocksData(seq, []byte{0x05, 0x08, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
		return
	}

	target, err := net.Dial("tcp", targetAddr)
	if err != nil {
		slog.Warn("socks dial failed", "seq", seq, "target", targetAddr, "error", err)
		sm.sendSocksData(seq, []byte{0x05, 0x05, 0x00, 0x01, 0, 0, 0, 0, 0, 0})
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
	sm.sendSocksData(seq, resp)

	slog.Info("socks connected", "seq", seq, "target", targetAddr)

	// 3) Relay: controller -> target
	done := make(chan struct{}, 2)
	go func() {
		defer func() { done <- struct{}{} }()
		for data := range sess.dataChan {
			if _, err := target.Write(data); err != nil {
				return
			}
		}
	}()

	// 4) Relay: target -> controller
	go func() {
		defer func() { done <- struct{}{} }()
		buf := make([]byte, 32*1024)
		for {
			nr, err := target.Read(buf)
			if nr > 0 {
				sm.sendSocksData(seq, buf[:nr])
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

func (sm *SocksManager) sendSocksData(seq uint64, data []byte) {
	header := &protocol.Header{
		Version:     1,
		Sender:      sm.n.UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.SOCKSTCPDATA,
		RouteLen:    uint32(len(protocol.TEMP_ROUTE)),
		Route:       protocol.TEMP_ROUTE,
	}
	msg := &protocol.SocksTCPData{Seq: seq, DataLen: uint64(len(data)), Data: data}
	sm.sendToUpstream(header, msg)
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
	sMessage := protocol.NewUpMsg(sm.n.ParentConn, sm.n.Options.Secret, sm.n.UUID)
	if err := protocol.ConstructMessage(sMessage, header, payload, false); err != nil {
		slog.Error("send socks message failed", "error", err)
		return
	}
	if err := sMessage.SendMessage(); err != nil {
		slog.Error("send socks message failed", "error", err)
	}
}
