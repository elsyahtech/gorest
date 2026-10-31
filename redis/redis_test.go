package redis

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"

	"crypto/tls"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewRejectsMissingConfiguration(t *testing.T) {
	client, message, err := New(context.Background(), nil)
	if err == nil {
		t.Fatal("New() error = nil, want missing configuration error")
	}
	if client != nil {
		t.Fatalf("New() client = %v, want nil", client)
	}
	if message == "" {
		t.Fatal("New() message is empty, want configuration guidance")
	}
}

func TestRunReturnsValidatedRedisConfiguration(t *testing.T) {
	got := Run(Config{Host: "cache.internal"})
	if got.Host != "cache.internal" || got.Port != ConfigDefault.Port ||
		got.Timeout != ConfigDefault.Timeout || got.PoolTimeout != ConfigDefault.PoolTimeout {
		t.Fatalf("Run() configuration = %#v", got)
	}
}

func TestNewRejectsNilContext(t *testing.T) {
	config := ConfigDefault
	client, message, err := New(nil, &config)
	if err == nil {
		t.Fatal("New() error = nil, want nil-context error")
	}
	if client != nil {
		t.Fatalf("New() client = %v, want nil", client)
	}
	if message == "" {
		t.Fatal("New() message is empty, want context guidance")
	}
}

func TestNewRejectsCanceledStartupContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, message, err := New(ctx, &Config{Host: "127.0.0.1", Port: "6379"})
	if err == nil || message == "" {
		t.Fatalf("New(canceled context) = (%q, %v), want fail-fast guidance", message, err)
	}
}

func TestSetupTLSRejectsNilConfiguration(t *testing.T) {
	_, message, err := setupTLS(nil)
	if err == nil {
		t.Fatal("setupTLS(nil) error = nil")
	}
	if message == "" {
		t.Fatal("setupTLS(nil) returned no configuration guidance")
	}
}

func TestConfigSSLCertPathRejectsNilInputs(t *testing.T) {
	for _, test := range []struct {
		name   string
		config *Config
		tls    *tls.Config
	}{
		{name: "nil config", tls: &tls.Config{}},
		{name: "nil tls config", config: &Config{SSLCertPath: "client.crt", SSLKeyPath: "client.key"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := configSSLCertPath(test.config, test.tls)
			if err == nil {
				t.Fatal("configSSLCertPath() error = nil, want nil-input error")
			}
		})
	}
}

func TestSetupTLSAcceptsLocalhostCertificatePair(t *testing.T) {
	certPath, keyPath := writeLocalhostCertificatePair(t)
	tlsConfig, message, err := setupTLS(&Config{SSLCertPath: certPath, SSLKeyPath: keyPath})
	if err != nil {
		t.Fatalf("setupTLS(localhost pair) = %q, %v", message, err)
	}
	if len(tlsConfig.Certificates) != 1 {
		t.Fatalf("loaded certificate count = %d, want 1", len(tlsConfig.Certificates))
	}
}

func writeLocalhostCertificatePair(t *testing.T) (string, string) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{CommonName: "localhost"},
		NotBefore: time.Now().Add(-time.Hour),
		NotAfter: time.Now().Add(time.Hour),
		KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
		DNSNames: []string{"localhost"},
		IPAddresses: []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certPath := filepath.Join(t.TempDir(), "client.crt")
	keyPath := filepath.Join(t.TempDir(), "client.key")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)}), 0o600); err != nil {
		t.Fatal(err)
	}
	return certPath, keyPath
}

func TestSetupTLSKeepsCertificateVerificationEnabled(t *testing.T) {
	config, message, err := setupTLS(&Config{})
	if err != nil {
		t.Fatalf("setupTLS() error = %v, message = %q", err, message)
	}
	if config.InsecureSkipVerify {
		t.Fatal("setupTLS() enabled InsecureSkipVerify")
	}
}

func TestSetupTLSRejectsIncompleteClientCertificatePair(t *testing.T) {
	_, message, err := setupTLS(&Config{SSLCertPath: "client.crt"})
	if err == nil {
		t.Fatal("setupTLS() error = nil, want incomplete certificate pair error")
	}
	if message == "" {
		t.Fatal("setupTLS() message is empty, want certificate configuration guidance")
	}
}

func TestSetupTLSRejectsUnreadableCAFile(t *testing.T) {
	_, message, err := setupTLS(&Config{SSLCAPath: filepath.Join(t.TempDir(), "missing-ca.pem")})
	if err == nil {
		t.Fatal("setupTLS() error = nil, want CA file read error")
	}
	if message == "" {
		t.Fatal("setupTLS() message is empty, want CA path guidance")
	}
}

func TestSetupTLSRejectsInvalidCAFile(t *testing.T) {
	caPath := filepath.Join(t.TempDir(), "invalid-ca.pem")
	if err := os.WriteFile(caPath, []byte("not a PEM certificate"), 0o600); err != nil {
		t.Fatalf("write invalid CA fixture: %v", err)
	}

	_, message, err := setupTLS(&Config{SSLCAPath: caPath})
	if err == nil {
		t.Fatal("setupTLS() error = nil, want invalid CA error")
	}
	if message == "" {
		t.Fatal("setupTLS() message is empty, want CA contents guidance")
	}
}
