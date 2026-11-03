package server

import (
	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/compress"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"github.com/gofiber/fiber/v3/middleware/limiter"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
)

func Run(config ...Config) Config {
	if len(config) < 1 {
		return ConfigDefault
	}

	cfg := config[0]

	return configDefault(cfg)
}

func New(config *Config) (*Server, string, error) {
	return NewWithMiddleware(config)
}

// NewWithMiddleware creates a server and installs the supplied handlers after
// compression but before the remaining default middleware and application routes.
func NewWithMiddleware(config *Config, middleware ...fiber.Handler) (*Server, string, error) {
	var cfg Config

	if config == nil {
		cfg = configDefault()
	} else {
		cfg = configDefault(*config)
	}

	if err := validateListenerConfig(cfg.Listener); err != nil {
		return nil, "invalid server listener configuration", err
	}

	warnOnUnboundedTimeouts(cfg.Core)

	server := fiber.New(*cfg.Core)

	server.Use(recover.New(*cfg.Recover))
	server.Use(requestid.New(*cfg.Requestid))
	server.Use(compress.New(*cfg.Compress))

	for _, handler := range middleware {
		server.Use(handler)
	}

	server.Use(helmet.New(*cfg.Helmet))
	server.Use(cors.New(*cfg.Cors))
	server.Use(limiter.New(*cfg.Limiter))

	return &Server{
		SRV:             server,
		Router:          server,
		listening:       make(chan struct{}),
		listenDone:      make(chan struct{}),
		shutdownTimeout: cfg.Listener.ShutdownTimeout,
	}, "", nil
}

type RouteHandler func(any)
