package protocol

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"
)

func TestDeconstructDataRejectsHugeDataLen(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	go func() {
		// Minimal header: version(2) sender(10) accepter(10) type(2) routeLen(4)=0 dataLen(8)=huge
		buf := make([]byte, 2+10+10+2+4+8)
		binary.BigEndian.PutUint16(buf[0:2], 1)
		// skip sender/accepter zeros
		binary.BigEndian.PutUint16(buf[22:24], HI)
		binary.BigEndian.PutUint32(buf[24:28], 0)
		binary.BigEndian.PutUint64(buf[28:36], MaxDataLen+1)
		_, _ = c1.Write(buf)
	}()

	_ = c2.SetReadDeadline(time.Now().Add(2 * time.Second))
	msg := &RawMessage{Conn: c2, UUID: ControllerUUID, CryptoSecret: nil}
	_, _, err := msg.DeconstructData()
	if err == nil {
		t.Fatal("expected error for oversized DataLen")
	}
}

func TestDeconstructDataRejectsHugeRouteLen(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	go func() {
		buf := make([]byte, 2+10+10+2+4)
		binary.BigEndian.PutUint16(buf[0:2], 1)
		binary.BigEndian.PutUint16(buf[22:24], HI)
		binary.BigEndian.PutUint32(buf[24:28], MaxRouteLen+1)
		_, _ = c1.Write(buf)
		// reader should fail before reading the rest
	}()

	_ = c2.SetReadDeadline(time.Now().Add(2 * time.Second))
	msg := &RawMessage{Conn: c2, UUID: ControllerUUID, CryptoSecret: nil}
	_, _, err := msg.DeconstructData()
	if err == nil {
		t.Fatal("expected error for oversized RouteLen")
	}
}

func TestConstructDestructRoundTrip(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	done := make(chan error, 1)
	go func() {
		r := NewDownMsg(c2, "unit-test-secret", ControllerUUID)
		_, payload, err := DestructMessage(r)
		if err != nil {
			done <- err
			return
		}
		hm, ok := payload.(*HIMess)
		if !ok {
			done <- io.ErrUnexpectedEOF
			return
		}
		if hm.Greeting != HelloFromAgent {
			done <- io.ErrClosedPipe
			return
		}
		done <- nil
	}()

	s := NewDownMsg(c1, "unit-test-secret", JoinUUID)
	header := &Header{
		Version:     1,
		Sender:      JoinUUID,
		Accepter:    ControllerUUID,
		MessageType: HI,
		Route:       NoRoute,
		RouteLen:    uint32(len(NoRoute)),
	}
	hm := &HIMess{
		GreetingLen: uint16(len(HelloFromAgent)),
		Greeting:    HelloFromAgent,
		UUIDLen:     uint16(len(JoinUUID)),
		UUID:        JoinUUID,
		IsAdmin:     0,
		IsReconnect: 0,
	}
	if err := ConstructMessage(s, header, hm, false); err != nil {
		t.Fatal(err)
	}
	if err := s.SendMessage(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestScanReqRoundTrip(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	done := make(chan error, 1)
	go func() {
		r := NewDownMsg(c2, "unit-test-secret", ControllerUUID)
		h, payload, err := DestructMessage(r)
		if err != nil {
			done <- err
			return
		}
		if h.MessageType != SCANREQ {
			done <- io.ErrUnexpectedEOF
			return
		}
		req, ok := payload.(*ScanReq)
		if !ok || req.TaskID != "start_scan-1" || req.Mode != "fast" || req.Fingerprint != 1 {
			done <- io.ErrClosedPipe
			return
		}
		if req.Targets != "10.0.0.0/24" || req.Discover != 1 || req.Method != "auto" {
			done <- io.ErrUnexpectedEOF
			return
		}
		done <- nil
	}()

	s := NewDownMsg(c1, "unit-test-secret", ControllerUUID)
	header := &Header{
		Version:     1,
		Sender:      ControllerUUID,
		Accepter:    ControllerUUID,
		MessageType: SCANREQ,
		Route:       NoRoute,
	}
	req := &ScanReq{
		TaskIDLen:   uint16(len("start_scan-1")),
		TaskID:      "start_scan-1",
		TargetsLen:  uint32(len("10.0.0.0/24")),
		Targets:     "10.0.0.0/24",
		ModeLen:     uint16(len("fast")),
		Mode:        "fast",
		PortsLen:    0,
		Ports:       "",
		Fingerprint: 1,
		Concurrency: 50,
		TimeoutMs:   500,
		Discover:    1,
		MethodLen:   uint16(len("auto")),
		Method:      "auto",
	}
	if err := ConstructMessage(s, header, req, false); err != nil {
		t.Fatal(err)
	}
	if err := s.SendMessage(); err != nil {
		t.Fatal(err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}
