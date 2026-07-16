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
	msg := &RawMessage{Conn: c2, UUID: ADMIN_UUID, CryptoSecret: nil}
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
	msg := &RawMessage{Conn: c2, UUID: ADMIN_UUID, CryptoSecret: nil}
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
		r := NewDownMsg(c2, "unit-test-secret", ADMIN_UUID)
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
		if hm.Greeting != "Shhh..." {
			done <- io.ErrClosedPipe
			return
		}
		done <- nil
	}()

	s := NewDownMsg(c1, "unit-test-secret", TEMP_UUID)
	header := &Header{
		Version:     1,
		Sender:      TEMP_UUID,
		Accepter:    ADMIN_UUID,
		MessageType: HI,
		Route:       TEMP_ROUTE,
		RouteLen:    uint32(len(TEMP_ROUTE)),
	}
	hm := &HIMess{
		GreetingLen: uint16(len("Shhh...")),
		Greeting:    "Shhh...",
		UUIDLen:     uint16(len(TEMP_UUID)),
		UUID:        TEMP_UUID,
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
