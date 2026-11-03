package server

import (
	"crypto/tls"
	"log"
	"os"
	"runtime"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/compress"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
)

// ListenerConfig describes deployment-level HTTP listener options.
type ListenerConfig struct {
	Network                    string
	TLSCertKeyFile             string
	TLSCertClientFile          string
	TLSCertFile                string
	UnixSocketPath             string
	PreforkRecoverInterval     time.Duration
	ShutdownTimeout            time.Duration
	PreforkShutdownGracePeriod time.Duration
	PreforkRecoverThreshold    int
	UnixSocketFileMode         os.FileMode
	TLSMinVersion              uint16
	EnablePrefork              bool
	EnablePrintRoutes          bool
}

type Config struct {
	// Core defines the core Fiber framework configuration options. When nil,
	// Gorest applies its default finite read, write, and idle timeouts. When
	// provided, this configuration is used as-is; zero ReadTimeout and
	// WriteTimeout values remain unlimited in Fiber and cause server.New to log
	// a warning. A zero IdleTimeout falls back to ReadTimeout in Fiber.
	Core *fiber.Config

	// Cors defines the configuration options for Cross-Origin Resource Sharing (CORS) middleware.
	// A nil config uses the GOREST default; nil does not disable the middleware.
	Cors *cors.Config

	// Recover defines the configuration options for panic recovery middleware.
	// Catches panics during the request lifecycle and prevents server crashes.
	// A nil config uses the GOREST default; nil does not disable the middleware.
	Recover *recover.Config

	// Requestid defines the configuration options for Request ID generation middleware.
	// Injects a unique tracing ID into the request and response headers for tracking.
	// A nil config uses the GOREST default; nil does not disable the middleware.
	Requestid *requestid.Config

	// Compress defines the configuration options for response compression middleware.
	// Compresses HTTP responses (e.g., Gzip, Brotli) to reduce payload size.
	// A nil config uses the GOREST default; nil does not disable the middleware.
	Compress *compress.Config

	// Helmet defines the configuration options for security headers middleware (Helmet).
	// Adds various HTTP headers to secure the application against common web vulnerabilities.
	// A nil config uses the GOREST default; nil does not disable the middleware.
	Helmet *helmet.Config

	// Limiter defines the configuration options for rate limiting middleware.
	// Restricts the number of requests a client can make within a specific time window.
	// A nil config uses the GOREST default; nil does not disable the middleware.
	Limiter *limiter.Config

	// Host is server address
	// Default: "127.0.0.1"
	Host string

	Listener ListenerConfig

	// Port is server port
	// Default: 3000
	Port int
}

var defaultListenerConfig = ListenerConfig{
	Network:                    fiber.NetworkTCP4,
	TLSMinVersion:              tls.VersionTLS12,
	UnixSocketFileMode:         0o770,
	ShutdownTimeout:            DefaultServerShutdownTimeout,
	PreforkShutdownGracePeriod: 5 * time.Second,
	PreforkRecoverThreshold:    max(1, runtime.GOMAXPROCS(0)/2),
}

var ConfigDefault = Config{
	Core: &fiber.Config{
		ReadTimeout:  DefaultServerReadTimeout,
		WriteTimeout: DefaultServerWriteTimeout,
		IdleTimeout:  DefaultServerIdleTimeout,
		BodyLimit:    DefaultServerBodyLimit,
	},
	// Default: using cors fiber config default
	Cors: &cors.ConfigDefault,

	// Default: using recover fiber config default
	Recover: &recover.ConfigDefault,

	// Default: using requestid fiber config default
	Requestid: &requestid.ConfigDefault,

	// Default: using compress fiber config default
	Compress: &compress.ConfigDefault,

	// Default: using helmet fiber config default
	Helmet: &helmet.ConfigDefault,

	// Default: using limiter fiber config default
	Limiter: &limiter.ConfigDefault,

	// Default: "127.0.0.1"
	Host: DefaultServerHost,

	// Default: 3000
	Port: DefaultServerPort,

	Listener: defaultListenerConfig,
}

// configDefault returns the standard ConfigDefault when no config is provided.
// Otherwise, it fills missing sections and validates bounds. Nil middleware
// configurations mean use the GOREST defaults; they do not disable middleware.
func configDefault(config ...Config) Config {
	if len(config) < 1 {
		return ConfigDefault
	}

	// Override default config
	cfg := config[0]

	if cfg.Core == nil {
		cfg.Core = &fiber.Config{
			ReadTimeout:  DefaultServerReadTimeout,
			WriteTimeout: DefaultServerWriteTimeout,
			IdleTimeout:  DefaultServerIdleTimeout,
			BodyLimit:    DefaultServerBodyLimit,
		}
	}

	if cfg.Cors == nil {
		cfg.Cors = ConfigDefault.Cors
	}

	if cfg.Recover == nil {
		cfg.Recover = ConfigDefault.Recover
	}

	if cfg.Requestid == nil {
		cfg.Requestid = ConfigDefault.Requestid
	}

	if cfg.Compress == nil {
		cfg.Compress = ConfigDefault.Compress
	}

	if cfg.Helmet == nil {
		cfg.Helmet = ConfigDefault.Helmet
	}

	if cfg.Limiter == nil {
		cfg.Limiter = ConfigDefault.Limiter
	}

	if cfg.Host == "" {
		cfg.Host = DefaultServerHost
	}

	// Default: 3000
	if cfg.Port == 0 {
		cfg.Port = DefaultServerPort
	}

	if cfg.Port < 0 {
		log.Fatalf("Config Server: Port must be greater than or equal to 0") //nolint:revive
	}

	cfg.Listener = listenerConfigDefault(cfg.Listener)

	return cfg
}

func listenerConfigDefault(config ListenerConfig) ListenerConfig {
	if config.Network == "" {
		config.Network = defaultListenerConfig.Network
	}

	if config.TLSMinVersion == 0 {
		config.TLSMinVersion = defaultListenerConfig.TLSMinVersion
	}

	if config.UnixSocketFileMode == 0 {
		config.UnixSocketFileMode = defaultListenerConfig.UnixSocketFileMode
	}

	if config.ShutdownTimeout == 0 {
		config.ShutdownTimeout = defaultListenerConfig.ShutdownTimeout
	}

	if config.PreforkShutdownGracePeriod == 0 {
		config.PreforkShutdownGracePeriod = defaultListenerConfig.PreforkShutdownGracePeriod
	}

	if config.PreforkRecoverThreshold == 0 {
		config.PreforkRecoverThreshold = defaultListenerConfig.PreforkRecoverThreshold
	}

	return config
}

func (config ListenerConfig) fiberListenConfig() fiber.ListenConfig {
	return fiber.ListenConfig{
		ListenerNetwork:            config.Network,
		CertFile:                   config.TLSCertFile,
		CertKeyFile:                config.TLSCertKeyFile,
		CertClientFile:             config.TLSCertClientFile,
		ShutdownTimeout:            config.ShutdownTimeout,
		PreforkRecoverInterval:     config.PreforkRecoverInterval,
		PreforkShutdownGracePeriod: config.PreforkShutdownGracePeriod,
		UnixSocketFileMode:         config.UnixSocketFileMode,
		TLSMinVersion:              config.TLSMinVersion,
		EnablePrefork:              config.EnablePrefork,
		PreforkRecoverThreshold:    config.PreforkRecoverThreshold,
		EnablePrintRoutes:          config.EnablePrintRoutes,
		DisableStartupMessage:      true,
	}
}
