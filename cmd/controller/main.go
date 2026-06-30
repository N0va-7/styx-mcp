package main

import (
	"log/slog"
	"os"
	"time"

	"styx-mcp/pkg/controller"
	"styx-mcp/pkg/mcp"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug})))

	opt := controller.ParseOptions()
	if opt == nil {
		os.Exit(1)
	}

	ctrl := controller.NewController(opt)

	// Start controller networking in background.
	go func() {
		slog.Info("starting controller network layer")
		if err := ctrl.Run(); err != nil {
			slog.Error("controller exited", "error", err)
			os.Exit(1)
		}
	}()

	// Give the network layer a moment to start before accepting MCP stdio.
	time.Sleep(100 * time.Millisecond)

	// Start MCP server on stdio.
	slog.Info("starting MCP server")
	server := mcp.NewServer(ctrl)
	if err := server.Serve(); err != nil {
		slog.Error("mcp server exited", "error", err)
		os.Exit(1)
	}
}
