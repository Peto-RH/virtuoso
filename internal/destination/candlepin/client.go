package candlepin

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	httpTimeout = 60 * time.Second
	rhsmCADir   = "/etc/rhsm/ca"
)

func newHTTPClient(certPath, keyPath string) (*http.Client, error) {
	slog.Debug("loading client certificate", "cert", certPath, "key", keyPath)
	cert, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load client certificate: %w", err)
	}

	pool := x509.NewCertPool()

	loaded, err := appendPEMCertsFromDir(pool, rhsmCADir)
	if err != nil {
		return nil, err
	}
	if loaded == 0 {
		return nil, fmt.Errorf("no RHSM CA certificates loaded from %s", rhsmCADir)
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{
		RootCAs:      pool,
		Certificates: []tls.Certificate{cert},
	}

	return &http.Client{
		Timeout:   httpTimeout,
		Transport: transport,
	}, nil
}

func appendPEMCertsFromDir(pool *x509.CertPool, dir string) (int, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("failed to read RHSM CA directory %s: %w", dir, err)
	}

	loaded := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".pem") {
			continue
		}

		path := filepath.Join(dir, entry.Name())

		pemData, err := os.ReadFile(path)
		if err != nil {
			return loaded, fmt.Errorf("failed to read RHSM CA certificate %s: %w", path, err)
		}

		if ok := pool.AppendCertsFromPEM(pemData); !ok {
			return loaded, fmt.Errorf("failed to append RHSM CA certificate %s", path)
		}

		slog.Debug("loaded RHSM CA certificate", "file", entry.Name())
		loaded++
	}

	return loaded, nil
}
