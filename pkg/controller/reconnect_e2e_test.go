package controller

import (
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"styx-mcp/pkg/protocol"
)

// Lab-style local e2e: unexpected drop → agent reconnect → same node_id;
// intentional SHUTDOWN → no second join.
//
//	go test ./pkg/controller/ -run TestE2EAgentReconnect -count=1 -v
func TestE2EAgentReconnect(t *testing.T) {
	secret := "reconnect-e2e-secret"
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	listenAddr := ln.Addr().String()
	_ = ln.Close()

	c := NewController(&Options{
		Listen:       listenAddr,
		Secret:       secret,
		Downstream:   "raw",
		ReconnectMax: 3,
	})
	if err := c.Start(); err != nil {
		t.Fatalf("controller start: %v", err)
	}

	agentBin := agentBinaryPath(t)
	agentLog := filepath.Join(t.TempDir(), "agent.log")
	logFile, err := os.Create(agentLog)
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()

	cmd := exec.Command(agentBin,
		"-s", secret,
		"-c", listenAddr,
		"-reconnect", "1",
		"-reconnect-max", "3",
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	if err := cmd.Start(); err != nil {
		t.Fatalf("start agent: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	id, uuid := waitNode(t, c, 15*time.Second)
	if id != 0 {
		t.Fatalf("first node_id=%d want 0", id)
	}
	t.Logf("online uuid=%s node_id=%d", uuid, id)

	// Unexpected drop: close conn without SHUTDOWN.
	c.CloseNodeConn(uuid)

	deadline := time.Now().Add(20 * time.Second)
	sawEmpty := false
	for time.Now().Before(deadline) {
		nodes := c.ListNodes()
		if len(nodes) == 0 {
			sawEmpty = true
		}
		if sawEmpty && len(nodes) == 1 && nodes[0].ID == 0 && nodes[0].Node.UUID == uuid {
			t.Logf("reonline same node_id=0 uuid=%s", uuid)
			goto reonlineOK
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("reonline failed; sawEmpty=%v nodes=%v agentLog=%s\n%s",
		sawEmpty, summarizeNodes(c), agentLog, readFileTail(agentLog))
reonlineOK:

	// Intentional shutdown: SHUTDOWN then close → must not rejoin.
	header := &protocol.Header{
		Version:     1,
		Sender:      protocol.ControllerUUID,
		Accepter:    uuid,
		MessageType: protocol.SHUTDOWN,
	}
	if err := c.SendToNode(uuid, header, &protocol.Shutdown{OK: 1}); err != nil {
		t.Fatalf("send shutdown: %v", err)
	}
	c.CloseNodeConn(uuid)

	// Wait past reconnect budget (base 1s × attempts with backoff).
	time.Sleep(8 * time.Second)
	if nodes := c.ListNodes(); len(nodes) != 0 {
		t.Fatalf("after shutdown expected no nodes, got %v\nagent:\n%s",
			summarizeNodes(c), readFileTail(agentLog))
	}
	t.Log("shutdown: topology stayed empty (no rejoin)")
}

func waitNode(t *testing.T, c *Controller, d time.Duration) (id int, uuid string) {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		nodes := c.ListNodes()
		if len(nodes) >= 1 {
			return nodes[0].ID, nodes[0].Node.UUID
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for agent; nodes=%v", summarizeNodes(c))
	return 0, ""
}

func summarizeNodes(c *Controller) string {
	nodes := c.ListNodes()
	s := fmt.Sprintf("n=%d", len(nodes))
	for _, e := range nodes {
		s += fmt.Sprintf(" [%d %s]", e.ID, e.Node.UUID)
	}
	return s
}

func agentBinaryPath(t *testing.T) string {
	t.Helper()
	root := findModuleRoot(t)
	p := filepath.Join(root, "release", fmt.Sprintf("%s-%s", runtime.GOOS, runtime.GOARCH), "agent")
	if st, err := os.Stat(p); err != nil || st.IsDir() {
		t.Fatalf("agent binary missing: %s (run make build)", p)
	}
	return p
}

func findModuleRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	dir := wd
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func readFileTail(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return err.Error()
	}
	if len(b) > 4000 {
		return string(b[len(b)-4000:])
	}
	return string(b)
}
