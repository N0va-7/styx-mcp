package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"

	"mcp-stowaway/pkg/controller"
	"mcp-stowaway/pkg/protocol"
	"mcp-stowaway/pkg/tasks"
	"mcp-stowaway/pkg/topology"
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
		mcpserver:  server.NewMCPServer("mcp-stowaway", "0.1.0"),
	}
	s.registerTools()
	return s
}

// Serve starts the MCP server on stdio.
func (s *Server) Serve() error {
	return server.ServeStdio(s.mcpserver)
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
		mcp.WithNumber("node_id", mcp.Required(), mcp.Description("Numeric node ID to run SOCKS5 on")),
		mcp.WithString("address", mcp.Required(), mcp.Description("Listen address [ip]:<port>")),
	), s.handleStartSocks)

	s.mcpserver.AddTool(mcp.NewTool("upload_file",
		mcp.WithNumber("node_id", mcp.Required(), mcp.Description("Numeric node ID to upload to")),
		mcp.WithString("local_path", mcp.Required(), mcp.Description("Local file path")),
		mcp.WithString("remote_path", mcp.Required(), mcp.Description("Remote destination path")),
	), s.handleUploadFile)

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

	nodes := []map[string]interface{}{}
	for i := 0; ; i++ {
		node, ok := s.controller.GetNodeInfo(i)
		if !ok {
			break
		}
		nodes = append(nodes, map[string]interface{}{
			"id":       i,
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
		Sender:      protocol.ADMIN_UUID,
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
		Sender:      protocol.ADMIN_UUID,
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
			Sender:      protocol.ADMIN_UUID,
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
			Sender:      protocol.ADMIN_UUID,
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
		header := &protocol.Header{
			Version:     1,
			Sender:      protocol.ADMIN_UUID,
			Accepter:    res.UUID,
			MessageType: protocol.SOCKSSTART,
		}
		req := &protocol.SocksStart{
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
			Sender:      protocol.ADMIN_UUID,
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
			Sender:      protocol.ADMIN_UUID,
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
		Sender:      protocol.ADMIN_UUID,
		Accepter:    res.UUID,
		MessageType: protocol.SHUTDOWN,
	}
	shutdownMess := &protocol.Shutdown{OK: 1}
	if err := s.controller.SendToNode(res.UUID, header, shutdownMess); err != nil {
		return s.failure(fmt.Sprintf("failed to send shutdown: %v", err)), nil
	}

	return s.success(map[string]interface{}{"success": true, "node_id": nodeID}), nil
}
