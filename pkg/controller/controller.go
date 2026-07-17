package controller

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"syscall"

	"styx-mcp/internal/utils"
	"styx-mcp/pkg/protocol"
	"styx-mcp/pkg/share/preauth"
	"styx-mcp/pkg/tasks"
	"styx-mcp/pkg/topology"
	"styx-mcp/pkg/transport"
)

// Controller is the MCP-facing control plane.
type Controller struct {
	Options     *Options
	Topology    *topology.Topology
	TaskManager *tasks.Manager

	conns   map[string]net.Conn
	connsMu sync.RWMutex

	listeners   map[string]bool
	listenersMu sync.RWMutex

	backwardListeners   map[string]*BackwardListener
	backwardListenersMu sync.RWMutex

	socksServices   map[string]*SocksService
	socksServicesMu sync.RWMutex

	pendingDownloads map[string]*pendingDownload
	downloadsMu      sync.Mutex

	// acks correlates agent RES/READY with MCP start_* waiters.
	acks   map[ackKey]chan bool
	acksMu sync.Mutex
}

// NewController creates a new controller.
func NewController(opt *Options) *Controller {
	return &Controller{
		Options:           opt,
		Topology:          topology.NewTopology(),
		TaskManager:       tasks.NewManager(),
		conns:             make(map[string]net.Conn),
		listeners:         make(map[string]bool),
		backwardListeners: make(map[string]*BackwardListener),
		socksServices:     make(map[string]*SocksService),
		pendingDownloads:  make(map[string]*pendingDownload),
		acks:              make(map[ackKey]chan bool),
	}
}

// Start brings up topology and agent networking.
// For passive mode it binds the listen socket before returning; bind failure is a hard error
// so MCP clients do not sit "healthy" with an empty topology.
func (c *Controller) Start() error {
	preauth.GenerateToken(c.Options.Secret)
	protocol.SetUpDownStream("raw", c.Options.Downstream)

	go c.Topology.Run()

	if c.Options.Mode() == "passive" {
		return c.startPassive()
	}
	return c.startActive()
}

// Run is kept for callers that expect a blocking API; it starts networking then blocks forever.
func (c *Controller) Run() error {
	if err := c.Start(); err != nil {
		return err
	}
	select {}
}

func (c *Controller) startPassive() error {
	listenAddr, _, err := utils.CheckIPPort(c.Options.Listen)
	if err != nil {
		return fmt.Errorf("invalid listen address %q: %w", c.Options.Listen, err)
	}

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return listenBindError(listenAddr, err)
	}

	slog.Info("controller listening", "addr", listenAddr)
	go c.acceptLoop(listener)
	return nil
}

func (c *Controller) startActive() error {
	conn, err := c.activeConnect()
	if err != nil {
		return err
	}
	go c.handleNode(conn, true)
	return nil
}

func (c *Controller) acceptLoop(listener net.Listener) {
	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			slog.Warn("accept failed", "error", err)
			continue
		}
		go c.handleIncoming(conn)
	}
}

// listenBindError formats listen failures with an actionable port-conflict hint.
func listenBindError(addr string, err error) error {
	if isAddrInUse(err) {
		return fmt.Errorf(
			"listen on %s: %w (address already in use — another styx-mcp/Cursor/Grok controller may hold this port; free it or set STYX_LISTEN)",
			addr, err,
		)
	}
	return fmt.Errorf("listen on %s: %w", addr, err)
}

func isAddrInUse(err error) bool {
	if errors.Is(err, syscall.EADDRINUSE) {
		return true
	}
	var opErr *net.OpError
	if errors.As(err, &opErr) {
		return errors.Is(opErr.Err, syscall.EADDRINUSE)
	}
	return false
}

func (c *Controller) handleIncoming(conn net.Conn) {
	if c.Options.TlsEnable {
		config, err := transport.NewServerTLSConfig(c.Options.Secret, c.Options.Domain)
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

	if err := preauth.PassivePreAuth(conn, c.Options.Secret); err != nil {
		conn.Close()
		return
	}

	rMessage := protocol.NewDownMsg(conn, c.Options.Secret, protocol.ControllerUUID)
	fHeader, fMessage, err := protocol.DestructMessage(rMessage)
	if err != nil {
		conn.Close()
		return
	}

	if fHeader.MessageType != protocol.HI {
		conn.Close()
		return
	}

	mmess, ok := asMsg[*protocol.HIMess](fMessage, "HI", conn.RemoteAddr().String())
	if !ok || mmess.Greeting != protocol.HelloFromAgent || mmess.IsAdmin != 0 {
		conn.Close()
		return
	}

	sMessage := protocol.NewDownMsg(conn, c.Options.Secret, protocol.ControllerUUID)
	hiMess := &protocol.HIMess{
		GreetingLen: uint16(len(protocol.HelloFromController)),
		Greeting:    protocol.HelloFromController,
		UUIDLen:     uint16(len(protocol.ControllerUUID)),
		UUID:        protocol.ControllerUUID,
		IsAdmin:     1,
		IsReconnect: 0,
	}
	hiHeader := &protocol.Header{
		Version:     1,
		Sender:      protocol.ControllerUUID,
		Accepter:    protocol.JoinUUID,
		MessageType: protocol.HI,
		RouteLen:    uint32(len(protocol.NoRoute)),
		Route:       protocol.NoRoute,
	}
	if err := protocol.ConstructMessage(sMessage, hiHeader, hiMess, false); err != nil {
		conn.Close()
		return
	}
	if err := sMessage.SendMessage(); err != nil {
		conn.Close()
		return
	}

	uuid := utils.GenerateUUID()
	if uuid == "" {
		conn.Close()
		return
	}

	uuidMess := &protocol.UUIDMess{UUIDLen: uint16(len(uuid)), UUID: uuid}
	uuidHeader := &protocol.Header{
		Version:     1,
		Sender:      protocol.ControllerUUID,
		Accepter:    protocol.JoinUUID,
		MessageType: protocol.UUID,
		RouteLen:    uint32(len(protocol.NoRoute)),
		Route:       protocol.NoRoute,
	}
	sMessage = protocol.NewDownMsg(conn, c.Options.Secret, protocol.ControllerUUID)
	if err := protocol.ConstructMessage(sMessage, uuidHeader, uuidMess, false); err != nil {
		conn.Close()
		return
	}
	if err := sMessage.SendMessage(); err != nil {
		conn.Close()
		return
	}

	c.handleNode(conn, true)
}

func (c *Controller) activeConnect() (net.Conn, error) {
	hiMess := &protocol.HIMess{
		GreetingLen: uint16(len(protocol.HelloFromAgent)),
		Greeting:    protocol.HelloFromAgent,
		UUIDLen:     uint16(len(protocol.ControllerUUID)),
		UUID:        protocol.ControllerUUID,
		IsAdmin:     1,
		IsReconnect: 0,
	}
	header := &protocol.Header{
		Version:     1,
		Sender:      protocol.ControllerUUID,
		Accepter:    protocol.JoinUUID,
		MessageType: protocol.HI,
		RouteLen:    uint32(len(protocol.NoRoute)),
		Route:       protocol.NoRoute,
	}

	for {
		conn, err := net.Dial("tcp", c.Options.Connect)
		if err != nil {
			return nil, err
		}

		if c.Options.TlsEnable {
			config, err := transport.NewClientTLSConfig(c.Options.Secret, c.Options.Domain)
			if err != nil {
				conn.Close()
				continue
			}
			conn = transport.WrapTLSClientConn(conn, config)
		}

		param := &protocol.NegParam{Conn: conn, Domain: c.Options.Domain}
		proto := protocol.NewDownProto(param)
		if err := proto.CNegotiate(); err != nil {
			conn.Close()
			continue
		}

		if err := preauth.ActivePreAuth(conn, c.Options.Secret); err != nil {
			conn.Close()
			return nil, err
		}

		sMessage := protocol.NewDownMsg(conn, c.Options.Secret, protocol.ControllerUUID)
		if err := protocol.ConstructMessage(sMessage, header, hiMess, false); err != nil {
			conn.Close()
			return nil, err
		}
		if err := sMessage.SendMessage(); err != nil {
			conn.Close()
			return nil, err
		}

		rMessage := protocol.NewDownMsg(conn, c.Options.Secret, protocol.ControllerUUID)
		fHeader, fMessage, err := protocol.DestructMessage(rMessage)
		if err != nil {
			conn.Close()
			continue
		}

		if fHeader.MessageType == protocol.HI {
			mmess := fMessage.(*protocol.HIMess)
			if mmess.Greeting == protocol.HelloFromController && mmess.IsAdmin == 0 {
				return conn, nil
			}
		}

		conn.Close()
		return nil, fmt.Errorf("invalid node response")
	}
}

func (c *Controller) handleNode(conn net.Conn, isFirst bool) {
	// Wait for MYINFO to know the UUID, then start the read loop.
	rMessage := protocol.NewDownMsg(conn, c.Options.Secret, protocol.ControllerUUID)
	fHeader, fMessage, err := protocol.DestructMessage(rMessage)
	if err != nil {
		conn.Close()
		return
	}

	if fHeader.MessageType != protocol.MYINFO {
		conn.Close()
		return
	}

	info := fMessage.(*protocol.MyInfo)
	uuid := info.UUID

	c.connsMu.Lock()
	c.conns[uuid] = conn
	c.connsMu.Unlock()

	// Register in topology.
	parentUUID := protocol.ControllerUUID
	if !isFirst {
		// Non-first nodes should be registered by their parent via CHILDUUIDREQ.
		// If they connect directly, we still add them under admin for safety.
		parentUUID = protocol.ControllerUUID
	}

	c.Topology.Do(&topology.Task{
		Mode:       topology.AddNode,
		Target:     topology.NewNode(uuid, conn.RemoteAddr().String()),
		ParentUUID: parentUUID,
		IsFirst:    isFirst,
	})

	c.Topology.Do(&topology.Task{
		Mode:       topology.UpdateDetail,
		UUID:       uuid,
		UserName:   info.Username,
		HostName:   info.Hostname,
		Memo:       info.Memo,
		LocalAddrs: utils.SplitAddrs(info.LocalAddrs),
	})

	c.Topology.Do(&topology.Task{Mode: topology.Calculate})

	slog.Info("node online", "uuid", uuid, "peer", conn.RemoteAddr().String(), "local_addrs", info.LocalAddrs)

	// Start read loop.
	c.readLoop(uuid, conn)
}

func (c *Controller) readLoop(uuid string, conn net.Conn) {
	rMessage := protocol.NewDownMsg(conn, c.Options.Secret, protocol.ControllerUUID)

	for {
		header, message, err := protocol.DestructMessage(rMessage)
		if err != nil {
			if err == io.EOF {
				slog.Info("node disconnected", "uuid", uuid)
			} else {
				slog.Error("read from node failed", "uuid", uuid, "error", err)
			}
			c.nodeOffline(uuid)
			return
		}

		c.handleMessage(uuid, header, message)
	}
}

func (c *Controller) handleMessage(uuid string, header *protocol.Header, message interface{}) {
	switch header.MessageType {
	case protocol.MYINFO:
		info, ok := asMsg[*protocol.MyInfo](message, "MYINFO", uuid)
		if !ok {
			return
		}
		c.Topology.Do(&topology.Task{
			Mode:       topology.UpdateDetail,
			UUID:       info.UUID,
			UserName:   info.Username,
			HostName:   info.Hostname,
			Memo:       info.Memo,
			LocalAddrs: utils.SplitAddrs(info.LocalAddrs),
		})

	case protocol.CHILDUUIDREQ:
		req, ok := asMsg[*protocol.ChildUUIDReq](message, "CHILDUUIDREQ", uuid)
		if !ok {
			return
		}
		// The relaying parent overwrites Sender with the actual child's UUID.
		c.handleChildUUIDReq(header.Sender, req)

	case protocol.NODEOFFLINE:
		off, ok := asMsg[*protocol.NodeOffline](message, "NODEOFFLINE", uuid)
		if !ok {
			return
		}
		c.nodeOffline(off.UUID)

	case protocol.LISTENRES:
		res, ok := asMsg[*protocol.ListenRes](message, "LISTENRES", uuid)
		if !ok {
			return
		}
		slog.Info("listen response", "uuid", uuid, "ok", res.OK)
		c.signalAck(uuid, AckListen, res.OK == 1)

	case protocol.CONNECTDONE:
		res, ok := asMsg[*protocol.ConnectDone](message, "CONNECTDONE", uuid)
		if !ok {
			return
		}
		slog.Info("connect done", "uuid", uuid, "ok", res.OK)
		c.signalAck(uuid, AckConnect, res.OK == 1)

	case protocol.SOCKSREADY:
		res, ok := asMsg[*protocol.SocksReady](message, "SOCKSREADY", uuid)
		if !ok {
			return
		}
		slog.Info("socks ready", "uuid", uuid, "ok", res.OK)
		c.handleSocksReady(uuid, res.OK == 1)

	case protocol.SOCKSTCPDATA:
		data, ok := asMsg[*protocol.SocksTCPData](message, "SOCKSTCPDATA", uuid)
		if !ok {
			return
		}
		c.handleSocksData(uuid, data)

	case protocol.SOCKSTCPACK:
		ack, ok := asMsg[*protocol.SocksTCPAck](message, "SOCKSTCPACK", uuid)
		if !ok {
			return
		}
		c.handleSocksAck(uuid, ack)

	case protocol.SOCKSTCPFIN:
		fin, ok := asMsg[*protocol.SocksTCPFin](message, "SOCKSTCPFIN", uuid)
		if !ok {
			return
		}
		c.handleSocksFin(uuid, fin)

	case protocol.FORWARDREADY:
		res, ok := asMsg[*protocol.ForwardReady](message, "FORWARDREADY", uuid)
		if !ok {
			return
		}
		slog.Info("forward ready", "uuid", uuid, "ok", res.OK)
		c.signalAck(uuid, AckForward, res.OK == 1)

	case protocol.BACKWARDREADY:
		res, ok := asMsg[*protocol.BackwardReady](message, "BACKWARDREADY", uuid)
		if !ok {
			return
		}
		c.handleBackwardReady(uuid, res)

	case protocol.BACKWARDDATA:
		data, ok := asMsg[*protocol.BackwardData](message, "BACKWARDDATA", uuid)
		if !ok {
			return
		}
		c.handleBackwardData(uuid, data)

	case protocol.BACKWARDFIN:
		fin, ok := asMsg[*protocol.BackWardFin](message, "BACKWARDFIN", uuid)
		if !ok {
			return
		}
		c.handleBackwardFin(uuid, fin)

	case protocol.FILESTATRES:
		// upload ack from node (legacy no-op)

	case protocol.FILESTATREQ:
		req, ok := asMsg[*protocol.FileStatReq](message, "FILESTATREQ", uuid)
		if !ok {
			return
		}
		c.handleDownloadFileStat(uuid, req)

	case protocol.FILEDATA:
		data, ok := asMsg[*protocol.FileData](message, "FILEDATA", uuid)
		if !ok {
			return
		}
		c.handleDownloadFileData(uuid, data)

	case protocol.FILEDOWNRES:
		res, ok := asMsg[*protocol.FileDownRes](message, "FILEDOWNRES", uuid)
		if !ok {
			return
		}
		c.handleFileDownRes(uuid, res)

	case protocol.EXECRES:
		res, ok := asMsg[*protocol.ExecRes](message, "EXECRES", uuid)
		if !ok {
			return
		}
		c.handleExecRes(res)

	case protocol.SCANPROG:
		prog, ok := asMsg[*protocol.ScanProg](message, "SCANPROG", uuid)
		if !ok {
			return
		}
		c.handleScanProg(prog)

	case protocol.SCANRES:
		res, ok := asMsg[*protocol.ScanRes](message, "SCANRES", uuid)
		if !ok {
			return
		}
		c.handleScanRes(res, uuid)

	case protocol.HEARTBEAT:
		// no-op

	default:
		slog.Warn("unhandled message type", "type", header.MessageType, "from", uuid)
	}
}

// asMsg type-asserts a decoded payload; logs and returns false instead of panicking.
func asMsg[T any](message interface{}, kind, from string) (T, bool) {
	var zero T
	v, ok := message.(T)
	if !ok {
		slog.Warn("unexpected payload type", "kind", kind, "from", from, "got", fmt.Sprintf("%T", message))
		return zero, false
	}
	return v, true
}

func (c *Controller) handleChildUUIDReq(parentUUID string, req *protocol.ChildUUIDReq) {
	childUUID := utils.GenerateUUID()
	if childUUID == "" {
		slog.Error("failed to generate child UUID")
		return
	}

	res := &protocol.ChildUUIDRes{UUIDLen: uint16(len(childUUID)), UUID: childUUID}
	header := &protocol.Header{
		Version:     1,
		Sender:      protocol.ControllerUUID,
		Accepter:    parentUUID,
		MessageType: protocol.CHILDUUIDRES,
	}

	if err := c.SendToNode(parentUUID, header, res); err != nil {
		slog.Error("send child uuid res failed", "error", err)
		return
	}

	// Register child in topology under parent.
	c.Topology.Do(&topology.Task{
		Mode:       topology.AddNode,
		Target:     topology.NewNode(childUUID, req.IP),
		ParentUUID: parentUUID,
	})

	c.Topology.Do(&topology.Task{Mode: topology.Calculate})

	slog.Info("child uuid assigned", "parent", parentUUID, "child", childUUID)
}

func (c *Controller) nodeOffline(uuid string) {
	c.connsMu.Lock()
	if conn, ok := c.conns[uuid]; ok {
		conn.Close()
		delete(c.conns, uuid)
	}
	c.connsMu.Unlock()

	res := c.Topology.Do(&topology.Task{Mode: topology.DelNode, UUID: uuid})

	c.Topology.Do(&topology.Task{Mode: topology.Calculate})

	slog.Info("node offline", "uuid", uuid, "affected", res.AllNodes)
}

// SendToNode sends a message to a specific node by UUID.
func (c *Controller) SendToNode(uuid string, header *protocol.Header, payload interface{}) error {
	// Determine the directly-connected peer that can reach the target.
	routeUUID, route, err := c.resolveRoute(uuid)
	if err != nil {
		return err
	}

	c.connsMu.RLock()
	conn, ok := c.conns[routeUUID]
	c.connsMu.RUnlock()

	if !ok {
		return fmt.Errorf("node not found: %s", uuid)
	}

	if header.Route == "" {
		header.Route = route
		header.RouteLen = uint32(len(route))
	}

	sMessage := protocol.NewDownMsg(conn, c.Options.Secret, protocol.ControllerUUID)
	if err := protocol.ConstructMessage(sMessage, header, payload, false); err != nil {
		return err
	}
	return sMessage.SendMessage()
}

// resolveRoute returns the directly-connected UUID to send through and the
// remaining route segment to reach the target UUID.
func (c *Controller) resolveRoute(uuid string) (string, string, error) {
	res := c.Topology.Do(&topology.Task{Mode: topology.GetFirstHop, UUID: uuid})

	if res.UUID == "" {
		return "", "", fmt.Errorf("node not found: %s", uuid)
	}

	return res.UUID, res.Route, nil
}

// GetNodeInfo returns node details for MCP tools.
func (c *Controller) GetNodeInfo(uuidNum int) (*topology.Node, bool) {
	res := c.Topology.Do(&topology.Task{Mode: topology.GetUUID, UUIDNum: uuidNum})
	if res.UUID == "" {
		return nil, false
	}

	nodeRes := c.Topology.Do(&topology.Task{Mode: topology.GetNode, UUID: res.UUID})
	if !nodeRes.IsExist || nodeRes.Node == nil {
		return nil, false
	}

	return nodeRes.Node, true
}

// ListNodes returns all online nodes with their numeric IDs (sparse-ID safe).
func (c *Controller) ListNodes() []topology.NodeEntry {
	res := c.Topology.Do(&topology.Task{Mode: topology.ListAll})
	if res.Nodes == nil {
		return nil
	}
	return res.Nodes
}

func (c *Controller) StartBackward(nodeUUID, localAddr, targetAddr string) error {
	bl, err := NewBackwardListener(c, nodeUUID, localAddr, targetAddr)
	if err != nil {
		return err
	}

	c.backwardListenersMu.Lock()
	c.backwardListeners[localAddr] = bl
	c.backwardListenersMu.Unlock()

	go bl.Run()
	return nil
}

func (c *Controller) handleSocksReady(uuid string, ok bool) {
	c.socksServicesMu.RLock()
	svc, found := c.socksServices[uuid]
	c.socksServicesMu.RUnlock()
	if !found {
		slog.Warn("socks ready for unknown service", "uuid", uuid)
		return
	}
	svc.handleReady(ok)
}

func (c *Controller) handleSocksData(uuid string, data *protocol.SocksTCPData) {
	c.socksServicesMu.RLock()
	svc, found := c.socksServices[uuid]
	c.socksServicesMu.RUnlock()
	if !found {
		slog.Warn("socks data for unknown service", "uuid", uuid)
		return
	}
	svc.handleData(data.Seq, data.Data)
}

func (c *Controller) handleSocksAck(uuid string, ack *protocol.SocksTCPAck) {
	c.socksServicesMu.RLock()
	svc, found := c.socksServices[uuid]
	c.socksServicesMu.RUnlock()
	if !found {
		return
	}
	svc.handleAck(ack.Seq, ack.Credit)
}

func (c *Controller) handleSocksFin(uuid string, fin *protocol.SocksTCPFin) {
	c.socksServicesMu.RLock()
	svc, found := c.socksServices[uuid]
	c.socksServicesMu.RUnlock()
	if !found {
		return
	}
	svc.handleFin(fin.Seq)
}

func (c *Controller) handleBackwardReady(uuid string, res *protocol.BackwardReady) {
	if bl := c.findBackwardListener(uuid, res.Seq); bl != nil {
		bl.handleReady(res.Seq, res.OK == 1)
	}
}

func (c *Controller) handleBackwardData(uuid string, data *protocol.BackwardData) {
	if bl := c.findBackwardListener(uuid, data.Seq); bl != nil {
		bl.handleData(data.Seq, data.Data)
	}
}

func (c *Controller) handleBackwardFin(uuid string, fin *protocol.BackWardFin) {
	if bl := c.findBackwardListener(uuid, fin.Seq); bl != nil {
		bl.handleFin(fin.Seq)
	}
}

// findBackwardListener returns the reverse-forward listener for a node that owns seq.
func (c *Controller) findBackwardListener(nodeUUID string, seq uint64) *BackwardListener {
	c.backwardListenersMu.RLock()
	defer c.backwardListenersMu.RUnlock()
	for _, bl := range c.backwardListeners {
		if bl.nodeUUID != nodeUUID {
			continue
		}
		bl.mu.RLock()
		_, ok := bl.seqConn[seq]
		bl.mu.RUnlock()
		if ok {
			return bl
		}
	}
	return nil
}

