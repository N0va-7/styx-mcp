package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"styx-mcp/pkg/controller"
	"styx-mcp/pkg/protocol"
	"styx-mcp/pkg/tasks"
	"styx-mcp/pkg/topology"
)

// Server wraps an MCP server around a controller.
type Server struct {
	controller *controller.Controller
	mcpserver  *server.MCPServer
}

// NewServer creates a new MCP server.
func NewServer(ctrl *controller.Controller) *Server {
	s := &Server{
		controller: ctrl,
		mcpserver:  server.NewMCPServer("styx-mcp", "0.3.0"),
	}
	s.registerTools()
	return s
}

type mcpLogWriter struct {
	mu     sync.Mutex
	w      io.Writer
	log    *os.File
	prefix string
}

func (m *mcpLogWriter) Write(p []byte) (int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.log != nil {
		fmt.Fprintf(m.log, "[%s] %s\n", m.prefix, string(p))
	}
	return m.w.Write(p)
}

type mcpLogReader struct {
	r      io.Reader
	log    *os.File
	prefix string
}

func (m *mcpLogReader) Read(p []byte) (int, error) {
	n, err := m.r.Read(p)
	if n > 0 && m.log != nil {
		fmt.Fprintf(m.log, "[%s] %s\n", m.prefix, string(p[:n]))
	}
	return n, err
}

// Serve starts the MCP server on stdio.
// Set STYX_MCP_LOG to a file path to capture raw MCP stdio (may contain secrets).
// When unset, stdio is not logged.
func (s *Server) Serve() error {
	logPath := strings.TrimSpace(os.Getenv("STYX_MCP_LOG"))
	if logPath == "" {
		return server.ServeStdio(s.mcpserver)
	}

	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		slog.Warn("failed to open mcp log file", "path", logPath, "error", err)
		return server.ServeStdio(s.mcpserver)
	}
	defer logFile.Close()

	stdioServer := server.NewStdioServer(s.mcpserver)
	stdinLog := &mcpLogReader{r: os.Stdin, log: logFile, prefix: "IN"}
	stdoutLog := &mcpLogWriter{w: os.Stdout, log: logFile, prefix: "OUT"}
	return stdioServer.Listen(context.Background(), stdinLog, stdoutLog)
}

func (s *Server) registerTools() {
	s.mcpserver.AddTool(mcp.NewTool("list_nodes"), s.handleListNodes)

	s.mcpserver.AddTool(mcp.NewTool("get_node_detail",
		mcp.WithNumber("node_id", mcp.Required(), mcp.Description("Numeric node ID")),
	), s.handleGetNodeDetail)

	s.mcpserver.AddTool(mcp.NewTool("add_node_memo",
		mcp.WithNumber("node_id", mcp.Required(), mcp.Description("Numeric node ID")),
		mcp.WithString("memo", mcp.Required(), mcp.Description("Memo text")),
	), s.handleAddNodeMemo)

	s.mcpserver.AddTool(mcp.NewTool("delete_node_memo",
		mcp.WithNumber("node_id", mcp.Required(), mcp.Description("Numeric node ID")),
	), s.handleDeleteNodeMemo)

	s.mcpserver.AddTool(mcp.NewTool("start_listener",
		mcp.WithNumber("node_id", mcp.Required(), mcp.Description("Numeric node ID to listen on")),
		mcp.WithString("address", mcp.Required(), mcp.Description("Listen address [ip]:<port>")),
	), s.handleStartListener)

	s.mcpserver.AddTool(mcp.NewTool("connect_node",
		mcp.WithNumber("node_id", mcp.Required(), mcp.Description("Numeric node ID to connect from")),
		mcp.WithString("address", mcp.Required(), mcp.Description("Target address <ip>:<port>")),
	), s.handleConnectNode)

	s.mcpserver.AddTool(mcp.NewTool("start_socks",
		mcp.WithNumber("node_id", mcp.Required(), mcp.Description("Numeric node ID to tunnel SOCKS through")),
		mcp.WithString("address", mcp.Required(), mcp.Description("Local listen address on controller [ip]:<port>")),
	), s.handleStartSocks)

	s.mcpserver.AddTool(mcp.NewTool("start_forward",
		mcp.WithNumber("node_id", mcp.Required(), mcp.Description("Numeric node ID to forward on")),
		mcp.WithString("listen_address", mcp.Required(), mcp.Description("Listen address [ip]:<port>")),
		mcp.WithString("target_address", mcp.Required(), mcp.Description("Target address <ip>:<port>")),
	), s.handleStartForward)

	s.mcpserver.AddTool(mcp.NewTool("start_backward",
		mcp.WithNumber("node_id", mcp.Required(), mcp.Description("Numeric node ID to reverse forward through")),
		mcp.WithString("local_address", mcp.Required(), mcp.Description("Local listen address [ip]:<port>")),
		mcp.WithString("target_address", mcp.Required(), mcp.Description("Target address <ip>:<port>")),
	), s.handleStartBackward)

	s.mcpserver.AddTool(mcp.NewTool("upload_file",
		mcp.WithNumber("node_id", mcp.Required(), mcp.Description("Numeric node ID to upload to")),
		mcp.WithString("local_path", mcp.Required(), mcp.Description("Local file path")),
		mcp.WithString("remote_path", mcp.Required(), mcp.Description("Remote destination path (absolute or relative; no '..')")),
	), s.handleUploadFile)

	// Named like sibling tools (upload_file / start_*) — some MCP clients
	// silently drop names such as download_file / run_command / exec.
	s.mcpserver.AddTool(mcp.NewTool("pull_file",
		mcp.WithNumber("node_id", mcp.Required(), mcp.Description("Numeric node ID to pull from")),
		mcp.WithString("remote_path", mcp.Required(), mcp.Description("Remote file path on the node")),
		mcp.WithString("local_path", mcp.Required(), mcp.Description("Local destination path on controller")),
	), s.handleDownloadFile)

	s.mcpserver.AddTool(mcp.NewTool("start_cmd",
		mcp.WithNumber("node_id", mcp.Required(), mcp.Description("Numeric node ID")),
		mcp.WithString("line", mcp.Required(), mcp.Description("Non-interactive one-shot line for sh -c")),
		mcp.WithNumber("timeout_sec", mcp.Description("Timeout in seconds (default 30, max 120)")),
		mcp.WithString("workdir", mcp.Description("Optional working directory on the node")),
	), s.handleExec)

	s.mcpserver.AddTool(mcp.NewTool("get_task_status",
		mcp.WithString("task_id", mcp.Required(), mcp.Description("Task ID")),
	), s.handleGetTaskStatus)

	s.mcpserver.AddTool(mcp.NewTool("shutdown_node",
		mcp.WithNumber("node_id", mcp.Required(), mcp.Description("Numeric node ID to shutdown")),
	), s.handleShutdownNode)
}

func (s *Server) success(data map[string]interface{}) *mcp.CallToolResult {
	b, _ := json.Marshal(data)
	return mcp.NewToolResultText(string(b))
}

func (s *Server) failure(msg string) *mcp.CallToolResult {
	b, _ := json.Marshal(map[string]interface{}{"success": false, "error": msg})
	return mcp.NewToolResultText(string(b))
}

func getArgs(request mcp.CallToolRequest) (map[string]interface{}, error) {
	args, ok := request.Params.Arguments.(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid arguments")
	}
	return args, nil
}

func getNodeID(args map[string]interface{}) (int, error) {
	raw, ok := args["node_id"]
	if !ok {
		return 0, fmt.Errorf("node_id is required")
	}

	switch v := raw.(type) {
	case float64:
		return int(v), nil
	case int:
		return v, nil
	case string:
		return strconv.Atoi(v)
	default:
		return 0, fmt.Errorf("node_id must be a number")
	}
}

func (s *Server) handleListNodes(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	s.controller.Topology.TaskChan <- &topology.Task{Mode: topology.Calculate}
	<-s.controller.Topology.ResultChan

	entries := s.controller.ListNodes()
	nodes := make([]map[string]interface{}, 0, len(entries))
	for _, e := range entries {
		node := e.Node
		nodes = append(nodes, map[string]interface{}{
			"id":       e.ID,
			"uuid":     node.UUID,
			"ip":       node.CurrentIP,
			"hostname": node.CurrentHostname,
			"user":     node.CurrentUser,
			"memo":     node.Memo,
			"children": node.ChildrenUUID,
		})
	}

	return s.success(map[string]interface{}{"success": true, "nodes": nodes}), nil
}

func (s *Server) handleGetNodeDetail(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := getArgs(request)
	if err != nil {
		return s.failure(err.Error()), nil
	}

	nodeID, err := getNodeID(args)
	if err != nil {
		return s.failure(err.Error()), nil
	}

	node, ok := s.controller.GetNodeInfo(nodeID)
	if !ok {
		return s.failure(fmt.Sprintf("node %d not found", nodeID)), nil
	}

	return s.success(map[string]interface{}{
		"success":  true,
		"id":       nodeID,
		"uuid":     node.UUID,
		"ip":       node.CurrentIP,
		"hostname": node.CurrentHostname,
		"user":     node.CurrentUser,
		"memo":     node.Memo,
		"children": node.ChildrenUUID,
	}), nil
}

func (s *Server) handleAddNodeMemo(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := getArgs(request)
	if err != nil {
		return s.failure(err.Error()), nil
	}

	nodeID, err := getNodeID(args)
	if err != nil {
		return s.failure(err.Error()), nil
	}

	memo, ok := args["memo"].(string)
	if !ok {
		return s.failure("memo must be a string"), nil
	}

	s.controller.Topology.TaskChan <- &topology.Task{Mode: topology.GetUUID, UUIDNum: nodeID}
	res := <-s.controller.Topology.ResultChan
	if res.UUID == "" {
		return s.failure(fmt.Sprintf("node %d not found", nodeID)), nil
	}

	s.controller.Topology.TaskChan <- &topology.Task{
		Mode: topology.UpdateMemo,
		UUID: res.UUID,
		Memo: memo,
	}
	<-s.controller.Topology.ResultChan

	header := &protocol.Header{
		Version:     1,
		Sender:      protocol.ControllerUUID,
		Accepter:    res.UUID,
		MessageType: protocol.MYMEMO,
	}
	memoMess := &protocol.MyMemo{MemoLen: uint64(len(memo)), Memo: memo}
	if err := s.controller.SendToNode(res.UUID, header, memoMess); err != nil {
		return s.failure(fmt.Sprintf("memo updated locally but failed to notify node: %v", err)), nil
	}

	return s.success(map[string]interface{}{"success": true, "node_id": nodeID, "memo": memo}), nil
}

func (s *Server) handleDeleteNodeMemo(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := getArgs(request)
	if err != nil {
		return s.failure(err.Error()), nil
	}

	nodeID, err := getNodeID(args)
	if err != nil {
		return s.failure(err.Error()), nil
	}

	s.controller.Topology.TaskChan <- &topology.Task{Mode: topology.GetUUID, UUIDNum: nodeID}
	res := <-s.controller.Topology.ResultChan
	if res.UUID == "" {
		return s.failure(fmt.Sprintf("node %d not found", nodeID)), nil
	}

	s.controller.Topology.TaskChan <- &topology.Task{
		Mode: topology.UpdateMemo,
		UUID: res.UUID,
		Memo: "",
	}
	<-s.controller.Topology.ResultChan

	header := &protocol.Header{
		Version:     1,
		Sender:      protocol.ControllerUUID,
		Accepter:    res.UUID,
		MessageType: protocol.MYMEMO,
	}
	memoMess := &protocol.MyMemo{MemoLen: 0, Memo: ""}
	if err := s.controller.SendToNode(res.UUID, header, memoMess); err != nil {
		return s.failure(fmt.Sprintf("memo deleted locally but failed to notify node: %v", err)), nil
	}

	return s.success(map[string]interface{}{"success": true, "node_id": nodeID}), nil
}

func (s *Server) handleStartListener(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := getArgs(request)
	if err != nil {
		return s.failure(err.Error()), nil
	}

	nodeID, err := getNodeID(args)
	if err != nil {
		return s.failure(err.Error()), nil
	}

	address, ok := args["address"].(string)
	if !ok {
		return s.failure("address must be a string"), nil
	}

	s.controller.Topology.TaskChan <- &topology.Task{Mode: topology.GetUUID, UUIDNum: nodeID}
	res := <-s.controller.Topology.ResultChan
	if res.UUID == "" {
		return s.failure(fmt.Sprintf("node %d not found", nodeID)), nil
	}

	task := s.controller.TaskManager.Create("start_listener")

	go func() {
		s.controller.TaskManager.UpdateStatus(task.ID, tasks.Running)
		header := &protocol.Header{
			Version:     1,
			Sender:      protocol.ControllerUUID,
			Accepter:    res.UUID,
			MessageType: protocol.LISTENREQ,
		}
		req := &protocol.ListenReq{
			Method:  1,
			AddrLen: uint64(len(address)),
			Addr:    address,
		}
		if err := s.controller.SendToNode(res.UUID, header, req); err != nil {
			s.controller.TaskManager.SetError(task.ID, err)
			return
		}
		s.controller.TaskManager.SetResult(task.ID, map[string]interface{}{
			"node_id": nodeID,
			"address": address,
		})
	}()

	return s.success(map[string]interface{}{"success": true, "task_id": task.ID}), nil
}

func (s *Server) handleConnectNode(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := getArgs(request)
	if err != nil {
		return s.failure(err.Error()), nil
	}

	nodeID, err := getNodeID(args)
	if err != nil {
		return s.failure(err.Error()), nil
	}

	address, ok := args["address"].(string)
	if !ok {
		return s.failure("address must be a string"), nil
	}

	s.controller.Topology.TaskChan <- &topology.Task{Mode: topology.GetUUID, UUIDNum: nodeID}
	res := <-s.controller.Topology.ResultChan
	if res.UUID == "" {
		return s.failure(fmt.Sprintf("node %d not found", nodeID)), nil
	}

	task := s.controller.TaskManager.Create("connect_node")

	go func() {
		s.controller.TaskManager.UpdateStatus(task.ID, tasks.Running)
		header := &protocol.Header{
			Version:     1,
			Sender:      protocol.ControllerUUID,
			Accepter:    res.UUID,
			MessageType: protocol.CONNECTSTART,
		}
		req := &protocol.ConnectStart{
			AddrLen: uint16(len(address)),
			Addr:    address,
		}
		if err := s.controller.SendToNode(res.UUID, header, req); err != nil {
			s.controller.TaskManager.SetError(task.ID, err)
			return
		}
		s.controller.TaskManager.SetResult(task.ID, map[string]interface{}{
			"node_id": nodeID,
			"address": address,
		})
	}()

	return s.success(map[string]interface{}{"success": true, "task_id": task.ID}), nil
}

func (s *Server) handleStartSocks(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := getArgs(request)
	if err != nil {
		return s.failure(err.Error()), nil
	}

	nodeID, err := getNodeID(args)
	if err != nil {
		return s.failure(err.Error()), nil
	}

	address, ok := args["address"].(string)
	if !ok {
		return s.failure("address must be a string"), nil
	}

	s.controller.Topology.TaskChan <- &topology.Task{Mode: topology.GetUUID, UUIDNum: nodeID}
	res := <-s.controller.Topology.ResultChan
	if res.UUID == "" {
		return s.failure(fmt.Sprintf("node %d not found", nodeID)), nil
	}

	task := s.controller.TaskManager.Create("start_socks")

	go func() {
		s.controller.TaskManager.UpdateStatus(task.ID, tasks.Running)
		if err := s.controller.StartSocks(res.UUID, address); err != nil {
			s.controller.TaskManager.SetError(task.ID, err)
			return
		}
		s.controller.TaskManager.SetResult(task.ID, map[string]interface{}{
			"node_id": nodeID,
			"address": address,
			"note":    "SOCKS5 listens on controller; traffic exits via the selected node",
		})
	}()

	return s.success(map[string]interface{}{"success": true, "task_id": task.ID}), nil
}

func (s *Server) handleStartForward(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := getArgs(request)
	if err != nil {
		return s.failure(err.Error()), nil
	}

	nodeID, err := getNodeID(args)
	if err != nil {
		return s.failure(err.Error()), nil
	}

	listenAddress, ok := args["listen_address"].(string)
	if !ok {
		return s.failure("listen_address must be a string"), nil
	}

	targetAddress, ok := args["target_address"].(string)
	if !ok {
		return s.failure("target_address must be a string"), nil
	}

	s.controller.Topology.TaskChan <- &topology.Task{Mode: topology.GetUUID, UUIDNum: nodeID}
	res := <-s.controller.Topology.ResultChan
	if res.UUID == "" {
		return s.failure(fmt.Sprintf("node %d not found", nodeID)), nil
	}

	task := s.controller.TaskManager.Create("start_forward")

	go func() {
		s.controller.TaskManager.UpdateStatus(task.ID, tasks.Running)
		header := &protocol.Header{
			Version:     1,
			Sender:      protocol.ControllerUUID,
			Accepter:    res.UUID,
			MessageType: protocol.FORWARDSTART,
		}
		req := &protocol.ForwardStart{
			ListenAddrLen: uint16(len(listenAddress)),
			ListenAddr:    listenAddress,
			TargetAddrLen: uint16(len(targetAddress)),
			TargetAddr:    targetAddress,
		}
		if err := s.controller.SendToNode(res.UUID, header, req); err != nil {
			s.controller.TaskManager.SetError(task.ID, err)
			return
		}
		s.controller.TaskManager.SetResult(task.ID, map[string]interface{}{
			"node_id":        nodeID,
			"listen_address": listenAddress,
			"target_address": targetAddress,
		})
	}()

	return s.success(map[string]interface{}{"success": true, "task_id": task.ID}), nil
}

func (s *Server) handleStartBackward(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := getArgs(request)
	if err != nil {
		return s.failure(err.Error()), nil
	}

	nodeID, err := getNodeID(args)
	if err != nil {
		return s.failure(err.Error()), nil
	}

	localAddress, ok := args["local_address"].(string)
	if !ok {
		return s.failure("local_address must be a string"), nil
	}

	targetAddress, ok := args["target_address"].(string)
	if !ok {
		return s.failure("target_address must be a string"), nil
	}

	s.controller.Topology.TaskChan <- &topology.Task{Mode: topology.GetUUID, UUIDNum: nodeID}
	res := <-s.controller.Topology.ResultChan
	if res.UUID == "" {
		return s.failure(fmt.Sprintf("node %d not found", nodeID)), nil
	}

	task := s.controller.TaskManager.Create("start_backward")

	go func() {
		s.controller.TaskManager.UpdateStatus(task.ID, tasks.Running)
		if err := s.controller.StartBackward(res.UUID, localAddress, targetAddress); err != nil {
			s.controller.TaskManager.SetError(task.ID, err)
			return
		}
		s.controller.TaskManager.SetResult(task.ID, map[string]interface{}{
			"node_id":        nodeID,
			"local_address":  localAddress,
			"target_address": targetAddress,
		})
	}()

	return s.success(map[string]interface{}{"success": true, "task_id": task.ID}), nil
}

func (s *Server) handleUploadFile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := getArgs(request)
	if err != nil {
		return s.failure(err.Error()), nil
	}

	nodeID, err := getNodeID(args)
	if err != nil {
		return s.failure(err.Error()), nil
	}

	localPath, ok := args["local_path"].(string)
	if !ok {
		return s.failure("local_path must be a string"), nil
	}

	remotePath, ok := args["remote_path"].(string)
	if !ok {
		return s.failure("remote_path must be a string"), nil
	}

	data, err := os.ReadFile(localPath)
	if err != nil {
		return s.failure(fmt.Sprintf("read local file failed: %v", err)), nil
	}
	const maxUpload = 32 << 20
	if len(data) > maxUpload {
		return s.failure(fmt.Sprintf("local file too large: %d bytes (max %d)", len(data), maxUpload)), nil
	}

	s.controller.Topology.TaskChan <- &topology.Task{Mode: topology.GetUUID, UUIDNum: nodeID}
	res := <-s.controller.Topology.ResultChan
	if res.UUID == "" {
		return s.failure(fmt.Sprintf("node %d not found", nodeID)), nil
	}

	task := s.controller.TaskManager.Create("upload_file")

	go func() {
		s.controller.TaskManager.UpdateStatus(task.ID, tasks.Running)

		// Send FILESTATREQ.
		statHeader := &protocol.Header{
			Version:     1,
			Sender:      protocol.ControllerUUID,
			Accepter:    res.UUID,
			MessageType: protocol.FILESTATREQ,
		}
		statReq := &protocol.FileStatReq{
			FilenameLen: uint32(len(remotePath)),
			Filename:    remotePath,
			FileSize:    uint64(len(data)),
			SliceNum:    1,
		}
		if err := s.controller.SendToNode(res.UUID, statHeader, statReq); err != nil {
			s.controller.TaskManager.SetError(task.ID, err)
			return
		}

		// Send single FILEDATA slice.
		dataHeader := &protocol.Header{
			Version:     1,
			Sender:      protocol.ControllerUUID,
			Accepter:    res.UUID,
			MessageType: protocol.FILEDATA,
		}
		dataReq := &protocol.FileData{
			DataLen: uint64(len(data)),
			Data:    data,
		}
		if err := s.controller.SendToNode(res.UUID, dataHeader, dataReq); err != nil {
			s.controller.TaskManager.SetError(task.ID, err)
			return
		}

		s.controller.TaskManager.SetResult(task.ID, map[string]interface{}{
			"node_id":    nodeID,
			"local_path": localPath,
			"remote_path": remotePath,
			"bytes":      len(data),
		})
	}()

	return s.success(map[string]interface{}{"success": true, "task_id": task.ID}), nil
}

func (s *Server) handleDownloadFile(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := getArgs(request)
	if err != nil {
		return s.failure(err.Error()), nil
	}

	nodeID, err := getNodeID(args)
	if err != nil {
		return s.failure(err.Error()), nil
	}

	remotePath, ok := args["remote_path"].(string)
	if !ok || remotePath == "" {
		return s.failure("remote_path must be a non-empty string"), nil
	}
	localPath, ok := args["local_path"].(string)
	if !ok || localPath == "" {
		return s.failure("local_path must be a non-empty string"), nil
	}

	s.controller.Topology.TaskChan <- &topology.Task{Mode: topology.GetUUID, UUIDNum: nodeID}
	res := <-s.controller.Topology.ResultChan
	if res.UUID == "" {
		return s.failure(fmt.Sprintf("node %d not found", nodeID)), nil
	}

	task := s.controller.TaskManager.Create("pull_file")
	go func() {
		s.controller.TaskManager.UpdateStatus(task.ID, tasks.Running)
		if err := s.controller.StartDownload(res.UUID, task.ID, remotePath, localPath); err != nil {
			s.controller.TaskManager.SetError(task.ID, err)
		}
	}()

	return s.success(map[string]interface{}{"success": true, "task_id": task.ID}), nil
}

func (s *Server) handleExec(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := getArgs(request)
	if err != nil {
		return s.failure(err.Error()), nil
	}

	nodeID, err := getNodeID(args)
	if err != nil {
		return s.failure(err.Error()), nil
	}

	command, _ := args["line"].(string)
	if command == "" {
		command, _ = args["command"].(string) // legacy alias
	}
	if command == "" {
		return s.failure("line must be a non-empty string"), nil
	}

	timeoutSec := uint32(30)
	if v, ok := args["timeout_sec"].(float64); ok {
		if v > 0 {
			timeoutSec = uint32(v)
		}
		if timeoutSec > 120 {
			timeoutSec = 120
		}
	}

	workdir := ""
	if v, ok := args["workdir"].(string); ok {
		workdir = v
	}

	s.controller.Topology.TaskChan <- &topology.Task{Mode: topology.GetUUID, UUIDNum: nodeID}
	res := <-s.controller.Topology.ResultChan
	if res.UUID == "" {
		return s.failure(fmt.Sprintf("node %d not found", nodeID)), nil
	}

	task := s.controller.TaskManager.Create("start_cmd")
	go func() {
		s.controller.TaskManager.UpdateStatus(task.ID, tasks.Running)
		if err := s.controller.StartExec(res.UUID, task.ID, command, workdir, timeoutSec); err != nil {
			s.controller.TaskManager.SetError(task.ID, err)
		}
	}()

	return s.success(map[string]interface{}{"success": true, "task_id": task.ID}), nil
}

func (s *Server) handleGetTaskStatus(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := getArgs(request)
	if err != nil {
		return s.failure(err.Error()), nil
	}

	taskID, ok := args["task_id"].(string)
	if !ok {
		return s.failure("task_id must be a string"), nil
	}

	task, ok := s.controller.TaskManager.Get(taskID)
	if !ok {
		return s.failure(fmt.Sprintf("task %s not found", taskID)), nil
	}

	return s.success(map[string]interface{}{
		"success": true,
		"task":    task.ToMap(),
	}), nil
}

func (s *Server) handleShutdownNode(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	args, err := getArgs(request)
	if err != nil {
		return s.failure(err.Error()), nil
	}

	nodeID, err := getNodeID(args)
	if err != nil {
		return s.failure(err.Error()), nil
	}

	s.controller.Topology.TaskChan <- &topology.Task{Mode: topology.GetUUID, UUIDNum: nodeID}
	res := <-s.controller.Topology.ResultChan
	if res.UUID == "" {
		return s.failure(fmt.Sprintf("node %d not found", nodeID)), nil
	}

	header := &protocol.Header{
		Version:     1,
		Sender:      protocol.ControllerUUID,
		Accepter:    res.UUID,
		MessageType: protocol.SHUTDOWN,
	}
	shutdownMess := &protocol.Shutdown{OK: 1}
	if err := s.controller.SendToNode(res.UUID, header, shutdownMess); err != nil {
		return s.failure(fmt.Sprintf("failed to send shutdown: %v", err)), nil
	}

	return s.success(map[string]interface{}{"success": true, "node_id": nodeID}), nil
}
