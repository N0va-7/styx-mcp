package preauth

import (
	"errors"
	"io"
	"net"
	"time"

	"mcp-stowaway/internal/utils"
)

var authToken string

// GenerateToken stores the first 16 bytes of the MD5 digest of the secret.
func GenerateToken(secret string) {
	// Simple MD5-based token, matching Stowaway's original approach.
	// In a production system, use a proper KDF.
	authToken = utils.GetStringMd5(secret)[:16]
}

// Token returns the current pre-auth token.
func Token() string {
	return authToken
}

// ActivePreAuth runs the active side of the pre-auth handshake.
func ActivePreAuth(conn net.Conn) error {
	var (
		notValid = errors.New("invalid secret")
		timeout  = errors.New("connection timeout")
	)

	defer conn.SetReadDeadline(time.Time{})
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	if _, err := conn.Write([]byte(authToken)); err != nil {
		conn.Close()
		return err
	}

	buf := make([]byte, 16)
	n, err := io.ReadFull(conn, buf)
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		conn.Close()
		return timeout
	}
	if err != nil {
		conn.Close()
		return err
	}

	if string(buf[:n]) == authToken {
		return nil
	}

	conn.Close()
	return notValid
}

// PassivePreAuth runs the passive side of the pre-auth handshake.
func PassivePreAuth(conn net.Conn) error {
	var (
		notValid = errors.New("invalid secret")
		timeout  = errors.New("connection timeout")
	)

	defer conn.SetReadDeadline(time.Time{})
	conn.SetReadDeadline(time.Now().Add(10 * time.Second))

	buf := make([]byte, 16)
	n, err := io.ReadFull(conn, buf)
	if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
		conn.Close()
		return timeout
	}
	if err != nil {
		conn.Close()
		return err
	}

	if string(buf[:n]) == authToken {
		if _, err := conn.Write([]byte(authToken)); err != nil {
			conn.Close()
			return err
		}
		return nil
	}

	conn.Close()
	return notValid
}

