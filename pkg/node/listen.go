package node

import (
	"log/slog"
	"net"

	"styx-mcp/internal/utils"
	"styx-mcp/pkg/protocol"
	"styx-mcp/pkg/share/preauth"
	"styx-mcp/pkg/transport"
)

// handleListen starts a listener on this node for child connections.
func (n *Node) handleListen(req *protocol.ListenReq) {
	addr, _, err := utils.CheckIPPort(req.Addr)
	if err != nil {
		slog.Error("invalid listen address", "error", err)
		n.sendListenRes(false)
		return
	}

	listener, err := net.Listen("tcp", addr)
	if err != nil {
		slog.Error("listen failed", "error", err)
		n.sendListenRes(false)
		return
	}
	n.sendListenRes(true)

	// Accept exactly one child connection per listen request.
	conn, err := listener.Accept()
	if err != nil {
		slog.Error("accept child failed", "error", err)
		listener.Close()
		return
	}
	listener.Close()

	n.handleChildConnection(conn)
}

func (n *Node) sendListenRes(ok bool) {
	res := &protocol.ListenRes{OK: 0}
	if ok {
		res.OK = 1
	}

	header := &protocol.Header{
		Version:     1,
		Sender:      n.UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.LISTENRES,
		RouteLen:    uint32(len(protocol.TEMP_ROUTE)),
		Route:       protocol.TEMP_ROUTE,
	}

	sMessage := protocol.NewUpMsg(n.ParentConn, n.Options.Secret, n.UUID)
	if err := protocol.ConstructMessage(sMessage, header, res, false); err != nil {
		slog.Error("send listen res failed", "error", err)
		return
	}
	if err := sMessage.SendMessage(); err != nil {
		slog.Error("send listen res failed", "error", err)
	}
}

// acceptChildren listens for incoming child connections in passive mode.
func (n *Node) acceptChildren() {
	// In V1, passive mode at startup is primarily used for the first connection to controller.
	// Additional listeners are started on-demand via handleListen.
	slog.Info("passive mode: child accept loop ready")
}

// handleChildConnection performs handshake with a new child and requests a UUID from upstream.
func (n *Node) handleChildConnection(conn net.Conn) {
	if n.Options.TlsEnable {
		config, err := transport.NewServerTLSConfig()
		if err != nil {
			conn.Close()
			return
		}
		conn = transport.WrapTLSServerConn(conn, config)
	}

	param := &protocol.NegParam{Conn: conn}
	proto := protocol.NewDownProto(param)
	if err := proto.SNegotiate(); err != nil {
		conn.Close()
		return
	}

	if err := preauth.PassivePreAuth(conn); err != nil {
		conn.Close()
		return
	}

	// The child treats us as admin during handshake.
	rMessage := protocol.NewDownMsg(conn, n.Options.Secret, protocol.ADMIN_UUID)
	fHeader, fMessage, err := protocol.DestructMessage(rMessage)
	if err != nil {
		slog.Error("read child HI failed", "error", err)
		conn.Close()
		return
	}

	if fHeader.MessageType != protocol.HI {
		conn.Close()
		return
	}

	mmess := fMessage.(*protocol.HIMess)
	if mmess.Greeting != "Shhh..." || mmess.IsAdmin != 0 {
		conn.Close()
		return
	}

	// Request UUID from upstream (controller).
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

	sMessage := protocol.NewUpMsg(n.ParentConn, n.Options.Secret, n.UUID)
	if err := protocol.ConstructMessage(sMessage, reqHeader, childReq, false); err != nil {
		conn.Close()
		return
	}
	if err := sMessage.SendMessage(); err != nil {
		conn.Close()
		return
	}

	// Wait for UUID assignment from controller.
	respHeader, respMessage, err := protocol.DestructMessage(protocol.NewUpMsg(n.ParentConn, n.Options.Secret, n.UUID))
	if err != nil || respHeader.MessageType != protocol.CHILDUUIDRES {
		conn.Close()
		return
	}

	uuidRes := respMessage.(*protocol.ChildUUIDRes)
	childUUID := uuidRes.UUID

	// Respond to child with HI.
	hiMess := &protocol.HIMess{
		GreetingLen: uint16(len("Keep silent")),
		Greeting:    "Keep silent",
		UUIDLen:     uint16(len(childUUID)),
		UUID:        childUUID,
		IsAdmin:     1,
		IsReconnect: 0,
	}
	hiHeader := &protocol.Header{
		Version:     1,
		Sender:      n.UUID,
		Accepter:    protocol.TEMP_UUID,
		MessageType: protocol.HI,
		RouteLen:    uint32(len(protocol.TEMP_ROUTE)),
		Route:       protocol.TEMP_ROUTE,
	}

	downMsg := protocol.NewDownMsg(conn, n.Options.Secret, n.UUID)
	if err := protocol.ConstructMessage(downMsg, hiHeader, hiMess, false); err != nil {
		conn.Close()
		return
	}
	if err := downMsg.SendMessage(); err != nil {
		conn.Close()
		return
	}

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

	downMsg = protocol.NewDownMsg(conn, n.Options.Secret, n.UUID)
	if err := protocol.ConstructMessage(downMsg, uuidHeader, uuidMess, false); err != nil {
		conn.Close()
		return
	}
	if err := downMsg.SendMessage(); err != nil {
		conn.Close()
		return
	}

	n.childrenMu.Lock()
	n.children[childUUID] = &ChildConn{UUID: childUUID, Conn: conn}
	n.childrenMu.Unlock()

	slog.Info("child connected", "uuid", childUUID, "ip", childIP)

	// Relay child messages upstream.
	go n.relayChildToUpstream(conn, childUUID)
}

// relayChildToUpstream forwards messages from a child to the upstream connection.
func (n *Node) relayChildToUpstream(conn net.Conn, childUUID string) {
	rMessage := protocol.NewDownMsg(conn, n.Options.Secret, n.UUID)

	for {
		header, message, err := protocol.DestructMessage(rMessage)
		if err != nil {
			slog.Warn("child read failed", "uuid", childUUID, "error", err)
			n.removeChild(childUUID)
			return
		}

		// Set sender to child UUID so controller can identify the source.
		header.Sender = childUUID
		if header.Accepter == protocol.ADMIN_UUID {
			header.Route = protocol.TEMP_ROUTE
			header.RouteLen = uint32(len(protocol.TEMP_ROUTE))
		}

		sMessage := protocol.NewUpMsg(n.ParentConn, n.Options.Secret, n.UUID)
		if err := protocol.ConstructMessage(sMessage, header, message, true); err != nil {
			slog.Error("relay upstream failed", "error", err)
			continue
		}
		if err := sMessage.SendMessage(); err != nil {
			slog.Error("relay upstream failed", "error", err)
		}
	}
}

func (n *Node) removeChild(uuid string) {
	n.childrenMu.Lock()
	defer n.childrenMu.Unlock()
	if child, ok := n.children[uuid]; ok {
		child.Conn.Close()
		child.Closed = true
		delete(n.children, uuid)
	}

	// Notify controller that the child is offline.
	off := &protocol.NodeOffline{
		UUIDLen: uint16(len(uuid)),
		UUID:    uuid,
	}
	header := &protocol.Header{
		Version:     1,
		Sender:      n.UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.NODEOFFLINE,
		RouteLen:    uint32(len(protocol.TEMP_ROUTE)),
		Route:       protocol.TEMP_ROUTE,
	}
	sMessage := protocol.NewUpMsg(n.ParentConn, n.Options.Secret, n.UUID)
	if err := protocol.ConstructMessage(sMessage, header, off, false); err != nil {
		slog.Error("notify child offline failed", "error", err)
		return
	}
	if err := sMessage.SendMessage(); err != nil {
		slog.Error("notify child offline failed", "error", err)
	}
}

