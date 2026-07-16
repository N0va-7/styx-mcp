package transport

import (
	"crypto/tls"
	"net"

	"styx-mcp/pkg/crypto"
)

// WrapTLSClientConn wraps a net.Conn with TLS client config.
func WrapTLSClientConn(conn net.Conn, config *tls.Config) net.Conn {
	return tls.Client(conn, config)
}

// WrapTLSServerConn wraps a net.Conn with TLS server config.
func WrapTLSServerConn(conn net.Conn, config *tls.Config) net.Conn {
	return tls.Server(conn, config)
}

// NewClientTLSConfig returns a TLS config for client connections.
// secret and domain must match the peer (domain may be empty).
func NewClientTLSConfig(secret, domain string) (*tls.Config, error) {
	return crypto.NewClientTLSConfig(secret, domain)
}

// NewServerTLSConfig returns a TLS config for server/listener connections.
func NewServerTLSConfig(secret, domain string) (*tls.Config, error) {
	return crypto.NewServerTLSConfig(secret, domain)
}
