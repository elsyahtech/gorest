package server

import (
	"net/http"
	"net/http/httptest"
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
