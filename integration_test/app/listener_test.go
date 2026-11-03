//go:build integration

package app_test

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	gorest "github.com/elsyahtech/gorest"
	"github.com/elsyahtech/gorest/server"
	"github.com/gofiber/fiber/v3"
)

const (
	listenerHelperEnv        = "GOREST_IT_LISTENER_HELPER"
	listenerNetworkEnv       = "GOREST_IT_LISTENER_NETWORK"
	listenerPortEnv          = "GOREST_IT_LISTENER_PORT"
	listenerSocketPathEnv    = "GOREST_IT_LISTENER_SOCKET_PATH"
	listenerCertFileEnv      = "GOREST_IT_LISTENER_CERT_FILE"
	listenerKeyFileEnv       = "GOREST_IT_LISTENER_KEY_FILE"
	listenerClientCAEnv      = "GOREST_IT_LISTENER_CLIENT_CA_FILE"
	listenerStartedFileEnv   = "GOREST_IT_LISTENER_STARTED_FILE"
	listenerPreforkEnv       = "GOREST_IT_LISTENER_PREFORK"
	listenerPrintRoutesEnv   = "GOREST_IT_LISTENER_PRINT_ROUTES"
	listenerTLSMinVersionEnv = "GOREST_IT_LISTENER_TLS_MIN_VERSION"
)

func TestGORESTListenerConfigurationE2E(t *testing.T) {
	if os.Getenv(listenerHelperEnv) == "1" {
		runListenerAppHelper(t)
		return
	}

	t.Run("TLS and mutual TLS", testListenerTLSAndMutualTLS)
	t.Run("Unix socket", testListenerUnixSocket)
	t.Run("prefork", func(t *testing.T) {
		if runtime.GOOS != "darwin" && runtime.GOOS != "linux" {
			t.Skip("GOREST-managed prefork end-to-end testing is supported on macOS and Linux")
		}
		testListenerPrefork(t)
	})
}

func testListenerTLSAndMutualTLS(t *testing.T) {
	certificates := createListenerCertificates(t)
	port := reserveListenerPort(t)
	startedFile := filepath.Join(t.TempDir(), "slow-request-started")
	process := startListenerApp(t, map[string]string{
		listenerNetworkEnv:       fiber.NetworkTCP4,
		listenerPortEnv:          strconv.Itoa(port),
		listenerCertFileEnv:      certificates.serverCertFile,
		listenerKeyFileEnv:       certificates.serverKeyFile,
		listenerClientCAEnv:      certificates.caCertFile,
		listenerStartedFileEnv:   startedFile,
		listenerTLSMinVersionEnv: strconv.Itoa(tls.VersionTLS13),
		listenerPrintRoutesEnv:   "1",
	})
	t.Cleanup(process.forceStop)

	baseURL := fmt.Sprintf("https://127.0.0.1:%d", port)
	client := listenerHTTPClient(t, certificates, true)
	waitForListenerHTTP(t, process, client, baseURL+"/listener-ready", http.StatusOK)
	unauthenticatedClient := listenerHTTPClient(t, certificates, false)
	if _, err := unauthenticatedClient.Get(baseURL + "/listener-ready"); err == nil {
		t.Fatal("TLS listener accepted a client without a certificate")
	}

	response := getListenerResponse(t, client, baseURL+"/listener-ready")
	assertListenerResponse(t, response, http.StatusOK, "ready")

	if !strings.Contains(process.output.String(), "/listener-ready") {
		t.Errorf("Fiber route output does not contain the test route:\n%s", process.output.String())
	}

	finishListenerShutdown(t, process, client, baseURL, startedFile)
}

func testListenerUnixSocket(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix domain sockets are not supported on Windows")
	}

	socketDir, err := os.MkdirTemp("/tmp", "gr-")
	if err != nil {
		t.Fatalf("create short Unix socket directory: %v", err)
	}
	t.Cleanup(func() {
		if err := os.RemoveAll(socketDir); err != nil {
			t.Errorf("remove Unix socket directory: %v", err)
		}
	})
	socketPath := filepath.Join(socketDir, "gorest.sock")
	startedFile := filepath.Join(t.TempDir(), "slow-request-started")
	process := startListenerApp(t, map[string]string{
		listenerNetworkEnv:     fiber.NetworkUnix,
		listenerPortEnv:        "0",
		listenerSocketPathEnv:  socketPath,
		listenerStartedFileEnv: startedFile,
	})
	t.Cleanup(process.forceStop)

	client := unixSocketHTTPClient(socketPath)
	waitForListenerHTTP(t, process, client, "http://unix/listener-ready", http.StatusOK)

	info, err := os.Stat(socketPath)
	if err != nil {
		t.Fatalf("stat Unix socket: %v", err)
	}
	if got, want := info.Mode().Perm(), os.FileMode(0o600); got != want {
		t.Errorf("Unix socket mode = %#o, want %#o", got, want)
	}

	response := getListenerResponse(t, client, "http://unix/listener-ready")
	assertListenerResponse(t, response, http.StatusOK, "ready")

	finishListenerShutdown(t, process, client, "http://unix", startedFile)
}

func testListenerPrefork(t *testing.T) {
	port := reserveListenerPort(t)
	startedFile := filepath.Join(t.TempDir(), "slow-request-started")
	process := startListenerApp(t, map[string]string{
		listenerNetworkEnv:     fiber.NetworkTCP4,
		listenerPortEnv:        strconv.Itoa(port),
		listenerPreforkEnv:     "1",
		listenerPrintRoutesEnv: "1",
		listenerStartedFileEnv: startedFile,
		"GOMAXPROCS":           "2",
	})
	t.Cleanup(process.forceStop)

	baseURL := fmt.Sprintf("http://127.0.0.1:%d", port)
	client := &http.Client{Timeout: 5 * time.Second}
	waitForListenerHTTP(t, process, client, baseURL+"/listener-ready", http.StatusOK)
	response := getListenerResponse(t, client, baseURL+"/listener-ready")
	assertListenerResponse(t, response, http.StatusOK, "ready")
	if !strings.Contains(process.output.String(), "/listener-ready") {
		t.Errorf("prefork route output does not contain the test route:\n%s", process.output.String())
	}

	crashClient := &http.Client{
		Timeout:   5 * time.Second,
		Transport: &http.Transport{DisableKeepAlives: true},
	}
	if response, err := crashClient.Get(baseURL + "/listener-crash-worker"); err == nil {
		response.Body.Close()
		t.Fatal("crash endpoint unexpectedly returned a response")
	}
	waitForListenerOutput(t, process, "restart 1 of 2")
	waitForListenerHTTP(t, process, client, baseURL+"/listener-ready", http.StatusOK)

	finishListenerShutdown(t, process, client, baseURL, startedFile)

	logFiles, err := os.ReadDir(filepath.Join(process.workDir, "logs"))
	if err != nil {
		t.Fatalf("read prefork log directory: %v", err)
	}
	workerLogFiles := 0
	hasMasterLog := false
	for _, file := range logFiles {
		if file.Name() == "app.log" {
			hasMasterLog = true
		}
		if strings.HasPrefix(file.Name(), "app-") && strings.HasSuffix(file.Name(), ".log") {
			workerLogFiles++
		}
	}
	if !hasMasterLog || workerLogFiles < 2 {
		t.Fatalf("prefork log files = %v, want the master log and separate files for at least two workers", logFiles)
	}
}

func runListenerAppHelper(t *testing.T) {
	port, err := strconv.Atoi(os.Getenv(listenerPortEnv))
	if err != nil {
		t.Fatalf("parse listener port: %v", err)
	}

	listener := server.ConfigDefault.Listener
	listener.Network = os.Getenv(listenerNetworkEnv)
	listener.UnixSocketPath = os.Getenv(listenerSocketPathEnv)
	listener.TLSCertFile = os.Getenv(listenerCertFileEnv)
	listener.TLSCertKeyFile = os.Getenv(listenerKeyFileEnv)
	listener.TLSCertClientFile = os.Getenv(listenerClientCAEnv)
	listener.ShutdownTimeout = 5 * time.Second
	listener.UnixSocketFileMode = 0o600
	listener.EnablePrefork = os.Getenv(listenerPreforkEnv) == "1"
	listener.EnablePrintRoutes = os.Getenv(listenerPrintRoutesEnv) == "1"
	listener.PreforkRecoverInterval = 100 * time.Millisecond
	listener.PreforkShutdownGracePeriod = 2 * time.Second
	listener.PreforkRecoverThreshold = 2
	if version, parseErr := strconv.ParseUint(os.Getenv(listenerTLSMinVersionEnv), 10, 16); parseErr == nil {
		listener.TLSMinVersion = uint16(version)
	}

	serverConfig := server.ConfigDefault
	serverConfig.Host = "127.0.0.1"
	serverConfig.Port = port
	serverConfig.Listener = listener

	application := gorest.New(gorest.Config{CheckForUpdates: gorest.DisableAutoUpdate})
	application.Use(serverConfig)
	application.RegisterServices([]gorest.RouterRegistrar{
		func(app *gorest.App) {
			app.Get("/listener-ready", func(ctx *gorest.Context) error {
				return ctx.SendString("ready")
			})
			app.Get("/listener-crash-worker", func(*gorest.Context) error {
				os.Exit(73)
				return nil
			})
			app.Get("/listener-slow", func(ctx *gorest.Context) error {
				if err := os.WriteFile(os.Getenv(listenerStartedFileEnv), []byte("started"), 0o600); err != nil {
					return err
				}
				time.Sleep(750 * time.Millisecond)
				return ctx.SendString("completed")
			})
		},
	})
	application.Start()
}

type listenerProcess struct {
	command *exec.Cmd
	output  *listenerOutput
	workDir string
}

type listenerOutput struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (output *listenerOutput) Write(value []byte) (int, error) {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.buffer.Write(value)
}

func (output *listenerOutput) String() string {
	output.mu.Lock()
	defer output.mu.Unlock()
	return output.buffer.String()
}

func startListenerApp(t *testing.T, values map[string]string) *listenerProcess {
	t.Helper()

	workDir := t.TempDir()
	output := &listenerOutput{}
	command := exec.CommandContext(
		listenerTestContext(t, 30*time.Second),
		os.Args[0],
		"-test.run=^TestGORESTListenerConfigurationE2E$",
	)
	command.Dir = workDir
	command.Stdout = output
	command.Stderr = output
	command.Env = setListenerTestEnv(os.Environ(), listenerHelperEnv, "1")
	for key, value := range values {
		command.Env = setListenerTestEnv(command.Env, key, value)
	}
	if err := command.Start(); err != nil {
		t.Fatalf("start GOREST listener helper: %v", err)
	}

	return &listenerProcess{command: command, output: output, workDir: workDir}
}

func setListenerTestEnv(environment []string, key, value string) []string {
	prefix := key + "="
	updated := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if !strings.HasPrefix(entry, prefix) {
			updated = append(updated, entry)
		}
	}
	return append(updated, prefix+value)
}

func (process *listenerProcess) forceStop() {
	if process.command.ProcessState != nil {
		return
	}

	_ = process.command.Process.Kill()
	_ = process.command.Wait()
}

func finishListenerShutdown(
	t *testing.T,
	process *listenerProcess,
	client *http.Client,
	baseURL,
	startedFile string,
) {
	t.Helper()

	responseDone := make(chan error, 1)
	go func() {
		response, err := client.Get(baseURL + "/listener-slow")
		if err != nil {
			responseDone <- err
			return
		}
		defer response.Body.Close()
		body, err := readListenerBody(response)
		if err != nil {
			responseDone <- err
			return
		}
		if response.StatusCode != http.StatusOK || string(body) != "completed" {
			responseDone <- fmt.Errorf("slow response = (%d, %q), want (200, completed)", response.StatusCode, body)
			return
		}
		responseDone <- nil
	}()
	waitForListenerFile(t, process, startedFile)

	if err := process.command.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatalf("send SIGTERM to GOREST listener helper: %v\n%s", err, process.output.String())
	}

	select {
	case err := <-responseDone:
		if err != nil {
			t.Fatalf("in-flight request did not complete during shutdown: %v\n%s", err, process.output.String())
		}
	case <-time.After(4 * time.Second):
		t.Fatalf("in-flight request did not finish during configured shutdown timeout\n%s", process.output.String())
	}

	waitDone := make(chan error, 1)
	go func() { waitDone <- process.command.Wait() }()
	select {
	case err := <-waitDone:
		if err != nil {
			t.Fatalf("GOREST listener helper exited with error: %v\n%s", err, process.output.String())
		}
	case <-time.After(8 * time.Second):
		t.Fatalf("GOREST listener helper did not exit after SIGTERM\n%s", process.output.String())
	}

	if !strings.Contains(process.output.String(), "Application closed successfully") {
		t.Errorf("helper did not report graceful shutdown:\n%s", process.output.String())
	}
}

func waitForListenerHTTP(
	t *testing.T,
	process *listenerProcess,
	client *http.Client,
	endpoint string,
	wantStatus int,
) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		response, err := client.Get(endpoint)
		if err == nil {
			response.Body.Close()
			if response.StatusCode == wantStatus {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf("listener endpoint %s did not return HTTP %d\n%s", endpoint, wantStatus, process.output.String())
}

func getListenerResponse(t *testing.T, client *http.Client, endpoint string) *http.Response {
	t.Helper()
	response, err := client.Get(endpoint)
	if err != nil {
		t.Fatalf("GET %s: %v", endpoint, err)
	}
	return response
}

func assertListenerResponse(t *testing.T, response *http.Response, wantStatus int, wantBody string) {
	t.Helper()
	defer response.Body.Close()

	body, err := readListenerBody(response)
	if err != nil {
		t.Fatalf("read listener response: %v", err)
	}
	if response.StatusCode != wantStatus || string(body) != wantBody {
		t.Fatalf("listener response = (%d, %q), want (%d, %q)", response.StatusCode, body, wantStatus, wantBody)
	}
}

func readListenerBody(response *http.Response) ([]byte, error) {
	return io.ReadAll(response.Body)
}

func waitForListenerFile(t *testing.T, process *listenerProcess, path string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("slow handler did not create marker %s\n%s", path, process.output.String())
}

func waitForListenerOutput(t *testing.T, process *listenerProcess, text string) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(process.output.String(), text) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("listener output did not contain %q\n%s", text, process.output.String())
}

func reserveListenerPort(t *testing.T) int {
	t.Helper()

	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve listener port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatalf("release listener port: %v", err)
	}
	return port
}

func listenerHTTPClient(t *testing.T, certificates listenerCertificates, withClientCertificate bool) *http.Client {
	t.Helper()

	caPEM, err := os.ReadFile(certificates.caCertFile)
	if err != nil {
		t.Fatalf("read test CA certificate: %v", err)
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(caPEM) {
		t.Fatal("append test CA certificate")
	}

	tlsConfig := &tls.Config{
		RootCAs:    roots,
		MinVersion: tls.VersionTLS13,
	}
	if withClientCertificate {
		clientCertificate, err := tls.LoadX509KeyPair(certificates.clientCertFile, certificates.clientKeyFile)
		if err != nil {
			t.Fatalf("load test client certificate: %v", err)
		}
		tlsConfig.Certificates = []tls.Certificate{clientCertificate}
	}

	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: tlsConfig,
		},
	}
}

func unixSocketHTTPClient(socketPath string) *http.Client {
	return &http.Client{
		Timeout: 5 * time.Second,
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				return (&net.Dialer{}).DialContext(ctx, "unix", socketPath)
			},
		},
	}
}

type listenerCertificates struct {
	caCertFile     string
	serverCertFile string
	serverKeyFile  string
	clientCertFile string
	clientKeyFile  string
}

func createListenerCertificates(t *testing.T) listenerCertificates {
	t.Helper()

	directory := t.TempDir()
	now := time.Now()
	caKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate test CA key: %v", err)
	}
	caTemplate := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "GOREST listener integration test CA"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, &caKey.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create test CA certificate: %v", err)
	}
	caCert := mustParseListenerCertificate(t, caDER)
	caCertFile := filepath.Join(directory, "ca.crt")
	writeListenerPEM(t, caCertFile, "CERTIFICATE", caDER)

	serverCertFile, serverKeyFile := createListenerLeafCertificate(t, directory, caCert, caKey, "server", x509.ExtKeyUsageServerAuth)
	clientCertFile, clientKeyFile := createListenerLeafCertificate(t, directory, caCert, caKey, "client", x509.ExtKeyUsageClientAuth)

	return listenerCertificates{
		caCertFile:     caCertFile,
		serverCertFile: serverCertFile,
		serverKeyFile:  serverKeyFile,
		clientCertFile: clientCertFile,
		clientKeyFile:  clientKeyFile,
	}
}

func createListenerLeafCertificate(
	t *testing.T,
	directory string,
	caCert *x509.Certificate,
	caKey *ecdsa.PrivateKey,
	name string,
	usage x509.ExtKeyUsage,
) (string, string) {
	t.Helper()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate %s key: %v", name, err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(int64(len(name) + 10)),
		Subject:      pkix.Name{CommonName: name},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{usage},
	}
	if usage == x509.ExtKeyUsageServerAuth {
		template.DNSNames = []string{"localhost"}
		template.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	}
	certificateDER, err := x509.CreateCertificate(rand.Reader, template, caCert, &key.PublicKey, caKey)
	if err != nil {
		t.Fatalf("create %s certificate: %v", name, err)
	}

	certFile := filepath.Join(directory, name+".crt")
	keyFile := filepath.Join(directory, name+".key")
	writeListenerPEM(t, certFile, "CERTIFICATE", certificateDER)
	privateKeyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal %s key: %v", name, err)
	}
	writeListenerPEM(t, keyFile, "PRIVATE KEY", privateKeyDER)
	return certFile, keyFile
}

func mustParseListenerCertificate(t *testing.T, der []byte) *x509.Certificate {
	t.Helper()
	certificate, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse generated CA certificate: %v", err)
	}
	return certificate
}

func writeListenerPEM(t *testing.T, path, blockType string, contents []byte) {
	t.Helper()
	block := pem.EncodeToMemory(&pem.Block{Type: blockType, Bytes: contents})
	if err := os.WriteFile(path, block, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func listenerTestContext(t *testing.T, timeout time.Duration) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	t.Cleanup(cancel)
	return ctx
}
