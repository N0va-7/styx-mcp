package node

import (
	"bytes"
	"context"
	"log/slog"
	"os/exec"
	"time"

	"styx-mcp/pkg/protocol"
)

const (
	execOutputLimit = 512 << 10 // 512 KiB combined stdout+stderr
	execDefaultTimeout = 30
	execMaxTimeout     = 120
)

func (n *Node) handleExecReq(req *protocol.ExecReq) {
	go n.runExec(req)
}

func (n *Node) runExec(req *protocol.ExecReq) {
	timeout := int(req.TimeoutSec)
	if timeout <= 0 {
		timeout = execDefaultTimeout
	}
	if timeout > execMaxTimeout {
		timeout = execMaxTimeout
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "sh", "-c", req.Cmd)
	if req.Workdir != "" {
		cmd.Dir = req.Workdir
	}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	duration := time.Since(start)

	res := &protocol.ExecRes{
		TaskIDLen:  uint16(len(req.TaskID)),
		TaskID:     req.TaskID,
		DurationMs: uint32(duration.Milliseconds()),
	}

	if ctx.Err() == context.DeadlineExceeded {
		res.TimedOut = 1
		res.ExitCode = 124
	} else if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			res.ExitCode = uint32(ee.ExitCode())
		} else {
			res.ExitCode = 1
			stderr.WriteString(err.Error())
		}
	}

	outBytes := stdout.Bytes()
	errBytes := stderr.Bytes()
	truncated := false
	if len(outBytes)+len(errBytes) > execOutputLimit {
		truncated = true
		// Prefer keeping stderr tail small; truncate stdout first.
		remain := execOutputLimit
		if len(errBytes) > remain/4 {
			errBytes = errBytes[:remain/4]
		}
		remain -= len(errBytes)
		if len(outBytes) > remain {
			outBytes = outBytes[:remain]
		}
	}
	if truncated {
		res.Truncated = 1
	}
	res.Stdout = outBytes
	res.StdoutLen = uint32(len(outBytes))
	res.Stderr = errBytes
	res.StderrLen = uint32(len(errBytes))

	header := &protocol.Header{
		Version:     1,
		Sender:      n.UUID,
		Accepter:    protocol.ControllerUUID,
		MessageType: protocol.EXECRES,
		RouteLen:    uint32(len(protocol.NoRoute)),
		Route:       protocol.NoRoute,
	}
	if err := n.sendToParent(header, res); err != nil {
		slog.Error("send exec result failed", "task", req.TaskID, "error", err)
	}
}

func (n *Node) sendToParent(header *protocol.Header, payload interface{}) error {
	sMessage := protocol.NewUpMsg(n.ParentConn, n.Options.Secret, n.UUID)
	if err := protocol.ConstructMessage(sMessage, header, payload, false); err != nil {
		return err
	}
	return sMessage.SendMessage()
}
