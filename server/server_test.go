package server

import (
	"bytes"
	"context"
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	gorestlog "github.com/elsyahtech/gorest/log"
	"github.com/gofiber/fiber/v3"
)

func routeNameTestHandler() {}

func TestRouteNameReturnsRegisteredFunctionName(t *testing.T) {
	if got := routeName(routeNameTestHandler); !strings.HasSuffix(got, "routeNameTestHandler") {
		t.Fatalf("routeName() = %q, want function name suffix", got)
	}
}

func TestNewAppliesDefaultsForNilAndEmptyConfig(t *testing.T) {
	for _, test := range []struct {
		config *Config
		name   string
	}{
		{name: "nil config"},
		{name: "empty config", config: &Config{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			server, _, err := New(test.config)
			if err != nil {
				t.Fatalf("New() error = %v", err)
			}

			coreConfig := server.SRV.Config()
			if coreConfig.ReadTimeout != DefaultServerReadTimeout ||
				coreConfig.WriteTimeout != DefaultServerWriteTimeout ||
				coreConfig.IdleTimeout != DefaultServerIdleTimeout {
				t.Errorf(
					"New() timeouts = (%s, %s, %s), want (%s, %s, %s)",
					coreConfig.ReadTimeout,
					coreConfig.WriteTimeout,
					coreConfig.IdleTimeout,
					DefaultServerReadTimeout,
					DefaultServerWriteTimeout,
					DefaultServerIdleTimeout,
				)
			}
			if server.shutdownTimeout != DefaultServerShutdownTimeout {
				t.Errorf("shutdown timeout = %s, want %s", server.shutdownTimeout, DefaultServerShutdownTimeout)
			}

			server.SRV.Get("/", func(c fiber.Ctx) error {
				return c.SendString("ok")
			})

			response, err := server.SRV.Test(httptest.NewRequest(http.MethodGet, "/", nil))
			if err != nil {
				t.Fatalf("Test() error = %v", err)
			}
			defer response.Body.Close()

			if got := response.Header.Get("X-Content-Type-Options"); got != "nosniff" {
				t.Errorf("X-Content-Type-Options = %q, want %q", got, "nosniff")
			}
			if got := response.Header.Get("X-Request-ID"); got == "" {
				t.Error("expected default request ID middleware to set X-Request-ID")
			}
		})
	}
}

func TestConfigDefaultAppliesFiniteTimeoutsWhenCoreIsNil(t *testing.T) {
	config := configDefault(Config{})
	if config.Core == nil {
		t.Fatal("expected default Fiber core configuration")
	}

	if config.Core.ReadTimeout != DefaultServerReadTimeout || config.Core.ReadTimeout <= 0 {
		t.Errorf("ReadTimeout = %s, want finite default %s", config.Core.ReadTimeout, DefaultServerReadTimeout)
	}
	if config.Core.WriteTimeout != DefaultServerWriteTimeout || config.Core.WriteTimeout <= 0 {
		t.Errorf("WriteTimeout = %s, want finite default %s", config.Core.WriteTimeout, DefaultServerWriteTimeout)
	}
	if config.Core.IdleTimeout != DefaultServerIdleTimeout || config.Core.IdleTimeout <= 0 {
		t.Errorf("IdleTimeout = %s, want finite default %s", config.Core.IdleTimeout, DefaultServerIdleTimeout)
	}
}

func TestListenerConfigDefaultsAndFiberMapping(t *testing.T) {
	defaults := configDefault(Config{}).Listener
	if defaults.Network != fiber.NetworkTCP4 ||
		defaults.TLSMinVersion != tls.VersionTLS12 ||
		defaults.UnixSocketFileMode != 0o770 ||
		defaults.ShutdownTimeout != DefaultServerShutdownTimeout {
		t.Fatalf("listener defaults = %#v", defaults)
	}

	listener := ListenerConfig{
		TLSCertFile:                "server.crt",
		TLSCertKeyFile:             "server.key",
		TLSCertClientFile:          "client-ca.crt",
		TLSMinVersion:              tls.VersionTLS13,
		EnablePrefork:              true,
		Network:                    fiber.NetworkTCP6,
		UnixSocketFileMode:         0o700,
		ShutdownTimeout:            15 * time.Second,
		PreforkRecoverInterval:     time.Second,
		PreforkShutdownGracePeriod: 2 * time.Second,
		PreforkRecoverThreshold:    3,
		EnablePrintRoutes:          true,
	}
	mapped := listener.fiberListenConfig()
	if mapped.CertFile != listener.TLSCertFile ||
		mapped.CertKeyFile != listener.TLSCertKeyFile ||
		mapped.CertClientFile != listener.TLSCertClientFile ||
		mapped.TLSMinVersion != listener.TLSMinVersion ||
		!mapped.EnablePrefork ||
		mapped.ListenerNetwork != listener.Network ||
		mapped.UnixSocketFileMode != listener.UnixSocketFileMode ||
		mapped.ShutdownTimeout != listener.ShutdownTimeout ||
		mapped.PreforkRecoverInterval != listener.PreforkRecoverInterval ||
		mapped.PreforkShutdownGracePeriod != listener.PreforkShutdownGracePeriod ||
		mapped.PreforkRecoverThreshold != listener.PreforkRecoverThreshold ||
		!mapped.EnablePrintRoutes ||
		!mapped.DisableStartupMessage {
		t.Fatalf("Fiber listener mapping = %#v", mapped)
	}
}

func TestValidateListenerConfig(t *testing.T) {
	tests := []struct {
		name    string
		config  ListenerConfig
		wantErr bool
	}{
		{name: "default TCP listener", config: configDefault(Config{}).Listener},
		{
			name: "Unix listener requires a socket path",
			config: ListenerConfig{
				Network:       fiber.NetworkUnix,
				TLSMinVersion: tls.VersionTLS12,
			},
			wantErr: true,
		},
		{
			name: "Unix listener with socket path",
			config: ListenerConfig{
				Network:        fiber.NetworkUnix,
				UnixSocketPath: "/tmp/gorest.sock",
				TLSMinVersion:  tls.VersionTLS12,
			},
		},
		{
			name: "certificate and key must be paired",
			config: ListenerConfig{
				Network:       fiber.NetworkTCP4,
				TLSCertFile:   "server.crt",
				TLSMinVersion: tls.VersionTLS12,
			},
			wantErr: true,
		},
		{
			name: "client CA requires server TLS",
			config: ListenerConfig{
				Network:           fiber.NetworkTCP4,
				TLSCertClientFile: "client-ca.crt",
				TLSMinVersion:     tls.VersionTLS12,
			},
			wantErr: true,
		},
		{
			name: "unsupported TLS version",
			config: ListenerConfig{
				Network:       fiber.NetworkTCP4,
				TLSMinVersion: tls.VersionTLS11,
			},
			wantErr: true,
		},
		{
			name: "prefork requires TCP4 or TCP6",
			config: ListenerConfig{
				Network:       fiber.NetworkTCP,
				EnablePrefork: true,
				TLSMinVersion: tls.VersionTLS12,
			},
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := listenerConfigDefault(test.config)
			err := validateListenerConfig(config)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateListenerConfig() error = %v, wantErr %t", err, test.wantErr)
			}
		})
	}
}

func TestValidatePreforkPlatform(t *testing.T) {
	config := configDefault(Config{Listener: ListenerConfig{EnablePrefork: true}}).Listener
	err := validateListenerConfig(config)
	supported := runtime.GOOS == "darwin" || runtime.GOOS == "linux"
	if supported && err != nil {
		t.Fatalf("validateListenerConfig() error = %v on supported platform %s", err, runtime.GOOS)
	}
	if !supported && err == nil {
		t.Fatalf("validateListenerConfig() error = nil on unsupported platform %s", runtime.GOOS)
	}
}

func TestPreforkSupervisorIgnoresInvalidLocalRegistration(t *testing.T) {
	supervisor, err := newPreforkSupervisor(&Server{}, ListenerConfig{})
	if err != nil {
		t.Fatalf("newPreforkSupervisor() error = %v", err)
	}
	go supervisor.acceptWorkers()

	connection, err := net.Dial("tcp4", supervisor.control.Addr().String())
	if err != nil {
		t.Fatalf("dial supervisor control listener: %v", err)
	}
	if _, err := connection.Write([]byte(strings.Repeat("x", preforkRegistrationLimit+1))); err != nil {
		t.Fatalf("write invalid registration: %v", err)
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("close invalid registration connection: %v", err)
	}

	select {
	case err := <-supervisor.failures:
		t.Fatalf("invalid local registration caused supervisor failure: %v", err)
	case registration := <-supervisor.ready:
		t.Fatalf("invalid local registration was accepted: %#v", registration)
	case <-time.After(100 * time.Millisecond):
	}

	if err := supervisor.control.Close(); err != nil {
		t.Fatalf("close supervisor control listener: %v", err)
	}
	close(supervisor.done)
}

func TestConfigDefaultPreservesExplicitCoreTimeouts(t *testing.T) {
	core := &fiber.Config{
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 0,
		IdleTimeout:  15 * time.Second,
	}

	config := configDefault(Config{Core: core})
	if config.Core != core {
		t.Fatal("expected explicit Fiber core configuration to be preserved")
	}
	if config.Core.ReadTimeout != core.ReadTimeout {
		t.Errorf("ReadTimeout = %s, want %s", config.Core.ReadTimeout, core.ReadTimeout)
	}
	if config.Core.WriteTimeout != 0 {
		t.Errorf("WriteTimeout = %s, want explicit zero", config.Core.WriteTimeout)
	}
	if config.Core.IdleTimeout != core.IdleTimeout {
		t.Errorf("IdleTimeout = %s, want %s", config.Core.IdleTimeout, core.IdleTimeout)
	}
}

func TestRunAndStartServerInputValidation(t *testing.T) {
	if got := Run(); got.Port != DefaultServerPort || got.Host != DefaultServerHost {
		t.Fatalf("Run() = %#v, want default host and port", got)
	}
	config := Config{Port: 8081, Host: "0.0.0.0"}
	if got := Run(config); got.Port != config.Port || got.Host != config.Host {
		t.Fatalf("Run(config) = %#v, want configured host and port", got)
	}

	if message, err := StartServer(nil, &config, nil, "", "", "", "", nil); err == nil || message == "" {
		t.Fatalf("StartServer(nil) = (%q, %v), want an initialization error", message, err)
	}

	server, _, err := New(&config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if message, err := StartServer(server, &config, nil, "", "", "", "", nil); err == nil || message == "" {
		t.Fatalf("StartServer(nil logger) = (%q, %v), want a logger error", message, err)
	}

	if _, _, err := New(&Config{Listener: ListenerConfig{Network: "udp"}}); err == nil {
		t.Fatal("New() error = nil for unsupported listener network")
	}
}

func TestNewWarnsWhenExplicitCoreHasUnboundedTimeouts(t *testing.T) {
	var logs bytes.Buffer
	previousWriter := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previousWriter)

	core := &fiber.Config{
		ReadTimeout:  0,
		WriteTimeout: 0,
		IdleTimeout:  0,
	}
	server, _, err := New(&Config{Core: core})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if server.SRV.Config().ReadTimeout != 0 || server.SRV.Config().WriteTimeout != 0 {
		t.Fatal("New() did not preserve explicit zero timeout values")
	}
	for _, timeout := range []string{"ReadTimeout", "WriteTimeout"} {
		if !strings.Contains(logs.String(), timeout) {
			t.Errorf("warning %q does not identify %s: %q", logs.String(), timeout, logs.String())
		}
	}
	if !strings.Contains(logs.String(), "slow request bodies") {
		t.Errorf("warning does not describe slow-client risk: %q", logs.String())
	}
}

func TestNewDoesNotWarnForZeroIdleTimeoutWithFiniteReadAndWrite(t *testing.T) {
	var logs bytes.Buffer
	previousWriter := log.Writer()
	log.SetOutput(&logs)
	defer log.SetOutput(previousWriter)

	_, _, err := New(&Config{Core: &fiber.Config{
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
		IdleTimeout:  0,
	}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	if logs.Len() != 0 {
		t.Errorf("unexpected warning for finite read/write timeouts: %q", logs.String())
	}
}

func TestReadTimeoutClosesSlowRequestBody(t *testing.T) {
	const readTimeout = 250 * time.Millisecond
	server, _, err := New(&Config{Core: &fiber.Config{
		ReadTimeout:  readTimeout,
		WriteTimeout: time.Second,
		IdleTimeout:  time.Second,
		BodyLimit:    1024,
	}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	handlerCalled := make(chan struct{}, 1)
	server.SRV.Post("/slow", func(c fiber.Ctx) error {
		handlerCalled <- struct{}{}
		return c.SendString("unexpected")
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	listenerDone := make(chan error, 1)
	go func() {
		listenerDone <- server.SRV.Listener(listener, fiber.ListenConfig{DisableStartupMessage: true})
	}()
	defer func() {
		_ = server.SRV.Shutdown()
		select {
		case <-listenerDone:
		case <-time.After(2 * time.Second):
			t.Error("Fiber listener did not stop after shutdown")
		}
	}()

	conn, err := net.DialTimeout("tcp", listener.Addr().String(), time.Second)
	if err != nil {
		t.Fatalf("net.DialTimeout() error = %v", err)
	}
	defer conn.Close()

	if _, err := io.WriteString(conn,
		"POST /slow HTTP/1.1\r\nHost: localhost\r\nContent-Length: 10\r\n\r\nx",
	); err != nil {
		t.Fatalf("writing partial request failed: %v", err)
	}
	if err := conn.SetReadDeadline(time.Now().Add(3 * time.Second)); err != nil {
		t.Fatalf("SetReadDeadline() error = %v", err)
	}

	response, err := io.ReadAll(conn)
	if err != nil {
		if netErr, ok := err.(net.Error); !ok || !netErr.Timeout() {
			t.Fatalf("reading response failed: %v", err)
		}
	}
	if !strings.Contains(string(response), "408 Request Timeout") {
		t.Fatalf("response = %q, want 408 Request Timeout", response)
	}
	select {
	case <-handlerCalled:
		t.Fatal("handler ran before the incomplete request body was received")
	default:
	}
}

func TestServerAddress(t *testing.T) {
	tests := []struct {
		name string
		host string
		want string
		port int
	}{
		{
			name: "default host",
			port: 3000,
			want: "127.0.0.1:3000",
		},
		{
			name: "configured host",
			host: "0.0.0.0",
			port: 8080,
			want: "0.0.0.0:8080",
		},
		{
			name: "ipv6 host",
			host: "::1",
			port: 8080,
			want: "[::1]:8080",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := serverAddress(test.host, test.port); got != test.want {
				t.Fatalf("serverAddress(%q, %d) = %q, want %q", test.host, test.port, got, test.want)
			}
		})
	}
}

func TestListenerAddressUsesUnixSocketPath(t *testing.T) {
	config := configDefault(Config{
		Host: "0.0.0.0",
		Port: 8080,
		Listener: ListenerConfig{
			Network:        fiber.NetworkUnix,
			UnixSocketPath: "/run/gorest/server.sock",
		},
	})
	if got, want := listenerAddress(config), "/run/gorest/server.sock"; got != want {
		t.Fatalf("listenerAddress() = %q, want %q", got, want)
	}
}

func TestShutdownUsesConfiguredTimeout(t *testing.T) {
	server, _, err := New(&Config{Listener: ListenerConfig{
		ShutdownTimeout: 10 * time.Millisecond,
	}})
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	server.ExpectListen()

	startedAt := time.Now()
	err = server.Shutdown(context.Background())
	if err == nil {
		t.Fatal("Shutdown() error = nil, want timeout while waiting for listener")
	}
	if elapsed := time.Since(startedAt); elapsed < 10*time.Millisecond || elapsed > time.Second {
		t.Errorf("Shutdown() elapsed = %s, want approximately configured timeout", elapsed)
	}
}

func TestWaitForListeningReportsListenerFailure(t *testing.T) {
	server, _, err := New(nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer listener.Close()

	listenDone := make(chan error, 1)
	go func() {
		listenDone <- server.Listen(listener.Addr().String(), fiber.ListenConfig{DisableStartupMessage: true})
	}()

	if err := server.WaitForListening(context.Background()); err == nil {
		t.Fatal("WaitForListening() succeeded despite listener failure")
	}

	select {
	case err := <-listenDone:
		if err == nil {
			t.Fatal("Listen() succeeded despite occupied address")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Listen() did not return after bind failure")
	}
}

func TestWaitForListeningRequiresScheduledListener(t *testing.T) {
	server, _, err := New(nil)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	if err := server.WaitForListening(context.Background()); err == nil {
		t.Fatal("WaitForListening() succeeded before listener was scheduled")
	}
}

func TestStartServerReportsBindFailureAndCompletesListenerLifecycle(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen() error = %v", err)
	}
	defer listener.Close()

	_, portText, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("SplitHostPort() error = %v", err)
	}
	port, err := strconv.Atoi(portText)
	if err != nil {
		t.Fatalf("Atoi(%q) error = %v", portText, err)
	}
	config := &Config{Host: "127.0.0.1", Port: port}
	httpServer, _, err := New(config)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	logger, message, err := gorestlog.New(&gorestlog.Config{
		LogDir:  t.TempDir(),
		LogFile: "server-test.log",
	}, "server-test")
	if err != nil {
		t.Fatalf("log.New() error = %v, message = %q", err, message)
	}
	t.Cleanup(func() {
		if _, _, err := logger.Close(); err != nil {
			t.Errorf("closing test logger: %v", err)
		}
	})

	message, err = StartServer(httpServer, config, logger, "test", "1.0", "UTC", "testing", nil)
	if err == nil || !strings.Contains(message, portText) {
		t.Fatalf("StartServer() = (%q, %v), want a bind error naming port %s", message, err, portText)
	}
	if err := httpServer.WaitForListening(context.Background()); err == nil {
		t.Fatal("WaitForListening() succeeded after StartServer failed to bind")
	}
}
