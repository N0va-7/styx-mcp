package transport

import (
	"crypto/tls"
	"net"

	"mcp-stowaway/pkg/crypto"
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
func NewClientTLSConfig(domain string) (*tls.Config, error) {
	return crypto.NewClientTLSConfig(domain)
}

// NewServerTLSConfig returns a TLS config for server/listener connections.
func NewServerTLSConfig() (*tls.Config, error) {
	return crypto.NewServerTLSConfig()
}
