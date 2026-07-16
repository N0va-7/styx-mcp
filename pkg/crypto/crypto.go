package crypto

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"time"
)

const (
	// aesKeyInfo is the HKDF info string for AES-256-GCM keys.
	aesKeyInfo = "styx-mcp/aes-256-gcm/v1"
	// tlsSeedInfo is the HKDF info string for deterministic TLS key material.
	tlsSeedInfo = "styx-mcp/tls-ed25519/v1"
	// TLSServerName is the fixed SNI / cert DNS name for secret-derived certs.
	TLSServerName = "styx-mcp.local"
	// maxGzipOut limits decompressed payload size (matches protocol.MaxDataLen).
	maxGzipOut = 36 << 20
)

// KeyPadding is retained for API compatibility. Prefer DeriveAESKey.
// It pads or truncates a key to 32 bytes for AES-256 (legacy, weak for short secrets).
func KeyPadding(key []byte) []byte {
	if len(key) == 0 {
		return nil
	}
	if len(key) >= 32 {
		return key[:32]
	}
	padding := make([]byte, 32-len(key))
	return append(key, padding...)
}

// DeriveAESKey derives a 32-byte AES-256 key from a shared secret via HKDF-SHA256.
// An empty secret returns nil (plaintext mode, same as KeyPadding).
func DeriveAESKey(secret []byte) []byte {
	if len(secret) == 0 {
		return nil
	}
	return hkdfSHA256(secret, []byte(aesKeyInfo), 32)
}

// hkdfSHA256 implements HKDF-Extract + Expand (RFC 5869) with a fixed salt label.
func hkdfSHA256(secret, info []byte, length int) []byte {
	// Extract: PRK = HMAC-Hash(salt, IKM)
	salt := []byte("styx-mcp-hkdf-salt-v1")
	extractor := hmac.New(sha256.New, salt)
	extractor.Write(secret)
	prk := extractor.Sum(nil)

	// Expand: OKM = T(1) | T(2) | ... where T(i) = HMAC-Hash(PRK, T(i-1) | info | i)
	out := make([]byte, 0, length)
	var t []byte
	var counter byte = 1
	for len(out) < length {
		mac := hmac.New(sha256.New, prk)
		mac.Write(t)
		mac.Write(info)
		mac.Write([]byte{counter})
		t = mac.Sum(nil)
		out = append(out, t...)
		counter++
	}
	return out[:length]
}

// AESEncrypt encrypts data with AES-256-GCM.
// If key is nil, returns plaintext unchanged.
func AESEncrypt(plainData, key []byte) ([]byte, error) {
	if key == nil {
		return plainData, nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}

	return gcm.Seal(nonce, nonce, plainData, nil), nil
}

// AESDecrypt decrypts data with AES-256-GCM.
// If key is nil, returns ciphertext unchanged.
func AESDecrypt(cipherData, key []byte) ([]byte, error) {
	if key == nil {
		return cipherData, nil
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}

	nonceSize := gcm.NonceSize()
	if len(cipherData) < nonceSize {
		return nil, errors.New("ciphertext too short")
	}

	nonce, cipherData := cipherData[:nonceSize], cipherData[nonceSize:]
	return gcm.Open(nil, nonce, cipherData, nil)
}

// GzipCompress compresses data using gzip.
func GzipCompress(src []byte) ([]byte, error) {
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write(src); err != nil {
		return nil, err
	}
	if err := w.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// GzipDecompress decompresses gzip data with a hard output size limit.
func GzipDecompress(src []byte) ([]byte, error) {
	br := bytes.NewReader(src)
	gr, err := gzip.NewReader(br)
	if err != nil {
		return nil, err
	}
	defer gr.Close()
	// +1 so we can detect overflow past maxGzipOut.
	limited := io.LimitReader(gr, int64(maxGzipOut)+1)
	out, err := io.ReadAll(limited)
	if err != nil {
		return nil, err
	}
	if len(out) > maxGzipOut {
		return nil, errors.New("gzip payload exceeds maximum size")
	}
	return out, nil
}

// GenerateTLSConfigFromSecret builds a tls.Config with a cert derived from the
// shared secret so both peers materialize the same identity and can verify it.
// When domain is non-empty it is used as ServerName and included in DNS SANs
// (both peers must pass the same domain). HMAC preauth remains the primary
// application-layer mutual auth.
func GenerateTLSConfigFromSecret(secret, domain string) (*tls.Config, error) {
	if secret == "" {
		return nil, errors.New("secret required for TLS")
	}

	serverName := TLSServerName
	if domain != "" {
		serverName = domain
	}

	// Bind key material to domain so secret+domain pairs stay consistent.
	seedInfo := append([]byte(tlsSeedInfo), []byte("|"+serverName)...)
	seed := hkdfSHA256([]byte(secret), seedInfo, ed25519.SeedSize)
	priv := ed25519.NewKeyFromSeed(seed)
	pub := priv.Public().(ed25519.PublicKey)

	serialBytes := hkdfSHA256([]byte(secret), append([]byte("styx-mcp/tls-serial/v1|"), []byte(serverName)...), 16)
	template := x509.Certificate{
		SerialNumber: new(big.Int).SetBytes(serialBytes),
		Subject: pkix.Name{
			Organization: []string{"styx-mcp"},
			CommonName:   serverName,
		},
		// Fixed validity so cert DER is identical on every peer.
		NotBefore:             time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		NotAfter:              time.Date(2044, 1, 1, 0, 0, 0, 0, time.UTC),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{serverName, TLSServerName, "localhost"},
	}

	// Ed25519 signatures are deterministic; rand.Reader is unused for the signature.
	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, pub, priv)
	if err != nil {
		return nil, fmt.Errorf("create certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPKCS8, err := x509.MarshalPKCS8PrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("marshal key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyPKCS8})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("load key pair: %w", err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(certPEM) {
		return nil, errors.New("failed to build cert pool")
	}

	return &tls.Config{
		Certificates: []tls.Certificate{tlsCert},
		RootCAs:      pool,
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS12,
		ServerName:   serverName,
	}, nil
}

// GenerateTLSConfig generates a legacy self-signed TLS config (dev only).
// Prefer GenerateTLSConfigFromSecret when a shared secret is available.
func GenerateTLSConfig() (*tls.Config, *tls.Certificate, error) {
	cfg, err := GenerateTLSConfigFromSecret("styx-mcp-legacy-dev-only", "")
	if err != nil {
		return nil, nil, err
	}
	return cfg, &cfg.Certificates[0], nil
}

// NewServerTLSConfig returns a tls.Config for the controller/listener side.
func NewServerTLSConfig(secret, domain string) (*tls.Config, error) {
	return GenerateTLSConfigFromSecret(secret, domain)
}

// NewClientTLSConfig returns a tls.Config for the node/connecting side.
func NewClientTLSConfig(secret, domain string) (*tls.Config, error) {
	return GenerateTLSConfigFromSecret(secret, domain)
}
