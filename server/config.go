package server

import (
	"log"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/compress"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
)

type Config struct {
	// Core defines the core Fiber framework configuration options.
	// Default: nil (uses framework default settings)
	Core *fiber.Config

	// Cors defines the configuration options for Cross-Origin Resource Sharing (CORS) middleware.
	// Controls how resources can be requested from another domain.
	// Default: nil (uses framework default settings)
	Cors *cors.Config

	// Recover defines the configuration options for panic recovery middleware.
	// Catches panics during the request lifecycle and prevents server crashes.
	// Default: nil (uses framework default settings)
	Recover *recover.Config

	// Requestid defines the configuration options for Request ID generation middleware.
	// Injects a unique tracing ID into the request and response headers for tracking.
	// Default: nil (uses framework default settings)
	Requestid *requestid.Config

	// Compress defines the configuration options for response compression middleware.
	// Compresses HTTP responses (e.g., Gzip, Brotli) to reduce payload size.
	// Default: nil (uses framework default settings)
	Compress *compress.Config

	// Helmet defines the configuration options for security headers middleware (Helmet).
	// Adds various HTTP headers to secure the application against common web vulnerabilities.
	// Default: nil (uses framework default settings)
	Helmet *helmet.Config

	// Limiter defines the configuration options for rate limiting middleware.
	// Restricts the number of requests a client can make within a specific time window.
	// Default: nil (uses framework default settings)
	Limiter *limiter.Config

	// Host is server address
	// Default: "127.0.0.1"
	Host string

	// Port is server port
	// Default: 3000
	Port int
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
}

// configDefault is a helper function that processes the optional config argument.
// If no configuration is provided, it returns the standard ConfigDefault.
// Otherwise, it merges any missing/zero values with defaults and validates bounds.
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

	return cfg
}
