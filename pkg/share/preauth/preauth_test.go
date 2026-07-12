package preauth

import (
	"net"
	"testing"
	"time"
)

func TestMutualAuthSuccess(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	secret := "test-secret"

	done := make(chan error, 2)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		done <- PassivePreAuth(conn, secret)
	}()

	go func() {
		conn, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			done <- err
			return
		}
		done <- ActivePreAuth(conn, secret)
	}()

	for i := 0; i < 2; i++ {
		select {
		case err := <-done:
			if err != nil {
				t.Fatalf("handshake failed: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("handshake timeout")
		}
	}
}

func TestMutualAuthWrongSecret(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	done := make(chan error, 2)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			done <- err
			return
		}
		err = PassivePreAuth(conn, "correct-secret")
		done <- err
	}()

	go func() {
		conn, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			done <- err
			return
		}
		err = ActivePreAuth(conn, "wrong-secret")
		done <- err
	}()

	sawInvalid := false
	for i := 0; i < 2; i++ {
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("expected handshake to fail")
			}
			if err == errInvalidSecret {
				sawInvalid = true
			}
			// The other side may see EOF because the peer closes the connection
			// as soon as it detects the bad secret.
		case <-time.After(5 * time.Second):
			t.Fatal("handshake timeout")
		}
	}

	if !sawInvalid {
		t.Fatal("expected at least one side to return invalid secret")
	}
}
