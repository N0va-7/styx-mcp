package main

import (
	"fmt"
	"log/slog"
	"os"

	"styx-mcp/pkg/controller"
	"styx-mcp/pkg/mcp"
)

func main() {
	logFile, err := os.OpenFile("/tmp/styx-mcp-controller.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		logFile = os.Stderr
	}
	defer logFile.Close()
	slog.SetDefault(slog.New(slog.NewTextHandler(logFile, &slog.HandlerOptions{Level: slog.LevelInfo})))

	opt := controller.ParseOptions()
	if opt == nil {
		os.Exit(1)
	}

	ctrl := controller.NewController(opt)

	// Bind / dial before accepting MCP stdio so port conflicts fail the process
	// (MCP clients mark the server unhealthy instead of an empty topology).
	slog.Info("starting controller network layer")
	if err := ctrl.Start(); err != nil {
		slog.Error("controller network failed", "error", err)
		fmt.Fprintf(os.Stderr, "styx-mcp controller: %v\n", err)
		os.Exit(1)
	}

	slog.Info("starting MCP server")
	server := mcp.NewServer(ctrl)
	if err := server.Serve(); err != nil {
		slog.Error("mcp server exited", "error", err)
		os.Exit(1)
	}
}
