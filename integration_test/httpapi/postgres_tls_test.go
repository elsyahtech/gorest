//go:build integration && integrationcontainers

package httpapi_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elsyahtech/gorest/database"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestPostgresTLSCertificateVerification(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	caPath, certPath, keyPath := writePostgresTLSCertificates(t)
	configPath := filepath.Join(t.TempDir(), "postgres-ssl.conf")
	config := "listen_addresses = '*'\n" +
		"ssl = on\n" +
		"ssl_ca_file = '/tmp/testcontainers-go/postgres/ca_cert.pem'\n" +
		"ssl_cert_file = '/tmp/testcontainers-go/postgres/server.cert'\n" +
		"ssl_key_file = '/tmp/testcontainers-go/postgres/server.key'\n"
	if err := os.WriteFile(configPath, []byte(config), 0o600); err != nil {
		t.Fatalf("write PostgreSQL TLS configuration: %v", err)
	}

	container, err := postgres.Run(
		ctx,
		postgresImage,
		postgres.WithDatabase("gorest_tls_integration"),
		postgres.WithUsername("gorest_tls_test"),
		postgres.WithPassword("gorest_tls_test_only"),
		postgres.WithConfigFile(configPath),
		postgres.WithSSLCert(caPath, certPath, keyPath),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start TLS-enabled PostgreSQL test container: %v", err)
	}
	t.Cleanup(func() {
		terminateCtx, terminateCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer terminateCancel()
		if err := container.Terminate(terminateCtx); err != nil {
			t.Errorf("terminate TLS-enabled PostgreSQL test container: %v", err)
		}
	})

	exitCode, output, err := container.Exec(ctx, []string{
		"psql", "-U", "gorest_tls_test", "-d", "gorest_tls_integration", "-tAc", "SHOW ssl",
	})
	if err != nil {
		t.Fatalf("check PostgreSQL TLS setting: %v", err)
	}
	serverTLS, err := io.ReadAll(output)
	if err != nil {
		t.Fatalf("read PostgreSQL TLS setting: %v", err)
	}
	if exitCode != 0 || !bytes.Contains(serverTLS, []byte("on\n")) {
		t.Fatalf("PostgreSQL SSL setting = %q (exit %d), want on", serverTLS, exitCode)
	}
	port, err := container.MappedPort(ctx, "5432/tcp")
	if err != nil {
		t.Fatalf("get TLS-enabled PostgreSQL mapped port: %v", err)
	}
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("get TLS-enabled PostgreSQL host: %v", err)
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}
	baseConfig := database.Config{
		Driver:   database.POSTGRES,
		Host:     host,
		Port:     port.Port(),
		Name:     "gorest_tls_integration",
		Username: "gorest_tls_test",
		Password: "gorest_tls_test_only",
		TLS:      "require",
		Timeout:  10 * time.Second,
	}

	db, message, err := database.New(ctx, &baseConfig, "UTC")
	if err != nil {
		logs, logErr := container.Logs(ctx)
		if logErr != nil {
			t.Fatalf("connect to TLS-enabled PostgreSQL (TLS setup: %s): %v; read PostgreSQL logs: %v", message, err, logErr)
		}
		logOutput, readErr := io.ReadAll(logs)
		if readErr != nil {
			t.Fatalf("connect to TLS-enabled PostgreSQL (TLS setup: %s): %v; read PostgreSQL logs: %v", message, err, readErr)
		}
		t.Fatalf("connect to TLS-enabled PostgreSQL (TLS setup: %s): %v\nPostgreSQL logs:\n%s", message, err, logOutput)
	}
	defer func() {
		closeCtx, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()
		if _, _, err := db.Close(closeCtx, &baseConfig); err != nil {
			t.Errorf("close verified PostgreSQL TLS connection: %v", err)
		}
	}()

	var encrypted bool
	if err := db.SQL.QueryRowContext(ctx,
		"SELECT ssl FROM pg_stat_ssl WHERE pid = pg_backend_pid()",
	).Scan(&encrypted); err != nil {
		t.Fatalf("read PostgreSQL TLS session state: %v", err)
	}
	if !encrypted {
		t.Fatal("PostgreSQL session reports TLS disabled")
	}

	verifyConfig := baseConfig
	verifyConfig.TLS = database.TLSVerifyFull
	verifyConfig.SSLCAPath = caPath
	verifiedDB, tlsMessage, err := database.New(ctx, &verifyConfig, "UTC")
	if err != nil {
		t.Fatalf("connect with trusted CA and matching hostname (TLS setup: %s): %v", tlsMessage, err)
	}
	var result int
	if err := verifiedDB.SQL.QueryRowContext(ctx, "SELECT 1").Scan(&result); err != nil {
		t.Fatalf("query PostgreSQL with verify-full: %v", err)
	}
	if result != 1 {
		t.Fatalf("PostgreSQL verify-full query returned %d, want 1", result)
	}
	if _, _, err := verifiedDB.Close(ctx, &verifyConfig); err != nil {
		t.Fatalf("close PostgreSQL verify-full connection: %v", err)
	}

	untrustedCAPath, _, _, err := writePostgresTLSAuthority(t, "untrusted")
	if err != nil {
		t.Fatalf("create untrusted PostgreSQL CA: %v", err)
	}
	untrustedCAConfig := baseConfig
	untrustedCAConfig.TLS = database.TLSVerifyFull
	untrustedCAConfig.SSLCAPath = untrustedCAPath
	if unexpectedDB, tlsMessage, err := database.New(ctx, &untrustedCAConfig, "UTC"); err == nil {
		if _, _, closeErr := unexpectedDB.Close(ctx, &untrustedCAConfig); closeErr != nil {
			t.Errorf("close PostgreSQL connection accepted with untrusted CA: %v", closeErr)
		}
		t.Fatal("verify-full accepted a server certificate signed by an untrusted CA")
	} else if tlsMessage == "" {
		t.Fatal("PostgreSQL CA verification failed without setup guidance")
	}
}

func writePostgresTLSCertificates(t *testing.T) (string, string, string) {
	t.Helper()
	caPath, caCert, caKey, err := writePostgresTLSAuthority(t, "trusted")
	if err != nil {
		t.Fatalf("create PostgreSQL test CA: %v", err)
	}

	serverKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate PostgreSQL server key: %v", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		t.Fatalf("generate PostgreSQL server certificate serial: %v", err)
	}
	now := time.Now()
	serverTemplate := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "localhost"},
		NotBefore:    now.Add(-time.Minute),
		NotAfter:     now.Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSNames:     []string{"localhost"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, &serverKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("sign PostgreSQL server certificate: %v", err)
	}

	dir := t.TempDir()
	certPath := filepath.Join(dir, "server.crt")
	keyPath := filepath.Join(dir, "server.key")
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE", Bytes: serverDER,
	}), 0o600); err != nil {
		t.Fatalf("write PostgreSQL server certificate: %v", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(serverKey)
	if err != nil {
		t.Fatalf("marshal PostgreSQL server key: %v", err)
	}
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{
		Type: "PRIVATE KEY", Bytes: keyDER,
	}), 0o600); err != nil {
		t.Fatalf("write PostgreSQL server key: %v", err)
	}

	return caPath, certPath, keyPath
}

func writePostgresTLSAuthority(t *testing.T, name string) (string, *x509.Certificate, *ecdsa.PrivateKey, error) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return "", nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return "", nil, nil, err
	}
	now := time.Now()
	template := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "GOREST PostgreSQL " + name + " test CA"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return "", nil, nil, err
	}
	cert, err := x509.ParseCertificate(certDER)
	if err != nil {
		return "", nil, nil, err
	}
	caPath := filepath.Join(t.TempDir(), name+"-ca.crt")
	if err := os.WriteFile(caPath, pem.EncodeToMemory(&pem.Block{
		Type: "CERTIFICATE", Bytes: certDER,
	}), 0o600); err != nil {
		return "", nil, nil, err
	}
	return caPath, cert, key, nil
}
