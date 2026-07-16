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

	// childUUIDWait delivers CHILDUUIDRES without racing the upstream reader.
	childUUIDMu   sync.Mutex
	childUUIDWait chan *protocol.ChildUUIDRes

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
		UUID:        protocol.JoinUUID,
		Options:     opt,
		children:    make(map[string]*ChildConn),
		childrenMsg: make(chan *ChildrenMessage, 256),
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
		GreetingLen: uint16(len(protocol.HelloFromAgent)),
		Greeting:    protocol.HelloFromAgent,
		UUIDLen:     uint16(len(protocol.JoinUUID)),
		UUID:        protocol.JoinUUID,
		IsAdmin:     0,
		IsReconnect: 0,
	}

	header := &protocol.Header{
		Version:     1,
		Sender:      protocol.JoinUUID,
		Accepter:    protocol.ControllerUUID,
		MessageType: protocol.HI,
		RouteLen:    uint32(len(protocol.NoRoute)),
		Route:       protocol.NoRoute,
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
			config, err := transport.NewClientTLSConfig(n.Options.Secret, n.Options.Domain)
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

		sMessage := protocol.NewUpMsg(conn, n.Options.Secret, protocol.JoinUUID)
		if err := protocol.ConstructMessage(sMessage, header, hiMess, false); err != nil {
			conn.Close()
			return nil, err
		}
		if err := sMessage.SendMessage(); err != nil {
			conn.Close()
			return nil, err
		}

		rMessage := protocol.NewUpMsg(conn, n.Options.Secret, protocol.JoinUUID)
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
			if mmess.Greeting == protocol.HelloFromController && mmess.IsAdmin == 1 {
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
		GreetingLen: uint16(len(protocol.HelloFromController)),
		Greeting:    protocol.HelloFromController,
		UUIDLen:     uint16(len(protocol.JoinUUID)),
		UUID:        protocol.JoinUUID,
		IsAdmin:     0,
		IsReconnect: 0,
	}

	header := &protocol.Header{
		Version:     1,
		Sender:      protocol.JoinUUID,
		Accepter:    protocol.ControllerUUID,
		MessageType: protocol.HI,
		RouteLen:    uint32(len(protocol.NoRoute)),
		Route:       protocol.NoRoute,
	}

	for {
		conn, err := listener.Accept()
		if err != nil {
			slog.Warn("accept failed", "error", err)
			continue
		}
		slog.Info("accepted connection", "remote", conn.RemoteAddr())

		if n.Options.TlsEnable {
			config, err := transport.NewServerTLSConfig(n.Options.Secret, n.Options.Domain)
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

		rMessage := protocol.NewDownMsg(conn, n.Options.Secret, protocol.JoinUUID)
		fHeader, fMessage, err := protocol.DestructMessage(rMessage)
		if err != nil {
			slog.Error("read HI failed", "error", err)
			conn.Close()
			continue
		}
		slog.Info("read HI", "type", fHeader.MessageType)

		if fHeader.MessageType == protocol.HI {
			mmess := fMessage.(*protocol.HIMess)
			if mmess.Greeting == protocol.HelloFromAgent && mmess.IsAdmin == 1 {
				sMessage := protocol.NewUpMsg(conn, n.Options.Secret, protocol.JoinUUID)
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
	rMessage := protocol.NewDownMsg(conn, n.Options.Secret, protocol.JoinUUID)
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
		Accepter:    protocol.ControllerUUID,
		MessageType: protocol.MYINFO,
		RouteLen:    uint32(len(protocol.NoRoute)),
		Route:       protocol.NoRoute,
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

		if header.Accepter == n.UUID || header.Accepter == protocol.JoinUUID {
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
		if header.Accepter == protocol.ControllerUUID || header.Accepter != n.UUID {
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
		mmess, ok := asMsg[*protocol.MyMemo](message, "MYMEMO")
		if !ok {
			return
		}
		n.Memo = mmess.Memo
	case protocol.LISTENREQ:
		req, ok := asMsg[*protocol.ListenReq](message, "LISTENREQ")
		if !ok {
			return
		}
		n.handleListen(req) // non-blocking (spawns goroutine)
	case protocol.CONNECTSTART:
		req, ok := asMsg[*protocol.ConnectStart](message, "CONNECTSTART")
		if !ok {
			return
		}
		n.handleConnect(req) // non-blocking (spawns goroutine)
	case protocol.CHILDUUIDRES:
		res, ok := asMsg[*protocol.ChildUUIDRes](message, "CHILDUUIDRES")
		if !ok {
			return
		}
		n.deliverChildUUID(res)
	case protocol.SOCKSSTART:
		req, ok := asMsg[*protocol.SocksStart](message, "SOCKSSTART")
		if !ok {
			return
		}
		n.SocksManager.handleSocksStart(req)
	case protocol.SOCKSTCPDATA:
		req, ok := asMsg[*protocol.SocksTCPData](message, "SOCKSTCPDATA")
		if !ok {
			return
		}
		n.SocksManager.handleSocksData(req)
	case protocol.SOCKSTCPACK:
		req, ok := asMsg[*protocol.SocksTCPAck](message, "SOCKSTCPACK")
		if !ok {
			return
		}
		n.SocksManager.handleSocksAck(req)
	case protocol.SOCKSTCPFIN:
		req, ok := asMsg[*protocol.SocksTCPFin](message, "SOCKSTCPFIN")
		if !ok {
			return
		}
		n.SocksManager.handleSocksFin(req)
	case protocol.FORWARDSTART:
		req, ok := asMsg[*protocol.ForwardStart](message, "FORWARDSTART")
		if !ok {
			return
		}
		n.handleForward(req) // non-blocking (spawns goroutine)
	case protocol.BACKWARDSTART:
		req, ok := asMsg[*protocol.BackwardStart](message, "BACKWARDSTART")
		if !ok {
			return
		}
		n.BackwardManager.handleBackwardStart(req)
	case protocol.BACKWARDDATA:
		req, ok := asMsg[*protocol.BackwardData](message, "BACKWARDDATA")
		if !ok {
			return
		}
		n.BackwardManager.handleBackwardData(req)
	case protocol.BACKWARDFIN:
		req, ok := asMsg[*protocol.BackWardFin](message, "BACKWARDFIN")
		if !ok {
			return
		}
		n.BackwardManager.handleBackwardFin(req)
	case protocol.FILESTATREQ:
		req, ok := asMsg[*protocol.FileStatReq](message, "FILESTATREQ")
		if !ok {
			return
		}
		n.handleFileStat(req)
	case protocol.FILEDATA:
		req, ok := asMsg[*protocol.FileData](message, "FILEDATA")
		if !ok {
			return
		}
		n.handleFileData(req)
	case protocol.FILEDOWNREQ:
		req, ok := asMsg[*protocol.FileDownReq](message, "FILEDOWNREQ")
		if !ok {
			return
		}
		n.handleFileDownReq(req)
	case protocol.EXECREQ:
		req, ok := asMsg[*protocol.ExecReq](message, "EXECREQ")
		if !ok {
			return
		}
		n.handleExecReq(req)
	case protocol.SHUTDOWN:
		n.ParentConn.Close()
		slog.Info("shutdown received")
	case protocol.HEARTBEAT:
		// no-op
	default:
		slog.Warn("unhandled message type", "type", header.MessageType)
	}
}

// asMsg type-asserts a decoded payload; logs and returns false instead of panicking.
func asMsg[T any](message interface{}, kind string) (T, bool) {
	var zero T
	v, ok := message.(T)
	if !ok {
		slog.Warn("unexpected payload type", "kind", kind, "got", fmt.Sprintf("%T", message))
		return zero, false
	}
	return v, true
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

// deliverChildUUID routes a controller CHILDUUIDRES to the waiting connect/listen goroutine.
func (n *Node) deliverChildUUID(res *protocol.ChildUUIDRes) {
	n.childUUIDMu.Lock()
	ch := n.childUUIDWait
	n.childUUIDMu.Unlock()
	if ch == nil {
		slog.Warn("unexpected CHILDUUIDRES (no waiter)")
		return
	}
	select {
	case ch <- res:
	default:
		slog.Warn("CHILDUUIDRES dropped (waiter busy)")
	}
}

// requestChildUUID asks the controller for a UUID for a new child and waits
// for CHILDUUIDRES on the main upstream path (must not read ParentConn here).
func (n *Node) requestChildUUID(childIP string) (string, error) {
	wait := make(chan *protocol.ChildUUIDRes, 1)

	n.childUUIDMu.Lock()
	if n.childUUIDWait != nil {
		n.childUUIDMu.Unlock()
		return "", fmt.Errorf("another child UUID request is already in progress")
	}
	n.childUUIDWait = wait
	n.childUUIDMu.Unlock()

	defer func() {
		n.childUUIDMu.Lock()
		n.childUUIDWait = nil
		n.childUUIDMu.Unlock()
	}()

	childReq := &protocol.ChildUUIDReq{
		ParentUUIDLen: uint16(len(n.UUID)),
		ParentUUID:    n.UUID,
		IPLen:         uint16(len(childIP)),
		IP:            childIP,
	}
	reqHeader := &protocol.Header{
		Version:     1,
		Sender:      n.UUID,
		Accepter:    protocol.ControllerUUID,
		MessageType: protocol.CHILDUUIDREQ,
		RouteLen:    uint32(len(protocol.NoRoute)),
		Route:       protocol.NoRoute,
	}
	if err := n.sendToParent(reqHeader, childReq); err != nil {
		return "", fmt.Errorf("send child uuid req: %w", err)
	}

	select {
	case res := <-wait:
		if res == nil || res.UUID == "" {
			return "", fmt.Errorf("empty child UUID from controller")
		}
		return res.UUID, nil
	case <-time.After(30 * time.Second):
		return "", fmt.Errorf("timeout waiting for child UUID")
	}
}

// GetTLSConfig returns a tls.Config for the node's secret/domain.
func GetTLSConfig(secret, domain string) (*tls.Config, error) {
	return transport.NewClientTLSConfig(secret, domain)
}
