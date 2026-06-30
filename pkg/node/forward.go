package node

import (
	"io"
	"log/slog"
	"net"

	"mcp-stowaway/internal/utils"
	"mcp-stowaway/pkg/protocol"
)

// handleForward starts a TCP port forward on this node.
func (n *Node) handleForward(req *protocol.ForwardStart) {
	listenAddr, _, err := utils.CheckIPPort(req.ListenAddr)
	if err != nil {
		slog.Error("invalid forward listen address", "error", err)
		n.sendForwardReady(false)
		return
	}

	targetAddr, _, err := utils.CheckIPPort(req.TargetAddr)
	if err != nil {
		slog.Error("invalid forward target address", "error", err)
		n.sendForwardReady(false)
		return
	}

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		slog.Error("forward listen failed", "error", err)
		n.sendForwardReady(false)
		return
	}
	n.sendForwardReady(true)

	slog.Info("forward listening", "listen", listenAddr, "target", targetAddr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			slog.Warn("forward accept failed", "error", err)
			continue
		}
		go n.handleForwardConn(conn, targetAddr)
	}
}

func (n *Node) sendForwardReady(ok bool) {
	res := &protocol.ForwardReady{OK: 0}
	if ok {
		res.OK = 1
	}

	header := &protocol.Header{
		Version:     1,
		Sender:      n.UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.FORWARDREADY,
		RouteLen:    uint32(len(protocol.TEMP_ROUTE)),
		Route:       protocol.TEMP_ROUTE,
	}

	sMessage := protocol.NewUpMsg(n.ParentConn, n.Options.Secret, n.UUID)
	if err := protocol.ConstructMessage(sMessage, header, res, false); err != nil {
		slog.Error("send forward ready failed", "error", err)
		return
	}
	if err := sMessage.SendMessage(); err != nil {
		slog.Error("send forward ready failed", "error", err)
	}
}

func (n *Node) handleForwardConn(local net.Conn, targetAddr string) {
	defer local.Close()

	remote, err := net.Dial("tcp", targetAddr)
	if err != nil {
		slog.Warn("forward target connect failed", "target", targetAddr, "error", err)
		return
	}
	defer remote.Close()

	done := make(chan struct{}, 2)
	go func() {
		io.Copy(remote, local)
		done <- struct{}{}
	}()
	go func() {
		io.Copy(local, remote)
		done <- struct{}{}
	}()
	<-done
}
