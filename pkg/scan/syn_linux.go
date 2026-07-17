//go:build linux

package scan

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// synChecker sends TCP SYN and waits for SYN-ACK via raw ip4:tcp.
type synChecker struct {
	timeout time.Duration
	conn    net.PacketConn
	srcIP   net.IP // IPv4 local address used in pseudo-header (best-effort)

	mu      sync.Mutex
	waiters map[string]chan bool // key: "remoteIP:dport:sport"
	sport   uint32
	closed  atomic.Bool
	done    chan struct{}
}

func newSynChecker(timeout time.Duration) (PortChecker, error) {
	pc, err := net.ListenPacket("ip4:tcp", "0.0.0.0")
	if err != nil {
		return nil, err
	}
	s := &synChecker{
		timeout: timeout,
		conn:    pc,
		srcIP:   pickLocalIPv4(),
		waiters: make(map[string]chan bool),
		sport:   40000,
		done:    make(chan struct{}),
	}
	go s.readLoop()
	return s, nil
}

func (s *synChecker) Method() string { return MethodSYN }

func (s *synChecker) Close() error {
	if s.closed.Swap(true) {
		return nil
	}
	close(s.done)
	return s.conn.Close()
}

func (s *synChecker) nextSport() uint16 {
	n := atomic.AddUint32(&s.sport, 1)
	// Keep ephemeral-ish range.
	return uint16(40000 + (n % 20000))
}

func (s *synChecker) Open(ctx context.Context, ip string, port uint16) (bool, error) {
	if s.closed.Load() {
		return false, errf("syn checker closed")
	}
	dstIP := net.ParseIP(ip).To4()
	if dstIP == nil {
		return false, nil
	}
	sport := s.nextSport()
	key := fmt.Sprintf("%s:%d:%d", dstIP.String(), port, sport)
	ch := make(chan bool, 1)

	s.mu.Lock()
	s.waiters[key] = ch
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		delete(s.waiters, key)
		s.mu.Unlock()
	}()

	pkt := buildTCPSYN(s.srcIP, dstIP, sport, port)
	deadline := time.Now().Add(s.timeout)
	if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
		deadline = dl
	}
	_ = s.conn.SetWriteDeadline(deadline)
	if _, err := s.conn.WriteTo(pkt, &net.IPAddr{IP: dstIP}); err != nil {
		return false, nil
	}

	timer := time.NewTimer(time.Until(deadline))
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false, nil
	case <-timer.C:
		return false, nil
	case ok := <-ch:
		return ok, nil
	}
}

func (s *synChecker) readLoop() {
	buf := make([]byte, 64)
	for {
		if s.closed.Load() {
			return
		}
		_ = s.conn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, addr, err := s.conn.ReadFrom(buf)
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				continue
			}
		}
		if n < 20 {
			continue
		}
		// PacketConn ip4:tcp delivers TCP segment (no IP header) on Linux.
		tcp := buf[:n]
		srcPort := binary.BigEndian.Uint16(tcp[0:2])
		dstPort := binary.BigEndian.Uint16(tcp[2:4])
		flags := tcp[13]
		// SYN+ACK = 0x12
		if flags&0x12 != 0x12 {
			continue
		}
		rip := ""
		if ipa, ok := addr.(*net.IPAddr); ok && ipa.IP != nil {
			rip = ipa.IP.To4().String()
		} else if hip, ok := addr.(*net.IPAddr); ok {
			rip = hip.String()
		} else {
			rip = addr.String()
		}
		// We sent sport=local, dport=remote; reply has src=remotePort, dst=localSport.
		key := fmt.Sprintf("%s:%d:%d", rip, srcPort, dstPort)
		s.mu.Lock()
		ch := s.waiters[key]
		s.mu.Unlock()
		if ch != nil {
			select {
			case ch <- true:
			default:
			}
		}
	}
}

func buildTCPSYN(srcIP, dstIP net.IP, sport, dport uint16) []byte {
	tcp := make([]byte, 20)
	binary.BigEndian.PutUint16(tcp[0:2], sport)
	binary.BigEndian.PutUint16(tcp[2:4], dport)
	binary.BigEndian.PutUint32(tcp[4:8], 0) // seq
	binary.BigEndian.PutUint32(tcp[8:12], 0)
	tcp[12] = 5 << 4 // data offset 5 * 4 = 20
	tcp[13] = 0x02   // SYN
	binary.BigEndian.PutUint16(tcp[14:16], 65535)
	// checksum at 16:18
	binary.BigEndian.PutUint16(tcp[18:20], 0)

	src := srcIP.To4()
	dst := dstIP.To4()
	if src == nil {
		src = net.IPv4zero.To4()
	}
	sum := tcpChecksum(src, dst, tcp)
	binary.BigEndian.PutUint16(tcp[16:18], sum)
	return tcp
}

func tcpChecksum(src, dst net.IP, tcp []byte) uint16 {
	// Pseudo-header + TCP
	var sum uint32
	for i := 0; i < 4; i += 2 {
		sum += uint32(binary.BigEndian.Uint16(src[i : i+2]))
		sum += uint32(binary.BigEndian.Uint16(dst[i : i+2]))
	}
	sum += 6 // protocol TCP
	sum += uint32(len(tcp))
	for i := 0; i+1 < len(tcp); i += 2 {
		sum += uint32(binary.BigEndian.Uint16(tcp[i : i+2]))
	}
	if len(tcp)%2 == 1 {
		sum += uint32(tcp[len(tcp)-1]) << 8
	}
	for sum > 0xffff {
		sum = (sum >> 16) + (sum & 0xffff)
	}
	return ^uint16(sum)
}

func pickLocalIPv4() net.IP {
	ifaces, err := net.Interfaces()
	if err != nil {
		return net.IPv4zero.To4()
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip4 := ip.To4(); ip4 != nil && !ip4.IsLoopback() {
				return ip4
			}
		}
	}
	return net.IPv4zero.To4()
}
