package node

import (
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"

	"styx-mcp/internal/utils"
	"styx-mcp/pkg/protocol"
	"styx-mcp/pkg/share/preauth"
	"styx-mcp/pkg/transport"
)

// Node is the runtime state of an agent node.
type Node struct {
	UUID       string
	Memo       string
	Options    *Options
	ParentConn net.Conn

	children    map[string]*ChildConn
	childrenMu  sync.RWMutex
	childrenMsg chan *ChildrenMessage

	BackwardManager *BackwardManager
	SocksManager    *SocksManager

	pendingFile *PendingFile
}

// PendingFile tracks an in-progress file upload for this node.
type PendingFile struct {
	filename string
	fileSize uint64
	sliceNum uint64
}

// ChildConn holds a connection to a child node.
type ChildConn struct {
	UUID   string
	Conn   net.Conn
	Closed bool
}

// ChildrenMessage is a message destined for a child node.
type ChildrenMessage struct {
	Header  *protocol.Header
	Payload []byte
}

// NewNode creates a new node runtime.
func NewNode(opt *Options) *Node {
	n := &Node{
		UUID:        protocol.TEMP_UUID,
		Options:     opt,
		children:    make(map[string]*ChildConn),
		childrenMsg: make(chan *ChildrenMessage, 10),
		pendingFile: &PendingFile{},
	}
	n.BackwardManager = NewBackwardManager(n)
	n.SocksManager = NewSocksManager(n)
	return n
}

// Run starts the node.
func (n *Node) Run() error {
	conn, err := n.establishConnection()
	if err != nil {
		return fmt.Errorf("establish connection: %w", err)
	}

	n.ParentConn = conn
	n.sendMyInfo()

	// Start child message dispatcher.
	go n.dispatchChildrenMessages()

	// If passive mode, start accepting child connections.
	if n.Options.Mode() == "passive" {
		go n.acceptChildren()
	}

	// Main upstream message loop.
	n.handleUpstream()
	return nil
}

func (n *Node) establishConnection() (net.Conn, error) {
	preauth.GenerateToken(n.Options.Secret)
	protocol.SetUpDownStream(n.Options.Upstream, n.Options.Downstream)

	if n.Options.Mode() == "active" {
		return n.activeConnect()
	}
	return n.passiveAccept()
}

func (n *Node) activeConnect() (net.Conn, error) {
	hiMess := &protocol.HIMess{
		GreetingLen: uint16(len("Shhh...")),
		Greeting:    "Shhh...",
		UUIDLen:     uint16(len(protocol.TEMP_UUID)),
		UUID:        protocol.TEMP_UUID,
		IsAdmin:     0,
		IsReconnect: 0,
	}

	header := &protocol.Header{
		Version:     1,
		Sender:      protocol.TEMP_UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.HI,
		RouteLen:    uint32(len(protocol.TEMP_ROUTE)),
		Route:       protocol.TEMP_ROUTE,
	}

	for {
		conn, err := net.Dial("tcp", n.Options.Connect)
		if err != nil {
			if n.Options.Reconnect > 0 {
				slog.Warn("connect failed, retrying", "error", err, "interval", n.Options.Reconnect)
				time.Sleep(time.Duration(n.Options.Reconnect) * time.Second)
				continue
			}
			return nil, err
		}

		if n.Options.TlsEnable {
			config, err := transport.NewClientTLSConfig(n.Options.Domain)
			if err != nil {
				conn.Close()
				return nil, err
			}
			conn = transport.WrapTLSClientConn(conn, config)
		}

		param := &protocol.NegParam{Conn: conn, Domain: n.Options.Domain}
		proto := protocol.NewUpProto(param)
		if err := proto.CNegotiate(); err != nil {
			conn.Close()
			return nil, err
		}

		if err := preauth.ActivePreAuth(conn, n.Options.Secret); err != nil {
			conn.Close()
			return nil, err
		}

		sMessage := protocol.NewUpMsg(conn, n.Options.Secret, protocol.TEMP_UUID)
		if err := protocol.ConstructMessage(sMessage, header, hiMess, false); err != nil {
			conn.Close()
			return nil, err
		}
		if err := sMessage.SendMessage(); err != nil {
			conn.Close()
			return nil, err
		}

		rMessage := protocol.NewUpMsg(conn, n.Options.Secret, protocol.TEMP_UUID)
		fHeader, fMessage, err := protocol.DestructMessage(rMessage)
		if err != nil {
			conn.Close()
			if n.Options.Reconnect > 0 {
				time.Sleep(time.Duration(n.Options.Reconnect) * time.Second)
				continue
			}
			return nil, err
		}

		if fHeader.MessageType == protocol.HI {
			mmess := fMessage.(*protocol.HIMess)
			if mmess.Greeting == "Keep silent" && mmess.IsAdmin == 1 {
				n.UUID = n.achieveUUID(conn)
				if n.UUID == "" {
					conn.Close()
					return nil, fmt.Errorf("failed to obtain UUID")
				}
				return conn, nil
			}
		}

		conn.Close()
		return nil, fmt.Errorf("invalid upstream response")
	}
}

func (n *Node) passiveAccept() (net.Conn, error) {
	listenAddr, _, err := utils.CheckIPPort(n.Options.Listen)
	if err != nil {
		return nil, err
	}

	slog.Info("passive accept waiting", "addr", listenAddr)
	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return nil, err
	}
	defer listener.Close()

	hiMess := &protocol.HIMess{
		GreetingLen: uint16(len("Keep silent")),
		Greeting:    "Keep silent",
		UUIDLen:     uint16(len(protocol.TEMP_UUID)),
		UUID:        protocol.TEMP_UUID,
		IsAdmin:     0,
		IsReconnect: 0,
	}

	header := &protocol.Header{
		Version:     1,
		Sender:      protocol.TEMP_UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.HI,
		RouteLen:    uint32(len(protocol.TEMP_ROUTE)),
		Route:       protocol.TEMP_ROUTE,
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			slog.Warn("accept failed", "error", err)
			continue
		}
		slog.Info("accepted connection", "remote", conn.RemoteAddr())

		if n.Options.TlsEnable {
			config, err := transport.NewServerTLSConfig()
			if err != nil {
				conn.Close()
				continue
			}
			conn = transport.WrapTLSServerConn(conn, config)
		}

		param := &protocol.NegParam{Conn: conn}
		proto := protocol.NewUpProto(param)
		if err := proto.SNegotiate(); err != nil {
			slog.Error("negotiate failed", "error", err)
			conn.Close()
			continue
		}

		if err := preauth.PassivePreAuth(conn, n.Options.Secret); err != nil {
			slog.Error("preauth failed", "error", err)
			conn.Close()
			continue
		}
		slog.Info("preauth success")

		rMessage := protocol.NewDownMsg(conn, n.Options.Secret, protocol.TEMP_UUID)
		fHeader, fMessage, err := protocol.DestructMessage(rMessage)
		if err != nil {
			slog.Error("read HI failed", "error", err)
			conn.Close()
			continue
		}
		slog.Info("read HI", "type", fHeader.MessageType)

		if fHeader.MessageType == protocol.HI {
			mmess := fMessage.(*protocol.HIMess)
			if mmess.Greeting == "Shhh..." && mmess.IsAdmin == 1 {
				sMessage := protocol.NewUpMsg(conn, n.Options.Secret, protocol.TEMP_UUID)
				if err := protocol.ConstructMessage(sMessage, header, hiMess, false); err != nil {
					conn.Close()
					continue
				}
				if err := sMessage.SendMessage(); err != nil {
					conn.Close()
					continue
				}
				slog.Info("sent HI response, waiting for UUID")
				n.UUID = n.achieveUUID(conn)
				if n.UUID == "" {
					slog.Error("achieve UUID failed")
					conn.Close()
					continue
				}
				slog.Info("got UUID", "uuid", n.UUID)
				return conn, nil
			}
		}

		conn.Close()
	}
}

func (n *Node) achieveUUID(conn net.Conn) string {
	rMessage := protocol.NewDownMsg(conn, n.Options.Secret, protocol.TEMP_UUID)
	fHeader, fMessage, err := protocol.DestructMessage(rMessage)
	if err != nil {
		slog.Error("achieveUUID read failed", "error", err)
		return ""
	}
	if fHeader.MessageType != protocol.UUID {
		slog.Error("achieveUUID unexpected type", "type", fHeader.MessageType)
		return ""
	}
	mmess, ok := fMessage.(*protocol.UUIDMess)
	if !ok {
		slog.Error("achieveUUID payload is not UUIDMess", "type", fmt.Sprintf("%T", fMessage))
		return ""
	}
	return mmess.UUID
}

func (n *Node) sendMyInfo() {
	hostname, username := utils.GetSystemInfo()

	header := &protocol.Header{
		Version:     1,
		Sender:      n.UUID,
		Accepter:    protocol.ADMIN_UUID,
		MessageType: protocol.MYINFO,
		RouteLen:    uint32(len(protocol.TEMP_ROUTE)),
		Route:       protocol.TEMP_ROUTE,
	}

	info := &protocol.MyInfo{
		UUIDLen:     uint16(len(n.UUID)),
		UUID:        n.UUID,
		UsernameLen: uint64(len(username)),
		Username:    username,
		HostnameLen: uint64(len(hostname)),
		Hostname:    hostname,
		MemoLen:     uint64(len(n.Memo)),
		Memo:        n.Memo,
	}

	sMessage := protocol.NewUpMsg(n.ParentConn, n.Options.Secret, n.UUID)
	if err := protocol.ConstructMessage(sMessage, header, info, false); err != nil {
		slog.Error("send myinfo failed", "error", err)
		return
	}
	if err := sMessage.SendMessage(); err != nil {
		slog.Error("send myinfo failed", "error", err)
	}
}

func (n *Node) handleUpstream() {
	rMessage := protocol.NewUpMsg(n.ParentConn, n.Options.Secret, n.UUID)

	for {
		header, message, err := protocol.DestructMessage(rMessage)
		if err != nil {
			if err == io.EOF {
				slog.Info("upstream closed")
			} else {
				slog.Error("upstream read failed", "error", err)
			}
			n.handleUpstreamOffline()
			return
		}

		slog.Info("upstream message", "type", header.MessageType, "accepter", header.Accepter, "sender", header.Sender, "me", n.UUID)

		if header.Accepter == n.UUID || header.Accepter == protocol.TEMP_UUID {
			n.handleLocalMessage(header, message)
			continue
		}

		// Message is for one of our descendants: dispatch downward.
		nexthop := header.Accepter
		if header.Route != "" {
			routes := strings.Split(header.Route, ":")
			nexthop = routes[0]
		}
		if n.isChild(nexthop) {
			payload, ok := message.([]byte)
			if !ok {
				slog.Warn("downward payload is not []byte", "type", header.MessageType)
				continue
			}
			select {
			case n.childrenMsg <- &ChildrenMessage{Header: header, Payload: payload}:
			default:
				slog.Warn("children message queue full", "uuid", nexthop)
			}
			continue
		}

		// Message is for admin or another ancestor: relay upstream.
		if header.Accepter == protocol.ADMIN_UUID || header.Accepter != n.UUID {
			payload, ok := message.([]byte)
			if !ok {
				slog.Warn("relay payload is not []byte", "type", header.MessageType)
				continue
			}
			n.relayUpstream(header, payload)
			continue
		}

		n.handleLocalMessage(header, message)
	}
}

func (n *Node) isChild(uuid string) bool {
	n.childrenMu.RLock()
	defer n.childrenMu.RUnlock()
	_, ok := n.children[uuid]
	return ok
}

func (n *Node) handleLocalMessage(header *protocol.Header, message interface{}) {
	switch header.MessageType {
	case protocol.MYMEMO:
		mmess := message.(*protocol.MyMemo)
		n.Memo = mmess.Memo
	case protocol.LISTENREQ:
		req := message.(*protocol.ListenReq)
		n.handleListen(req)
	case protocol.CONNECTSTART:
		req := message.(*protocol.ConnectStart)
		n.handleConnect(req)
	case protocol.SOCKSSTART:
		req := message.(*protocol.SocksStart)
		n.SocksManager.handleSocksStart(req)
	case protocol.SOCKSTCPDATA:
		req := message.(*protocol.SocksTCPData)
		n.SocksManager.handleSocksData(req)
	case protocol.SOCKSTCPFIN:
		req := message.(*protocol.SocksTCPFin)
		n.SocksManager.handleSocksFin(req)
	case protocol.FORWARDSTART:
		req := message.(*protocol.ForwardStart)
		n.handleForward(req)
	case protocol.BACKWARDSTART:
		req := message.(*protocol.BackwardStart)
		n.BackwardManager.handleBackwardStart(req)
	case protocol.BACKWARDDATA:
		req := message.(*protocol.BackwardData)
		n.BackwardManager.handleBackwardData(req)
	case protocol.BACKWARDFIN:
		req := message.(*protocol.BackWardFin)
		n.BackwardManager.handleBackwardFin(req)
	case protocol.FILESTATREQ:
		req := message.(*protocol.FileStatReq)
		n.handleFileStat(req)
	case protocol.FILEDATA:
		req := message.(*protocol.FileData)
		n.handleFileData(req)
	case protocol.SHUTDOWN:
		n.ParentConn.Close()
		slog.Info("shutdown received")
	case protocol.HEARTBEAT:
		// no-op
	default:
		slog.Warn("unhandled message type", "type", header.MessageType)
	}
}

func (n *Node) dispatchChildrenMessages() {
	for msg := range n.childrenMsg {
		childUUID := changeRoute(msg.Header)

		n.childrenMu.RLock()
		child, ok := n.children[childUUID]
		n.childrenMu.RUnlock()

		if !ok {
			slog.Warn("unknown child route", "uuid", childUUID)
			continue
		}

		sMessage := protocol.NewDownMsg(child.Conn, n.Options.Secret, n.UUID)
		if err := protocol.ConstructMessage(sMessage, msg.Header, msg.Payload, true); err != nil {
			slog.Error("forward to child failed", "error", err)
			continue
		}
		if err := sMessage.SendMessage(); err != nil {
			slog.Error("forward to child failed", "error", err)
		}
	}
}

func (n *Node) relayUpstream(header *protocol.Header, payload []byte) {
	sMessage := protocol.NewUpMsg(n.ParentConn, n.Options.Secret, n.UUID)
	if err := protocol.ConstructMessage(sMessage, header, payload, true); err != nil {
		slog.Error("relay upstream failed", "error", err)
		return
	}
	if err := sMessage.SendMessage(); err != nil {
		slog.Error("relay upstream failed", "error", err)
	}
}

func (n *Node) handleUpstreamOffline() {
	// TODO: reconnect logic and notify children.
	slog.Warn("upstream offline")
}

func changeRoute(header *protocol.Header) string {
	routes := strings.Split(header.Route, ":")
	if len(routes) == 1 {
		header.Route = ""
		header.RouteLen = 0
		return routes[0]
	}
	header.Route = strings.Join(routes[1:], ":")
	header.RouteLen = uint32(len(header.Route))
	return routes[0]
}

// GetTLSConfig returns a tls.Config. This helper exists to avoid unused import errors.
func GetTLSConfig() (*tls.Config, error) {
	return transport.NewClientTLSConfig("")
}
