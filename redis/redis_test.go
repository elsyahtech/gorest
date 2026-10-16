package redis

import (
	"context"
	"os"
	"path/filepath"
	"testing"
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
