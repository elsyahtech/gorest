package server

import (
	"bytes"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
)

func TestNewAppliesDefaultsForNilAndEmptyConfig(t *testing.T) {
	for _, test := range []struct {
		name   string
		config *Config
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
		port int
		want string
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
