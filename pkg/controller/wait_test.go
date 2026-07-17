package controller

import (
	"errors"
	"testing"
	"time"
)

func TestAfterSendWaitOK(t *testing.T) {
	c := NewController(&Options{Secret: "t", Listen: "127.0.0.1:0"})
	go func() {
		time.Sleep(20 * time.Millisecond)
		c.signalAck("node-a", AckListen, true)
	}()
	ok, err := c.AfterSendWait("node-a", AckListen, time.Second, func() error { return nil })
	if err != nil || !ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestAfterSendWaitReject(t *testing.T) {
	c := NewController(&Options{Secret: "t", Listen: "127.0.0.1:0"})
	go func() {
		time.Sleep(20 * time.Millisecond)
		c.signalAck("node-b", AckConnect, false)
	}()
	ok, err := c.AfterSendWait("node-b", AckConnect, time.Second, func() error { return nil })
	if err != nil || ok {
		t.Fatalf("ok=%v err=%v", ok, err)
	}
}

func TestAfterSendWaitTimeout(t *testing.T) {
	c := NewController(&Options{Secret: "t", Listen: "127.0.0.1:0"})
	ok, err := c.AfterSendWait("node-c", AckForward, 30*time.Millisecond, func() error { return nil })
	if err == nil || ok {
		t.Fatalf("expected timeout, ok=%v err=%v", ok, err)
	}
}

func TestAfterSendWaitSendError(t *testing.T) {
	c := NewController(&Options{Secret: "t", Listen: "127.0.0.1:0"})
	ok, err := c.AfterSendWait("node-d", AckListen, time.Second, func() error {
		return errors.New("send failed")
	})
	if err == nil || ok {
		t.Fatalf("expected send error, ok=%v err=%v", ok, err)
	}
}
