//go:build integration && integrationcontainers

package httpapi_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"testing"
	"time"

	dockercontainer "github.com/docker/docker/api/types/container"
	"github.com/docker/go-connections/nat"
	"github.com/elsyahtech/gorest/database"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestScyllaMTLSManaged(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	image := scyllaImageDefault
	if configured := os.Getenv("GOREST_IT_SCYLLA_IMAGE"); configured != "" {
		image = configured
	}
	hostPort := availableLoopbackPort(t)
	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        image,
			Cmd:          []string{"--smp", "1", "--memory", "1G", "--overprovisioned", "1"},
			ExposedPorts: []string{"9042/tcp"},
			HostConfigModifier: func(config *dockercontainer.HostConfig) {
				config.PortBindings = nat.PortMap{
					"9042/tcp": {{HostIP: "127.0.0.1", HostPort: hostPort}},
				}
			},
			WaitingFor: wait.ForAll(
				wait.ForLog("Starting listening for CQL clients").WithStartupTimeout(12*time.Minute),
				wait.ForListeningPort("9042/tcp").WithStartupTimeout(2*time.Minute),
			).WithDeadline(13 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start ScyllaDB mTLS test container using %s: %v", image, err)
	}
	t.Cleanup(func() {
		terminateCtx, terminateCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer terminateCancel()
		if err := container.Terminate(terminateCtx); err != nil {
			t.Errorf("terminate ScyllaDB mTLS test container: %v", err)
		}
	})

	certificates, err := newScyllaMTLSCertificates()
	if err != nil {
		t.Fatalf("generate ScyllaDB mTLS test certificates: %v", err)
	}
	caPath, clientCertPath, clientKeyPath := writeScyllaMTLSClientFiles(t, certificates)
	configContext, cancelConfig := context.WithTimeout(ctx, 30*time.Second)
	defer cancelConfig()
	if err := installScyllaMTLSConfig(configContext, container, certificates); err != nil {
		t.Fatalf("configure ScyllaDB to require client certificates: %v", err)
	}

	stopCtx, stopCancel := context.WithTimeout(ctx, 30*time.Second)
	zeroGrace := time.Duration(0)
	if err := container.Stop(stopCtx, &zeroGrace); err != nil {
		stopCancel()
		t.Fatalf("stop ScyllaDB before enabling mTLS: %v", err)
	}
	stopCancel()
	startCtx, startCancel := context.WithTimeout(ctx, 10*time.Minute)
	if err := container.Start(startCtx); err != nil {
		startCancel()
		t.Fatalf("restart ScyllaDB with mTLS enabled: %v", err)
	}
	startCancel()
	waitForContainerPort(t, database.SCYLLA, container)

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("get ScyllaDB mTLS test host: %v", err)
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}
	port, err := container.MappedPort(ctx, "9042/tcp")
	if err != nil {
		t.Fatalf("get ScyllaDB mTLS test port: %v", err)
	}
	baseConfig := database.Config{
		Driver:    database.SCYLLA,
		Host:      host,
		Port:      port.Port(),
		Name:      "system",
		TLS:       "enable",
		SSLCAPath: caPath,
		Timeout:   10 * time.Second,
	}

	noCertificateCtx, cancelNoCertificate := context.WithTimeout(ctx, 10*time.Second)
	_, _, err = database.New(noCertificateCtx, &baseConfig, "UTC")
	cancelNoCertificate()
	if err == nil {
		t.Fatal("ScyllaDB accepted a TLS connection without a required client certificate")
	}

	clientConfig := baseConfig
	clientConfig.SSLCertPath = clientCertPath
	clientConfig.SSLKeyPath = clientKeyPath
	clientCtx, cancelClient := context.WithTimeout(ctx, 15*time.Second)
	db, message, err := database.New(clientCtx, &clientConfig, "UTC")
	if err != nil {
		cancelClient()
		t.Fatalf("connect to ScyllaDB with a trusted client certificate (TLS setup: %s): %v", message, err)
	}
	defer func() {
		closeCtx, cancelClose := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelClose()
		if _, _, err := db.Close(closeCtx, &clientConfig); err != nil {
			t.Errorf("close ScyllaDB mTLS test connection: %v", err)
		}
	}()

	if message, err := db.Ping(clientCtx, &clientConfig); err != nil {
		cancelClient()
		t.Fatalf("ping ScyllaDB over mTLS (%s): %v", message, err)
	}
	var version string
	if err := db.Scylla.Query("SELECT release_version FROM system.local").
		WithContext(clientCtx).Scan(&version); err != nil {
		cancelClient()
		t.Fatalf("query ScyllaDB over mTLS: %v", err)
	}
	cancelClient()
	t.Logf("ScyllaDB mTLS verified: server version %s; connection without client certificate rejected", version)
}

type scyllaMTLSCertificates struct {
	caPEM         []byte
	serverCertPEM []byte
	serverKeyPEM  []byte
	clientCertPEM []byte
	clientKeyPEM  []byte
}

func newScyllaMTLSCertificates() (scyllaMTLSCertificates, error) {
	caKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return scyllaMTLSCertificates{}, fmt.Errorf("generate test CA key: %w", err)
	}
	now := time.Now()
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "GOREST ScyllaDB mTLS test CA"},
		NotBefore:             now.Add(-time.Minute),
		NotAfter:              now.Add(24 * time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return scyllaMTLSCertificates{}, fmt.Errorf("create test CA certificate: %w", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return scyllaMTLSCertificates{}, fmt.Errorf("parse test CA certificate: %w", err)
	}

	serverKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return scyllaMTLSCertificates{}, fmt.Errorf("generate server key: %w", err)
	}
	serverCert, err := issueScyllaMTLSCertificate(caCert, caKey, &serverKey.PublicKey, "localhost", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, []string{"localhost"}, []net.IP{net.ParseIP("127.0.0.1")})
	if err != nil {
		return scyllaMTLSCertificates{}, fmt.Errorf("issue server certificate: %w", err)
	}

	clientKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return scyllaMTLSCertificates{}, fmt.Errorf("generate client key: %w", err)
	}
	clientCert, err := issueScyllaMTLSCertificate(caCert, caKey, &clientKey.PublicKey, "gorest-scylla-client", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, nil, nil)
	if err != nil {
		return scyllaMTLSCertificates{}, fmt.Errorf("issue client certificate: %w", err)
	}

	return scyllaMTLSCertificates{
		caPEM:         encodeScyllaCertificate(caDER),
		serverCertPEM: encodeScyllaCertificate(serverCert),
		serverKeyPEM:  encodeScyllaPrivateKey(serverKey),
		clientCertPEM: encodeScyllaCertificate(clientCert),
		clientKeyPEM:  encodeScyllaPrivateKey(clientKey),
	}, nil
}

func issueScyllaMTLSCertificate(
	ca *x509.Certificate,
	caKey *rsa.PrivateKey,
	publicKey *rsa.PublicKey,
	commonName string,
	usage []x509.ExtKeyUsage,
	dnsNames []string,
	ipAddresses []net.IP,
) ([]byte, error) {
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("generate certificate serial: %w", err)
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(24 * time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  usage,
		DNSNames:     dnsNames,
		IPAddresses:  ipAddresses,
	}
	return x509.CreateCertificate(rand.Reader, template, ca, publicKey, caKey)
}

func encodeScyllaCertificate(der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func encodeScyllaPrivateKey(key *rsa.PrivateKey) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
}

func writeScyllaMTLSClientFiles(t *testing.T, certificates scyllaMTLSCertificates) (string, string, string) {
	t.Helper()
	dir := t.TempDir()
	caPath := filepath.Join(dir, "ca.pem")
	clientCertPath := filepath.Join(dir, "client.crt")
	clientKeyPath := filepath.Join(dir, "client.key")
	for _, file := range []struct {
		path string
		data []byte
	}{
		{path: caPath, data: certificates.caPEM},
		{path: clientCertPath, data: certificates.clientCertPEM},
		{path: clientKeyPath, data: certificates.clientKeyPEM},
	} {
		if err := os.WriteFile(file.path, file.data, 0o600); err != nil {
			t.Fatalf("write ScyllaDB mTLS test file: %v", err)
		}
	}
	return caPath, clientCertPath, clientKeyPath
}

func installScyllaMTLSConfig(ctx context.Context, container testcontainers.Container, certificates scyllaMTLSCertificates) error {
	exitCode, output, err := container.Exec(ctx, []string{"mkdir", "-p", "/etc/scylla/ssl"})
	if err != nil {
		return fmt.Errorf("create certificate directory (exit %d): %w", exitCode, err)
	}
	if exitCode != 0 {
		body, _ := io.ReadAll(output)
		return fmt.Errorf("create certificate directory exited %d: %s", exitCode, body)
	}
	for _, file := range []struct {
		path string
		data []byte
		mode int64
	}{
		{path: "/etc/scylla/ssl/ca.crt", data: certificates.caPEM, mode: 0o644},
		{path: "/etc/scylla/ssl/server.crt", data: certificates.serverCertPEM, mode: 0o644},
		{path: "/etc/scylla/ssl/server.key", data: certificates.serverKeyPEM, mode: 0o600},
	} {
		if err := container.CopyToContainer(ctx, file.data, file.path, file.mode); err != nil {
			return fmt.Errorf("copy ScyllaDB mTLS file %s: %w", filepath.Base(file.path), err)
		}
	}
	exitCode, output, err = container.Exec(ctx, []string{
		"sh", "-c",
		"chown scylla:scylla /etc/scylla/ssl/server.key && chmod 600 /etc/scylla/ssl/server.key",
	})
	if err != nil {
		return fmt.Errorf("set ScyllaDB server key permissions (exit %d): %w", exitCode, err)
	}
	if exitCode != 0 {
		body, _ := io.ReadAll(output)
		return fmt.Errorf("set ScyllaDB server key permissions exited %d: %s", exitCode, body)
	}

	configFile, err := container.CopyFileFromContainer(ctx, "/etc/scylla/scylla.yaml")
	if err != nil {
		return fmt.Errorf("read ScyllaDB configuration: %w", err)
	}
	config, readErr := io.ReadAll(configFile)
	closeErr := configFile.Close()
	if readErr != nil {
		return fmt.Errorf("read ScyllaDB configuration content: %w", readErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close ScyllaDB configuration reader: %w", closeErr)
	}
	config = append(config, []byte(`

client_encryption_options:
  enabled: true
  certificate: /etc/scylla/ssl/server.crt
  keyfile: /etc/scylla/ssl/server.key
  truststore: /etc/scylla/ssl/ca.crt
  require_client_auth: true
`)...)
	if err := container.CopyToContainer(ctx, config, "/etc/scylla/scylla.yaml", 0o644); err != nil {
		return fmt.Errorf("install ScyllaDB mTLS configuration: %w", err)
	}
	return nil
}
