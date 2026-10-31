package server

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"

	"github.com/elsyahtech/gorest/log"

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
	var cfg Config

	if config == nil {
		cfg = configDefault()
	} else {
		cfg = configDefault(*config)
	}

	warnOnUnboundedTimeouts(cfg.Core)

	server := fiber.New(*cfg.Core)

	server.Use(recover.New(*cfg.Recover))
	server.Use(requestid.New(*cfg.Requestid))
	server.Use(compress.New(*cfg.Compress))
	server.Use(helmet.New(*cfg.Helmet))
	server.Use(cors.New(*cfg.Cors))
	server.Use(limiter.New(*cfg.Limiter))

	return &Server{
		SRV:        server,
		Router:     server,
		listening:  make(chan struct{}),
		listenDone: make(chan struct{}),
	}, "", nil
}

type RouteHandler func(any)

// ExpectListen marks the server as scheduled to start so Shutdown can wait for
// the listener to become ready or finish starting.
func (server *Server) ExpectListen() {
	if server == nil {
		return
	}

	server.lifecycleMu.Lock()
	server.listenScheduled = true
	server.lifecycleMu.Unlock()
}

// Listen starts Fiber and tracks its readiness and completion for shutdown.
func (server *Server) Listen(address string, config fiber.ListenConfig) error {
	if server == nil || server.SRV == nil {
		return errors.New("listen: server is not running")
	}

	server.ExpectListen()
	defer server.markListenDone()

	server.SRV.Hooks().OnListen(func(fiber.ListenData) error {
		server.listeningOnce.Do(func() { close(server.listening) })
		return nil
	})

	return server.SRV.Listen(address, config) //nolint:wrapcheck
}

func (server *Server) markListenDone() {
	server.listenDoneOnce.Do(func() { close(server.listenDone) })
}

// WaitForListening waits until the server listener is ready or exits.
func (server *Server) WaitForListening(ctx context.Context) error {
	if server == nil || server.SRV == nil {
		return errors.New("wait for server listener: server is not running")
	}

	if ctx == nil {
		return errors.New("wait for server listener: context is nil")
	}

	server.lifecycleMu.Lock()
	scheduled := server.listenScheduled
	listening := server.listening
	listenDone := server.listenDone
	server.lifecycleMu.Unlock()

	if !scheduled {
		return errors.New("wait for server listener: listener has not been scheduled")
	}

	select {
	case <-listening:
		return nil
	case <-listenDone:
		return errors.New("server listener stopped before becoming ready")
	case <-ctx.Done():
		return fmt.Errorf("wait for server listener: %w", ctx.Err())
	}
}

// Shutdown gracefully stops Fiber and waits for its listener to return.
func (server *Server) Shutdown(ctx context.Context) error {
	if server == nil || server.SRV == nil {
		return nil
	}

	if ctx == nil {
		return errors.New("shutdown: context is nil")
	}

	server.lifecycleMu.Lock()
	scheduled := server.listenScheduled
	listening := server.listening
	listenDone := server.listenDone

	server.lifecycleMu.Unlock()

	if !scheduled {
		return nil
	}

	select {
	case <-listening:
	case <-listenDone:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for server listener before shutdown: %w", ctx.Err())
	}

	shutdownErr := server.SRV.ShutdownWithContext(ctx)
	if shutdownErr != nil {
		select {
		case <-listenDone:
			return nil
		default:
		}

		return fmt.Errorf("shutdown Fiber server: %w", shutdownErr)
	}

	select {
	case <-listenDone:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("wait for Fiber listener to stop: %w", ctx.Err())
	}
}

//nolint:revive
func StartServer(
	server *Server,
	config *Config,
	log *log.Log,
	appName, version, timezone, environment string,
	modules []string,
	routes ...any,
) (string, error) {
	if server == nil || config == nil {
		return "Register the server config with app.Use(server.Run(...)).",
			errors.New("startServer: server is not running")
	}

	if log == nil {
		return "Ensure the logger is running before starting the server.",
			errors.New("startServer: logger is not running")
	}

	defer server.markListenDone()

	port := config.Port
	portStr := strconv.Itoa(port)
	address := serverAddress(config.Host, port)

	printStartupBanner(server, config, log, appName, version, timezone, environment, modules, routes...)

	if err := server.Listen(address, fiber.ListenConfig{DisableStartupMessage: true}); err != nil {
		message := fmt.Sprintf("ensure the server configuration is set up correctly, "+
			"the host and port are configured, and the port '%s' is not already in use by another service", portStr)

		return message, fmt.Errorf("%w", err)
	}

	return "", nil
}

func serverAddress(host string, port int) string {
	hst := host

	if hst == "" {
		hst = DefaultServerHost
	}

	return net.JoinHostPort(hst, strconv.Itoa(port))
}
