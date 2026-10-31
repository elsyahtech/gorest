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
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/elsyahtech/gorest/database"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func TestMongoMTLSManaged(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	image := os.Getenv("GOREST_IT_MONGO_IMAGE")
	if image == "" {
		image = mongoImageDefault
	}
	certificates, err := newMongoMTLSCertificates()
	if err != nil {
		t.Fatalf("generate MongoDB mTLS test certificates: %v", err)
	}
	caPath, clientCertPath, clientKeyPath := writeMongoMTLSClientFiles(t, certificates)
	serverKeyPair := append(append([]byte(nil), certificates.serverCertPEM...), certificates.serverKeyPEM...)

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image: image,
			ExposedPorts: []string{
				"27017/tcp",
			},
			Files: []testcontainers.ContainerFile{
				{Reader: bytes.NewReader(certificates.caPEM), ContainerFilePath: "/tmp/gorest-mongo-ca.pem", FileMode: 0o644},
				{Reader: bytes.NewReader(serverKeyPair), ContainerFilePath: "/tmp/gorest-mongo-server.pem", FileMode: 0o644},
			},
			Cmd: []string{
				"mongod",
				"--bind_ip_all",
				"--tlsMode=requireTLS",
				"--tlsCertificateKeyFile=/tmp/gorest-mongo-server.pem",
				"--tlsCAFile=/tmp/gorest-mongo-ca.pem",
			},
			WaitingFor: wait.ForLog("Waiting for connections").WithStartupTimeout(3 * time.Minute),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start MongoDB mTLS test container using %s: %v", image, err)
	}
	t.Cleanup(func() {
		terminateCtx, terminateCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer terminateCancel()
		if err := container.Terminate(terminateCtx); err != nil {
			t.Errorf("terminate MongoDB mTLS test container: %v", err)
		}
	})

	port, err := container.MappedPort(ctx, "27017/tcp")
	if err != nil {
		t.Fatalf("get MongoDB mTLS mapped port: %v", err)
	}
	baseConfig := database.Config{
		Driver:    database.MONGO,
		Host:      "localhost",
		Port:      port.Port(),
		Name:      "admin",
		TLS:       "true",
		SSLCAPath: caPath,
		Timeout:   10 * time.Second,
	}

	noCertCtx, cancelNoCert := context.WithTimeout(ctx, 15*time.Second)
	_, _, err = database.New(noCertCtx, &baseConfig, "UTC")
	cancelNoCert()
	if err == nil {
		t.Fatal("managed MongoDB accepted a TLS connection without a required client certificate")
	}

	clientConfig := baseConfig
	clientConfig.SSLCertPath = clientCertPath
	clientConfig.SSLKeyPath = clientKeyPath
	clientCtx, cancelClient := context.WithTimeout(ctx, 20*time.Second)
	defer cancelClient()
	db, message, err := database.New(clientCtx, &clientConfig, "UTC")
	if err != nil {
		t.Fatalf("connect to managed MongoDB with a trusted client certificate (TLS setup: %s): %v", message, err)
	}
	defer func() {
		closeCtx, cancelClose := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancelClose()
		if err := db.Mongo.Disconnect(closeCtx); err != nil {
			t.Errorf("disconnect managed MongoDB mTLS client: %v", err)
		}
	}()

	if err := db.Mongo.Ping(clientCtx, nil); err != nil {
		t.Fatalf("ping managed MongoDB over mTLS: %v", err)
	}
	var result struct {
		Version string `bson:"version"`
	}
	if err := db.Mongo.Database("admin").RunCommand(clientCtx, bson.D{{Key: "buildInfo", Value: 1}}).Decode(&result); err != nil {
		t.Fatalf("query managed MongoDB version over mTLS: %v", err)
	}
	t.Logf("Managed MongoDB mTLS verified: server version %s; connection without client certificate rejected", result.Version)
}

type mongoMTLSCertificates struct {
	caPEM         []byte
	serverCertPEM []byte
	serverKeyPEM  []byte
	clientCertPEM []byte
	clientKeyPEM  []byte
}

func newMongoMTLSCertificates() (mongoMTLSCertificates, error) {
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return mongoMTLSCertificates{}, fmt.Errorf("generate test CA key: %w", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "GOREST MongoDB mTLS test CA"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		IsCA:                  true,
		BasicConstraintsValid: true,
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		return mongoMTLSCertificates{}, fmt.Errorf("create test CA certificate: %w", err)
	}
	caCert, err := x509.ParseCertificate(caDER)
	if err != nil {
		return mongoMTLSCertificates{}, fmt.Errorf("parse test CA certificate: %w", err)
	}

	serverCertPEM, serverKeyPEM, err := issueMongoMTLSCertificate(
		caCert, caKey, "localhost", []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, []string{"localhost"},
	)
	if err != nil {
		return mongoMTLSCertificates{}, fmt.Errorf("issue server certificate: %w", err)
	}
	clientCertPEM, clientKeyPEM, err := issueMongoMTLSCertificate(
		caCert, caKey, "gorest-mongo-client", []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, nil,
	)
	if err != nil {
		return mongoMTLSCertificates{}, fmt.Errorf("issue client certificate: %w", err)
	}

	return mongoMTLSCertificates{
		caPEM:         encodeMongoMTLSCertificate(caDER),
		serverCertPEM: serverCertPEM,
		serverKeyPEM:  serverKeyPEM,
		clientCertPEM: clientCertPEM,
		clientKeyPEM:  clientKeyPEM,
	}, nil
}

func issueMongoMTLSCertificate(
	ca *x509.Certificate,
	caKey *ecdsa.PrivateKey,
	commonName string,
	usage []x509.ExtKeyUsage,
	dnsNames []string,
) ([]byte, []byte, error) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("generate certificate key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, fmt.Errorf("generate certificate serial: %w", err)
	}
	template := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  usage,
		DNSNames:     dnsNames,
	}
	certDER, err := x509.CreateCertificate(rand.Reader, template, ca, &key.PublicKey, caKey)
	if err != nil {
		return nil, nil, fmt.Errorf("sign certificate: %w", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal certificate private key: %w", err)
	}
	return encodeMongoMTLSCertificate(certDER), pem.EncodeToMemory(&pem.Block{
		Type: "PRIVATE KEY", Bytes: keyDER,
	}), nil
}

func encodeMongoMTLSCertificate(der []byte) []byte {
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
}

func writeMongoMTLSClientFiles(t *testing.T, certificates mongoMTLSCertificates) (string, string, string) {
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
			t.Fatalf("write MongoDB mTLS client file: %v", err)
		}
	}
	return caPath, clientCertPath, clientKeyPath
}
