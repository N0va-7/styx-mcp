package crypto

import (
	"bytes"
	"testing"
)

func TestDeriveAESKeyDeterministic(t *testing.T) {
	a := DeriveAESKey([]byte("test-secret"))
	b := DeriveAESKey([]byte("test-secret"))
	if len(a) != 32 {
		t.Fatalf("key len = %d, want 32", len(a))
	}
	if !bytes.Equal(a, b) {
		t.Fatal("DeriveAESKey not deterministic")
	}
	c := DeriveAESKey([]byte("other-secret"))
	if bytes.Equal(a, c) {
		t.Fatal("different secrets produced same key")
	}
	if DeriveAESKey(nil) != nil {
		t.Fatal("empty secret should yield nil key")
	}
}

func TestDeriveAESKeyDiffersFromKeyPadding(t *testing.T) {
	secret := []byte("short")
	derived := DeriveAESKey(secret)
	padded := KeyPadding(secret)
	if bytes.Equal(derived, padded) {
		t.Fatal("HKDF key should not match zero-padded legacy key")
	}
}

func TestTLSConfigFromSecretStable(t *testing.T) {
	cfg1, err := GenerateTLSConfigFromSecret("shared-secret", "")
	if err != nil {
		t.Fatal(err)
	}
	cfg2, err := GenerateTLSConfigFromSecret("shared-secret", "")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(cfg1.Certificates[0].Certificate[0], cfg2.Certificates[0].Certificate[0]) {
		t.Fatal("TLS cert DER should be identical for same secret")
	}
	if cfg1.ServerName != TLSServerName {
		t.Fatalf("ServerName = %q, want %q", cfg1.ServerName, TLSServerName)
	}

	other, err := GenerateTLSConfigFromSecret("other-secret", "")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(cfg1.Certificates[0].Certificate[0], other.Certificates[0].Certificate[0]) {
		t.Fatal("different secrets should produce different certs")
	}
}

func TestGzipDecompressLimit(t *testing.T) {
	// Compress a small payload and ensure round-trip works.
	plain := bytes.Repeat([]byte("a"), 1024)
	comp, err := GzipCompress(plain)
	if err != nil {
		t.Fatal(err)
	}
	out, err := GzipDecompress(comp)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(out, plain) {
		t.Fatal("round-trip mismatch")
	}
}

func TestAESRoundTrip(t *testing.T) {
	key := DeriveAESKey([]byte("secret"))
	ct, err := AESEncrypt([]byte("hello"), key)
	if err != nil {
		t.Fatal(err)
	}
	pt, err := AESDecrypt(ct, key)
	if err != nil {
		t.Fatal(err)
	}
	if string(pt) != "hello" {
		t.Fatalf("got %q", pt)
	}
}
