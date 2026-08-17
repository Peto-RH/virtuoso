package candlepin

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendPEMCertsFromDir(t *testing.T) {
	t.Run("loads PEM certificates and ignores other entries", func(t *testing.T) {
		dir := t.TempDir()
		certPEM := testCertificatePEM(t)
		if err := os.WriteFile(filepath.Join(dir, "certificate.pem"), certPEM, 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("not a certificate"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(filepath.Join(dir, "nested.pem"), 0o700); err != nil {
			t.Fatal(err)
		}

		pool := x509.NewCertPool()
		loaded, err := appendPEMCertsFromDir(pool, dir)
		if err != nil {
			t.Fatalf("appendPEMCertsFromDir() error = %v", err)
		}
		if loaded != 1 {
			t.Errorf("appendPEMCertsFromDir() loaded = %d, want 1", loaded)
		}

		want := x509.NewCertPool()
		want.AppendCertsFromPEM(certPEM)
		if !pool.Equal(want) {
			t.Error("certificate pool does not contain expected certificate")
		}
	})

	t.Run("empty directory", func(t *testing.T) {
		loaded, err := appendPEMCertsFromDir(x509.NewCertPool(), t.TempDir())
		if err != nil {
			t.Fatalf("appendPEMCertsFromDir() error = %v", err)
		}
		if loaded != 0 {
			t.Errorf("appendPEMCertsFromDir() loaded = %d, want 0", loaded)
		}
	})

	t.Run("missing directory", func(t *testing.T) {
		dir := filepath.Join(t.TempDir(), "missing")
		loaded, err := appendPEMCertsFromDir(x509.NewCertPool(), dir)
		if loaded != 0 {
			t.Errorf("appendPEMCertsFromDir() loaded = %d, want 0", loaded)
		}
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("appendPEMCertsFromDir() error = %v, want %v", err, os.ErrNotExist)
		}
		if !strings.Contains(err.Error(), dir) {
			t.Errorf("appendPEMCertsFromDir() error = %q, want directory path", err)
		}
	})

	t.Run("malformed PEM certificate", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, "invalid.pem")
		if err := os.WriteFile(path, []byte("not a certificate"), 0o600); err != nil {
			t.Fatal(err)
		}

		loaded, err := appendPEMCertsFromDir(x509.NewCertPool(), dir)
		if loaded != 0 {
			t.Errorf("appendPEMCertsFromDir() loaded = %d, want 0", loaded)
		}
		if err == nil {
			t.Fatal("appendPEMCertsFromDir() error = nil, want malformed certificate error")
		}
		if !strings.Contains(err.Error(), path) {
			t.Errorf("appendPEMCertsFromDir() error = %q, want certificate path", err)
		}
	})
}

func testCertificatePEM(t *testing.T) []byte {
	t.Helper()

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("generate test certificate key: %v", err)
	}
	template := x509.Certificate{SerialNumber: big.NewInt(1)}
	certificate, err := x509.CreateCertificate(
		rand.Reader,
		&template,
		&template,
		privateKey.Public(),
		privateKey,
	)
	if err != nil {
		t.Fatalf("create test certificate: %v", err)
	}

	encoded := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate})
	if encoded == nil {
		t.Fatal("failed to encode test certificate")
	}
	return encoded
}
