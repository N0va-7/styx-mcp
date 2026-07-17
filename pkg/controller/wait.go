package controller

import (
	"fmt"
	"time"
)

// DefaultAckTimeout is how long MCP start_* waits for a peer RES/READY.
const DefaultAckTimeout = 20 * time.Second

type ackKey struct {
	uuid string
	kind string
}

// Ack kinds delivered from agent responses.
const (
	AckListen  = "listen"
	AckConnect = "connect"
	AckForward = "forward"
)

func (c *Controller) armAck(nodeUUID, kind string) <-chan bool {
	ch := make(chan bool, 1)
	key := ackKey{uuid: nodeUUID, kind: kind}

	c.acksMu.Lock()
	if c.acks == nil {
		c.acks = make(map[ackKey]chan bool)
	}
	if old, ok := c.acks[key]; ok {
		select {
		case old <- false:
		default:
		}
	}
	c.acks[key] = ch
	c.acksMu.Unlock()
	return ch
}

func (c *Controller) disarmAck(nodeUUID, kind string) {
	c.acksMu.Lock()
	delete(c.acks, ackKey{uuid: nodeUUID, kind: kind})
	c.acksMu.Unlock()
}

// signalAck completes a waiter if one is armed (LISTENRES / CONNECTDONE / FORWARDREADY).
func (c *Controller) signalAck(nodeUUID, kind string, ok bool) {
	key := ackKey{uuid: nodeUUID, kind: kind}
	c.acksMu.Lock()
	ch, exists := c.acks[key]
	if exists {
		delete(c.acks, key)
	}
	c.acksMu.Unlock()
	if !exists {
		return
	}
	select {
	case ch <- ok:
	default:
	}
}

// AfterSendWait arms an ack waiter, runs send, then waits for the agent response.
// The waiter is registered before send so a fast RES cannot be missed.
func (c *Controller) AfterSendWait(nodeUUID, kind string, timeout time.Duration, send func() error) (bool, error) {
	if timeout <= 0 {
		timeout = DefaultAckTimeout
	}
	ch := c.armAck(nodeUUID, kind)
	if err := send(); err != nil {
		c.disarmAck(nodeUUID, kind)
		return false, err
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case ok := <-ch:
		return ok, nil
	case <-timer.C:
		c.disarmAck(nodeUUID, kind)
		return false, fmt.Errorf("timeout waiting for %s ack from node %s", kind, nodeUUID)
	}
}
