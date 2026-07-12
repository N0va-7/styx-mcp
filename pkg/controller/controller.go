package controller

import (
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

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
	}
}

// Run starts the controller.
func (c *Controller) Run() error {
	preauth.GenerateToken(c.Options.Secret)
	protocol.SetUpDownStream("raw", c.Options.Downstream)

	go c.Topology.Run()

	if c.Options.Mode() == "passive" {
		go c.acceptLoop()
	} else {
		conn, err := c.activeConnect()
		if err != nil {
			return err
		}
		go c.handleNode(conn, true)
	}

	// Keep main goroutine alive.
	select {}
}

func (c *Controller) acceptLoop() {
	listenAddr, _, err := utils.CheckIPPort(c.Options.Listen)
	if err != nil {
		slog.Error("invalid listen address", "error", err)
		return
	}

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		slog.Error("listen failed", "error", err)
		return
	}
	defer listener.Close()

	slog.Info("controller listening", "addr", listenAddr)

	for {
		conn, err := listener.Accept()
		if err != nil {
			slog.Warn("accept failed", "error", err)
			continue
		}
		go c.handleIncoming(conn)
	}
}

func (c *Controller) handleIncoming(conn net.Conn) {
	if c.Options.TlsEnable {
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

	if err := preauth.PassivePreAuth(conn, c.Options.Secret); err != nil {
		conn.Close()
		return
	}

	rMessage := protocol.NewDownMsg(conn, c.Options.Secret, protocol.ADMIN_UUID)
	fHeader, fMessage, err := protocol.DestructMessage(rMessage)
	if err != nil {
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

	sMessage := protocol.NewDownMsg(conn, c.Options.Secret, protocol.ADMIN_UUID)
	hiMess := &protocol.HIMess{
		GreetingLen: uint16(len("Keep silent")),
		Greeting:    "Keep silent",
		UUIDLen:     uint16(len(protocol.ADMIN_UUID)),
		UUID:        protocol.ADMIN_UUID,
		IsAdmin:     1,
		IsReconnect: 0,
	}
	hiHeader := &protocol.Header{
		Version:     1,
		Sender:      protocol.ADMIN_UUID,
		Accepter:    protocol.TEMP_UUID,
		MessageType: protocol.HI,
		RouteLen:    uint32(len(protocol.TEMP_ROUTE)),
		Route:       protocol.TEMP_ROUTE,
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
		Sender:      protocol.ADMIN_UUID,
		Accepter:    protocol.TEMP_UUID,
		MessageType: protocol.UUID,
		RouteLen:    uint32(len(protocol.TEMP_ROUTE)),
		Route:       protocol.TEMP_ROUTE,
	}
	sMessage = protocol.NewDownMsg(conn, c.Options.Secret, protocol.ADMIN_UUID)
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
		GreetingLen: uint16(len("Shhh...")),
		Greeting:    "Shhh...",
		UUIDLen:     uint16(len(protocol.ADMIN_UUID)),
		UUID:        protocol.ADMIN_UUID,
		IsAdmin:     1,
		IsReconnect: 0,
	}
	header := &protocol.Header{
		Version:     1,
		Sender:      protocol.ADMIN_UUID,
		Accepter:    protocol.TEMP_UUID,
		MessageType: protocol.HI,
		RouteLen:    uint32(len(protocol.TEMP_ROUTE)),
		Route:       protocol.TEMP_ROUTE,
	}

	for {
		conn, err := net.Dial("tcp", c.Options.Connect)
		if err != nil {
			return nil, err
		}

		if c.Options.TlsEnable {
			config, err := transport.NewClientTLSConfig(c.Options.Domain)
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

		sMessage := protocol.NewDownMsg(conn, c.Options.Secret, protocol.ADMIN_UUID)
		if err := protocol.ConstructMessage(sMessage, header, hiMess, false); err != nil {
			conn.Close()
			return nil, err
		}
		if err := sMessage.SendMessage(); err != nil {
			conn.Close()
			return nil, err
		}

		rMessage := protocol.NewDownMsg(conn, c.Options.Secret, protocol.ADMIN_UUID)
		fHeader, fMessage, err := protocol.DestructMessage(rMessage)
		if err != nil {
			conn.Close()
			continue
		}

		if fHeader.MessageType == protocol.HI {
			mmess := fMessage.(*protocol.HIMess)
			if mmess.Greeting == "Keep silent" && mmess.IsAdmin == 0 {
				return conn, nil
			}
		}

		conn.Close()
		return nil, fmt.Errorf("invalid node response")
	}
}

func (c *Controller) handleNode(conn net.Conn, isFirst bool) {
	// Wait for MYINFO to know the UUID, then start the read loop.
	rMessage := protocol.NewDownMsg(conn, c.Options.Secret, protocol.ADMIN_UUID)
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
	parentUUID := protocol.ADMIN_UUID
	if !isFirst {
		// Non-first nodes should be registered by their parent via CHILDUUIDREQ.
		// If they connect directly, we still add them under admin for safety.
		parentUUID = protocol.ADMIN_UUID
	}

	c.Topology.TaskChan <- &topology.Task{
		Mode:       topology.AddNode,
		Target:     topology.NewNode(uuid, conn.RemoteAddr().String()),
		ParentUUID: parentUUID,
		IsFirst:    isFirst,
	}
	<-c.Topology.ResultChan

	c.Topology.TaskChan <- &topology.Task{
		Mode:     topology.UpdateDetail,
		UUID:     uuid,
		UserName: info.Username,
		HostName: info.Hostname,
		Memo:     info.Memo,
	}
	<-c.Topology.ResultChan

	c.Topology.TaskChan <- &topology.Task{Mode: topology.Calculate}
	<-c.Topology.ResultChan

	slog.Info("node online", "uuid", uuid, "ip", conn.RemoteAddr().String())

	// Start read loop.
	c.readLoop(uuid, conn)
}

func (c *Controller) readLoop(uuid string, conn net.Conn) {
	rMessage := protocol.NewDownMsg(conn, c.Options.Secret, protocol.ADMIN_UUID)

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
		info := message.(*protocol.MyInfo)
		c.Topology.TaskChan <- &topology.Task{
			Mode:     topology.UpdateDetail,
			UUID:     info.UUID,
			UserName: info.Username,
			HostName: info.Hostname,
			Memo:     info.Memo,
		}
		<-c.Topology.ResultChan

	case protocol.CHILDUUIDREQ:
		req := message.(*protocol.ChildUUIDReq)
		// The relaying parent overwrites Sender with the actual child's UUID.
		c.handleChildUUIDReq(header.Sender, req)

	case protocol.NODEOFFLINE:
		off := message.(*protocol.NodeOffline)
		c.nodeOffline(off.UUID)

	case protocol.LISTENRES:
		slog.Info("listen response", "uuid", uuid, "ok", message.(*protocol.ListenRes).OK)

	case protocol.CONNECTDONE:
		slog.Info("connect done", "uuid", uuid, "ok", message.(*protocol.ConnectDone).OK)

	case protocol.SOCKSREADY:
		res := message.(*protocol.SocksReady)
		slog.Info("socks ready", "uuid", uuid, "ok", res.OK)
		c.handleSocksReady(uuid, res.OK == 1)

	case protocol.SOCKSTCPDATA:
		data := message.(*protocol.SocksTCPData)
		c.handleSocksData(uuid, data)

	case protocol.SOCKSTCPACK:
		ack := message.(*protocol.SocksTCPAck)
		c.handleSocksAck(uuid, ack)

	case protocol.SOCKSTCPFIN:
		fin := message.(*protocol.SocksTCPFin)
		c.handleSocksFin(uuid, fin)

	case protocol.FORWARDREADY:
		slog.Info("forward ready", "uuid", uuid, "ok", message.(*protocol.ForwardReady).OK)

	case protocol.BACKWARDREADY:
		res := message.(*protocol.BackwardReady)
		c.handleBackwardReady(uuid, res)

	case protocol.BACKWARDDATA:
		data := message.(*protocol.BackwardData)
		c.handleBackwardData(uuid, data)

	case protocol.BACKWARDFIN:
		fin := message.(*protocol.BackWardFin)
		c.handleBackwardFin(uuid, fin)

	case protocol.FILESTATRES:
		// upload ack from node (legacy no-op)

	case protocol.FILESTATREQ:
		// download metadata from node
		req := message.(*protocol.FileStatReq)
		c.handleDownloadFileStat(uuid, req)

	case protocol.FILEDATA:
		data := message.(*protocol.FileData)
		c.handleDownloadFileData(uuid, data)

	case protocol.FILEDOWNRES:
		res := message.(*protocol.FileDownRes)
		c.handleFileDownRes(uuid, res)

	case protocol.EXECRES:
		res := message.(*protocol.ExecRes)
		c.handleExecRes(res)

	case protocol.HEARTBEAT:
		// no-op

	default:
		slog.Warn("unhandled message type", "type", header.MessageType, "from", uuid)
	}
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
		Sender:      protocol.ADMIN_UUID,
		Accepter:    parentUUID,
		MessageType: protocol.CHILDUUIDRES,
	}

	if err := c.SendToNode(parentUUID, header, res); err != nil {
		slog.Error("send child uuid res failed", "error", err)
		return
	}

	// Register child in topology under parent.
	c.Topology.TaskChan <- &topology.Task{
		Mode:       topology.AddNode,
		Target:     topology.NewNode(childUUID, req.IP),
		ParentUUID: parentUUID,
	}
	<-c.Topology.ResultChan

	c.Topology.TaskChan <- &topology.Task{Mode: topology.Calculate}
	<-c.Topology.ResultChan

	slog.Info("child uuid assigned", "parent", parentUUID, "child", childUUID)
}

func (c *Controller) nodeOffline(uuid string) {
	c.connsMu.Lock()
	if conn, ok := c.conns[uuid]; ok {
		conn.Close()
		delete(c.conns, uuid)
	}
	c.connsMu.Unlock()

	c.Topology.TaskChan <- &topology.Task{Mode: topology.DelNode, UUID: uuid}
	res := <-c.Topology.ResultChan

	c.Topology.TaskChan <- &topology.Task{Mode: topology.Calculate}
	<-c.Topology.ResultChan

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

	sMessage := protocol.NewDownMsg(conn, c.Options.Secret, protocol.ADMIN_UUID)
	if err := protocol.ConstructMessage(sMessage, header, payload, false); err != nil {
		return err
	}
	return sMessage.SendMessage()
}

// resolveRoute returns the directly-connected UUID to send through and the
// remaining route segment to reach the target UUID.
func (c *Controller) resolveRoute(uuid string) (string, string, error) {
	c.Topology.TaskChan <- &topology.Task{Mode: topology.GetFirstHop, UUID: uuid}
	res := <-c.Topology.ResultChan

	if res.UUID == "" {
		return "", "", fmt.Errorf("node not found: %s", uuid)
	}

	return res.UUID, res.Route, nil
}

// GetNodeInfo returns node details for MCP tools.
func (c *Controller) GetNodeInfo(uuidNum int) (*topology.Node, bool) {
	c.Topology.TaskChan <- &topology.Task{Mode: topology.GetUUID, UUIDNum: uuidNum}
	res := <-c.Topology.ResultChan
	if res.UUID == "" {
		return nil, false
	}

	c.Topology.TaskChan <- &topology.Task{Mode: topology.GetNode, UUID: res.UUID}
	nodeRes := <-c.Topology.ResultChan
	if !nodeRes.IsExist || nodeRes.Node == nil {
		return nil, false
	}

	return nodeRes.Node, true
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
	c.backwardListenersMu.RLock()
	defer c.backwardListenersMu.RUnlock()
	for _, bl := range c.backwardListeners {
		bl.handleReady(res.Seq, res.OK == 1)
	}
}

func (c *Controller) handleBackwardData(uuid string, data *protocol.BackwardData) {
	c.backwardListenersMu.RLock()
	defer c.backwardListenersMu.RUnlock()
	for _, bl := range c.backwardListeners {
		bl.handleData(data.Seq, data.Data)
	}
}

func (c *Controller) handleBackwardFin(uuid string, fin *protocol.BackWardFin) {
	c.backwardListenersMu.RLock()
	defer c.backwardListenersMu.RUnlock()
	for _, bl := range c.backwardListeners {
		bl.handleFin(fin.Seq)
	}
}

// unused avoids unused import errors.
var _ = tls.Config{}
var _ = time.Now
