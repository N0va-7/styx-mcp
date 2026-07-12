package preauth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"net"
	"time"
)

const nonceSize = 32

var (
	errInvalidSecret = errors.New("invalid secret")
	errTimeout       = errors.New("connection timeout")
)

// GenerateToken is retained for API compatibility but no longer stores global state.
// Authentication is now performed via a mutual HMAC challenge-response.
func GenerateToken(_ string) {}

// Token is retained for API compatibility but always returns an empty string.
func Token() string { return "" }

// ActivePreAuth runs the active (connecting) side of the challenge-response handshake.
// Flow:
//   1. Read nonceP from the peer.
//   2. Generate nonceA and send nonceA || HMAC(secret, nonceP || nonceA || "active").
//   3. Read HMAC(secret, nonceA || nonceP || "passive") from the peer and verify.
func ActivePreAuth(conn net.Conn, secret string) error {
	defer conn.SetReadDeadline(time.Time{})
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	nonceP := make([]byte, nonceSize)
	if err := readFull(conn, nonceP); err != nil {
		conn.Close()
		return fmt.Errorf("read peer nonce: %w", err)
	}

	nonceA := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonceA); err != nil {
		conn.Close()
		return fmt.Errorf("generate nonce: %w", err)
	}

	proof := activeProof(secret, nonceP, nonceA)
	if _, err := conn.Write(append(nonceA, proof...)); err != nil {
		conn.Close()
		return fmt.Errorf("send proof: %w", err)
	}

	expected := passiveProof(secret, nonceA, nonceP)
	peerProof := make([]byte, len(expected))
	if err := readFull(conn, peerProof); err != nil {
		conn.Close()
		return fmt.Errorf("read peer proof: %w", err)
	}

	if !hmac.Equal(peerProof, expected) {
		conn.Close()
		return errInvalidSecret
	}

	return nil
}

// PassivePreAuth runs the passive (listening) side of the challenge-response handshake.
// Flow:
//   1. Generate nonceP and send it.
//   2. Read nonceA || HMAC(secret, nonceP || nonceA || "active") and verify.
//   3. Send HMAC(secret, nonceA || nonceP || "passive").
func PassivePreAuth(conn net.Conn, secret string) error {
	defer conn.SetReadDeadline(time.Time{})
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	nonceP := make([]byte, nonceSize)
	if _, err := io.ReadFull(rand.Reader, nonceP); err != nil {
		conn.Close()
		return fmt.Errorf("generate nonce: %w", err)
	}
	if _, err := conn.Write(nonceP); err != nil {
		conn.Close()
		return fmt.Errorf("send nonce: %w", err)
	}

	buf := make([]byte, nonceSize+sha256.Size)
	if err := readFull(conn, buf); err != nil {
		conn.Close()
		return fmt.Errorf("read peer nonce and proof: %w", err)
	}

	nonceA := buf[:nonceSize]
	proof := buf[nonceSize:]

	expected := activeProof(secret, nonceP, nonceA)
	if !hmac.Equal(proof, expected) {
		conn.Close()
		return errInvalidSecret
	}

	response := passiveProof(secret, nonceA, nonceP)
	if _, err := conn.Write(response); err != nil {
		conn.Close()
		return fmt.Errorf("send response: %w", err)
	}

	return nil
}

func activeProof(secret string, peerNonce, ownNonce []byte) []byte {
	return hmacSum(secret, peerNonce, ownNonce, []byte("active"))
}

func passiveProof(secret string, peerNonce, ownNonce []byte) []byte {
	return hmacSum(secret, peerNonce, ownNonce, []byte("passive"))
}

func hmacSum(secret string, parts ...[]byte) []byte {
	mac := hmac.New(sha256.New, []byte(secret))
	for _, p := range parts {
		mac.Write(p)
	}
	return mac.Sum(nil)
}

func readFull(conn net.Conn, buf []byte) error {
	_, err := io.ReadFull(conn, buf)
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		return errTimeout
	}
	return err
}
