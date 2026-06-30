package main

import (
	"log/slog"
	"os"

	"mcp-stowaway/pkg/node"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	opt := node.ParseOptions()
	if opt == nil {
		os.Exit(1)
	}

	n := node.NewNode(opt)
	if err := n.Run(); err != nil {
		slog.Error("node exited", "error", err)
		os.Exit(1)
	}
}
