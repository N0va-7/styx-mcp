package node

import (
	"log/slog"
	"net"

	"styx-mcp/pkg/protocol"
	"styx-mcp/pkg/share/preauth"
	"styx-mcp/pkg/transport"
)

// handleConnect actively connects to a new child node.
func (n *Node) handleConnect(req *protocol.ConnectStart) {
	slog.Info("connecting to child", "addr", req.Addr)
	conn, err := net.Dial("tcp", req.Addr)
	if err != nil {
		slog.Error("connect to child failed", "addr", req.Addr, "error", err)
		n.sendConnectDone(false)
		return
	}
	slog.Info("connected to child", "addr", req.Addr)

	// Parent (active connector) announces itself with admin flag set.
	hiMess := &protocol.HIMess{
		GreetingLen: uint16(len("Shhh...")),
		Greeting:    "Shhh...",
		UUIDLen:     uint16(len(protocol.TEMP_UUID)),
		UUID:        protocol.TEMP_UUID,
		IsAdmin:     1,
		IsReconnect: 0,
	}

	header := &protocol.Header{
		Version:     1,
		Sender:      n.UUID,
		Accepter:    protocol.TEMP_UUID,
		MessageType: protocol.HI,
		RouteLen:    uint32(len(protocol.TEMP_ROUTE)),
		Route:       protocol.TEMP_ROUTE,
	}

	if n.Options.TlsEnable {
		config, err := transport.NewClientTLSConfig(n.Options.Domain)
		if err != nil {
			conn.Close()
			n.sendConnectDone(false)
			return
		}
		conn = transport.WrapTLSClientConn(conn, config)
	}

	param := &protocol.NegParam{Conn: conn, Domain: n.Options.Domain}
	proto := protocol.NewDownProto(param)
	if err := proto.CNegotiate(); err != nil {
		conn.Close()
		n.sendConnectDone(false)
		return
	}

	if err := preauth.ActivePreAuth(conn); err != nil {
		conn.Close()
		n.sendConnectDone(false)
		return
	}

	sMessage := protocol.NewDownMsg(conn, n.Options.Secret, n.UUID)
	if err := protocol.ConstructMessage(sMessage, header, hiMess, false); err != nil {
		conn.Close()
		n.sendConnectDone(false)
		return
	}
	if err := sMessage.SendMessage(); err != nil {
		conn.Close()
		n.sendConnectDone(false)
		return
	}

	// Read child HI response. The child treats us as admin, so use ADMIN_UUID
	// as the local UUID to trigger decryption of messages addressed to ADMIN_UUID.
	rMessage := protocol.NewDownMsg(conn, n.Options.Secret, protocol.ADMIN_UUID)
	fHeader, fMessage, err := protocol.DestructMessage(rMessage)
	if err != nil {
		slog.Error("read child HI response failed", "error", err)
		conn.Close()
		n.sendConnectDone(false)
		return
	}

	if fHeader.MessageType != protocol.HI {
		slog.Error("child did not send HI", "type", fHeader.MessageType)
		conn.Close()
		n.sendConnectDone(false)
		return
	}

	mmess := fMessage.(*protocol.HIMess)
	if mmess.Greeting != "Keep silent" || mmess.IsAdmin != 0 {
		slog.Error("child HI invalid", "greeting", mmess.Greeting, "isAdmin", mmess.IsAdmin)
		conn.Close()
		n.sendConnectDone(false)
		return
	}
	// Request UUID from controller for this new child.
	childIP := conn.RemoteAddr().String()
	childReq := &protocol.ChildUUIDReq{
		ParentUUIDLen: uint16(len(n.UUID)),
		ParentUUID:    n.UUID,
		IPLen:         uint16(len(childIP)),
		IP:            childIP,
	}

	reqHeader := &protocol.Header{
		Version:     1,
		Sender:      n.UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.CHILDUUIDREQ,
		RouteLen:    uint32(len(protocol.TEMP_ROUTE)),
		Route:       protocol.TEMP_ROUTE,
	}

	sMessage = protocol.NewUpMsg(n.ParentConn, n.Options.Secret, n.UUID)
	if err := protocol.ConstructMessage(sMessage, reqHeader, childReq, false); err != nil {
		conn.Close()
		n.sendConnectDone(false)
		return
	}
	if err := sMessage.SendMessage(); err != nil {
		conn.Close()
		n.sendConnectDone(false)
		return
	}

	// Wait for UUID assignment.
	respHeader, respMessage, err := protocol.DestructMessage(protocol.NewUpMsg(n.ParentConn, n.Options.Secret, n.UUID))
	if err != nil {
		slog.Error("wait for child UUID failed", "error", err)
		conn.Close()
		n.sendConnectDone(false)
		return
	}
	if respHeader.MessageType != protocol.CHILDUUIDRES {
		slog.Error("unexpected response type", "type", respHeader.MessageType)
		conn.Close()
		n.sendConnectDone(false)
		return
	}

	uuidRes := respMessage.(*protocol.ChildUUIDRes)
	childUUID := uuidRes.UUID

	// Send UUID to child.
	uuidMess := &protocol.UUIDMess{
		UUIDLen: uint16(len(childUUID)),
		UUID:    childUUID,
	}
	uuidHeader := &protocol.Header{
		Version:     1,
		Sender:      n.UUID,
		Accepter:    protocol.TEMP_UUID,
		MessageType: protocol.UUID,
		RouteLen:    uint32(len(protocol.TEMP_ROUTE)),
		Route:       protocol.TEMP_ROUTE,
	}

	downMsg := protocol.NewDownMsg(conn, n.Options.Secret, n.UUID)
	if err := protocol.ConstructMessage(downMsg, uuidHeader, uuidMess, false); err != nil {
		slog.Error("send UUID to child failed", "error", err)
		conn.Close()
		n.sendConnectDone(false)
		return
	}
	if err := downMsg.SendMessage(); err != nil {
		slog.Error("send UUID to child failed", "error", err)
		conn.Close()
		n.sendConnectDone(false)
		return
	}

	n.childrenMu.Lock()
	n.children[childUUID] = &ChildConn{UUID: childUUID, Conn: conn}
	n.childrenMu.Unlock()

	go n.relayChildToUpstream(conn, childUUID)
	n.sendConnectDone(true)

	slog.Info("connected to child", "uuid", childUUID, "addr", req.Addr)
}

func (n *Node) sendConnectDone(ok bool) {
	res := &protocol.ConnectDone{OK: 0}
	if ok {
		res.OK = 1
	}

	header := &protocol.Header{
		Version:     1,
		Sender:      n.UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.CONNECTDONE,
		RouteLen:    uint32(len(protocol.TEMP_ROUTE)),
		Route:       protocol.TEMP_ROUTE,
	}

	sMessage := protocol.NewUpMsg(n.ParentConn, n.Options.Secret, n.UUID)
	if err := protocol.ConstructMessage(sMessage, header, res, false); err != nil {
		slog.Error("send connect done failed", "error", err)
		return
	}
	if err := sMessage.SendMessage(); err != nil {
		slog.Error("send connect done failed", "error", err)
	}
}
