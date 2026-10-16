package server

import (
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

	server := fiber.New(*cfg.Core)

	server.Use(recover.New(*cfg.Recover))
	server.Use(requestid.New(*cfg.Requestid))
	server.Use(compress.New(*cfg.Compress))
	server.Use(helmet.New(*cfg.Helmet))
	server.Use(cors.New(*cfg.Cors))
	server.Use(limiter.New(*cfg.Limiter))

	return &Server{
		SRV:    server,
		Router: server,
	}, "", nil
}

type RouteHandler func(any)

//nolint:revive
func StartServer(
	server *Server,
	config *Config,
	log *log.Log,
	appName, version, timezone, environment string,
	modules []string,
	routes ...any,
) (string, error) {
	port := config.Port
	portStr := strconv.Itoa(port)
	address := serverAddress(config.Host, port)

	if err := printStartupBanner(server, config, log, appName, version, timezone, environment, modules, routes...); err != nil {
		return "please contact us on github", fmt.Errorf("gorest error: %w", err)
	}

	if err := server.SRV.Listen(address, fiber.ListenConfig{DisableStartupMessage: true}); err != nil {
		message := fmt.Sprintf("ensure the server configuration is set up correctly, "+
			"the host and port are configured, and the port '%s' is not already in use by another service", portStr)

		return message, fmt.Errorf("%w", err)
	}

	return "", nil
}

func serverAddress(host string, port int) string {
	if host == "" {
		host = DefaultServerHost
	}

	return net.JoinHostPort(host, strconv.Itoa(port))
}
