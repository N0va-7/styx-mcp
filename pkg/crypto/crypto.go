package crypto

import (
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/rsa"
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

// KeyPadding pads or truncates a key to 32 bytes for AES-256.
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

// GzipDecompress decompresses gzip data.
func GzipDecompress(src []byte) ([]byte, error) {
	br := bytes.NewReader(src)
	gr, err := gzip.NewReader(br)
	if err != nil {
		return nil, err
	}
	defer gr.Close()
	return io.ReadAll(gr)
}

// GenerateTLSConfig generates a self-signed TLS certificate and returns a tls.Config.
func GenerateTLSConfig() (*tls.Config, *tls.Certificate, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, fmt.Errorf("generate key: %w", err)
	}

	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			Organization: []string{"mcp-stowaway"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		DNSNames:              []string{"localhost"},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &priv.PublicKey, priv)
	if err != nil {
		return nil, nil, fmt.Errorf("create certificate: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)})

	cert, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, nil, fmt.Errorf("load key pair: %w", err)
	}

	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(certPEM)

	config := &tls.Config{
		Certificates:       []tls.Certificate{cert},
		RootCAs:            pool,
		InsecureSkipVerify: true,
	}

	return config, &cert, nil
}

// NewServerTLSConfig returns a tls.Config for the controller/listener side.
func NewServerTLSConfig() (*tls.Config, error) {
	cfg, _, err := GenerateTLSConfig()
	return cfg, err
}

// NewClientTLSConfig returns a tls.Config for the node/connecting side.
func NewClientTLSConfig(_ string) (*tls.Config, error) {
	cfg, _, err := GenerateTLSConfig()
	return cfg, err
}
