package node

import (
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"

	"styx-mcp/internal/utils"
	"styx-mcp/pkg/protocol"
)

// handleSocks starts a SOCKS5 proxy listener on this node.
func (n *Node) handleSocks(req *protocol.SocksStart) {
	addr, _, err := utils.CheckIPPort(req.Addr)
	if err != nil {
		slog.Error("invalid socks address", "error", err)
		n.sendSocksReady(false)
		return
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("socks listen failed", "error", err)
		n.sendSocksReady(false)
		return
	}
	n.sendSocksReady(true)

	slog.Info("socks5 proxy listening", "addr", addr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			slog.Warn("socks accept failed", "error", err)
			continue
		}
		go n.handleSocksConn(conn)
	}
}

func (n *Node) sendSocksReady(ok bool) {
	res := &protocol.SocksReady{OK: 0}
	if ok {
		res.OK = 1
	}

	header := &protocol.Header{
		Version:     1,
		Sender:      n.UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.SOCKSREADY,
		RouteLen:    uint32(len(protocol.TEMP_ROUTE)),
		Route:       protocol.TEMP_ROUTE,
	}

	sMessage := protocol.NewUpMsg(n.ParentConn, n.Options.Secret, n.UUID)
	if err := protocol.ConstructMessage(sMessage, header, res, false); err != nil {
		slog.Error("send socks ready failed", "error", err)
		return
	}
	if err := sMessage.SendMessage(); err != nil {
		slog.Error("send socks ready failed", "error", err)
	}
}

func (n *Node) handleSocksConn(client net.Conn) {
	defer client.Close()

	// Read greeting.
	greeting := make([]byte, 2)
	if _, err := io.ReadFull(client, greeting); err != nil {
		return
	}
	if greeting[0] != 0x05 {
		return
	}

	nmethods := int(greeting[1])
	methods := make([]byte, nmethods)
	if _, err := io.ReadFull(client, methods); err != nil {
		return
	}

	// Accept no authentication.
	if _, err := client.Write([]byte{0x05, 0x00}); err != nil {
		return
	}

	// Read request.
	reqHeader := make([]byte, 4)
	if _, err := io.ReadFull(client, reqHeader); err != nil {
		return
	}
	if reqHeader[0] != 0x05 || reqHeader[1] != 0x01 {
		return
	}

	var targetAddr string
	switch reqHeader[3] {
	case 0x01: // IPv4
		ip := make([]byte, 4)
		if _, err := io.ReadFull(client, ip); err != nil {
			return
		}
		portBytes := make([]byte, 2)
		if _, err := io.ReadFull(client, portBytes); err != nil {
			return
		}
		port := binary.BigEndian.Uint16(portBytes)
		targetAddr = fmt.Sprintf("%s:%d", net.IP(ip).String(), port)
	case 0x03: // Domain
		lenByte := make([]byte, 1)
		if _, err := io.ReadFull(client, lenByte); err != nil {
			return
		}
		domain := make([]byte, lenByte[0])
		if _, err := io.ReadFull(client, domain); err != nil {
			return
		}
		portBytes := make([]byte, 2)
		if _, err := io.ReadFull(client, portBytes); err != nil {
			return
		}
		port := binary.BigEndian.Uint16(portBytes)
		targetAddr = fmt.Sprintf("%s:%d", string(domain), port)
	case 0x04: // IPv6
		ip := make([]byte, 16)
		if _, err := io.ReadFull(client, ip); err != nil {
			return
		}
		portBytes := make([]byte, 2)
		if _, err := io.ReadFull(client, portBytes); err != nil {
			return
		}
		port := binary.BigEndian.Uint16(portBytes)
		targetAddr = fmt.Sprintf("[%s]:%d", net.IP(ip).String(), port)
	default:
		client.Write([]byte{0x05, 0x08, 0x00, reqHeader[3]})
		return
	}

	target, err := net.Dial("tcp", targetAddr)
	if err != nil {
		client.Write([]byte{0x05, 0x05, 0x00, reqHeader[3]})
		return
	}
	defer target.Close()

	// Send success response.
	localAddr := target.LocalAddr().(*net.TCPAddr)
	resp := append([]byte{0x05, 0x00, 0x00, 0x01}, localAddr.IP.To4()...)
	portBytes := make([]byte, 2)
	binary.BigEndian.PutUint16(portBytes, uint16(localAddr.Port))
	resp = append(resp, portBytes...)
	if _, err := client.Write(resp); err != nil {
		return
	}

	// Relay traffic.
	done := make(chan struct{}, 2)
	go func() {
		io.Copy(target, client)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(client, target)
		done <- struct{}{}
	}()
	<-done
}
