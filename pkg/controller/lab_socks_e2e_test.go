// Lab / SOCKS path checks (controller → local agent → HTTP).
//
//	go test ./pkg/controller/ -run 'TestE2E' -count=1
//	STYX_LAB_URL=http://10.7.11.116/ go test ./pkg/controller/ -run TestE2ESOCKSLabHTTP -count=1
package controller

import (
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestE2ESOCKSLocalAgentHTTP(t *testing.T) {
	// Always-on: SOCKS via local agent to a loopback HTTP server.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	wantBody := "styx-e2e-ok"
	go serveOnceOK(ln, wantBody)

	target := "http://" + ln.Addr().String() + "/"
	runSOCKSThroughLocalAgent(t, target, func(body string) {
		if !strings.Contains(body, wantBody) {
			t.Fatalf("body=%q", body)
		}
	})
}

func TestE2ESOCKSLabHTTP(t *testing.T) {
	lab := os.Getenv("STYX_LAB_URL")
	if lab == "" {
		lab = "http://10.7.11.116/"
	}
	// Prefer curl for reachability (matches operator workflow; more tolerant than net/http defaults).
	if err := exec.Command("curl", "-sS", "-m", "15", "-o", os.DevNull, lab).Run(); err != nil {
		t.Skipf("lab unreachable: %v", err)
	}

	runSOCKSThroughLocalAgent(t, lab, func(body string) {
		if !strings.Contains(body, "闻道基金") && !strings.Contains(body, "<html") {
			t.Fatalf("lab body missing expected markers (len=%d)", len(body))
		}
	})
}

func serveOnceOK(ln net.Listener, body string) {
	for {
		c, err := ln.Accept()
		if err != nil {
			return
		}
		go func(c net.Conn) {
			defer c.Close()
			buf := make([]byte, 1024)
			_, _ = c.Read(buf)
			_, _ = fmt.Fprintf(c, "HTTP/1.1 200 OK\r\nContent-Length: %d\r\nConnection: close\r\n\r\n%s", len(body), body)
		}(c)
	}
}

func runSOCKSThroughLocalAgent(t *testing.T, httpURL string, check func(body string)) {
	t.Helper()

	if _, err := exec.LookPath("curl"); err != nil {
		t.Skip("curl required for SOCKS HTTP check")
	}

	secret := "e2e-lab-secret"
	listen := freeListenAddr(t)

	ctrl := NewController(&Options{Secret: secret, Listen: listen, Downstream: "raw"})
	if err := ctrl.Start(); err != nil {
		t.Fatalf("controller Start: %v", err)
	}

	agentBin := agentBinary(t)
	cmd := exec.Command(agentBin, "-s", secret, "-c", listen)
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatalf("start agent: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	nodeUUID := waitForNode(t, ctrl, 8*time.Second)

	socksAddr := freeListenAddr(t)
	done := make(chan error, 1)
	go func() {
		done <- ctrl.StartSocks(nodeUUID, socksAddr)
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("StartSocks: %v", err)
		}
	case <-time.After(15 * time.Second):
		t.Fatal("StartSocks timeout")
	}

	body := curlViaSOCKS(t, socksAddr, httpURL)
	check(body)
}

func freeListenAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	return addr
}

func agentBinary(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	bin := filepath.Join(root, "release", runtime.GOOS+"-"+runtime.GOARCH, "agent")
	if st, err := os.Stat(bin); err != nil || st.IsDir() {
		t.Skipf("agent binary missing at %s (run make build)", bin)
	}
	return bin
}

func waitForNode(t *testing.T, c *Controller, d time.Duration) string {
	t.Helper()
	deadline := time.Now().Add(d)
	for time.Now().Before(deadline) {
		nodes := c.ListNodes()
		if len(nodes) > 0 && nodes[0].Node != nil {
			return nodes[0].Node.UUID
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("no agent joined")
	return ""
}

func curlViaSOCKS(t *testing.T, socksAddr, url string) string {
	t.Helper()
	// --socks5-hostname: resolve target through the proxy (agent exit).
	cmd := exec.Command("curl", "-sS", "-m", "20",
		"--socks5-hostname", socksAddr,
		url,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("curl via SOCKS: %v\n%s", err, out)
	}
	return string(out)
}
