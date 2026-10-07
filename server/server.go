package server

import (
	"fmt"
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
	if config == nil {
		//nolint:revive
		config = &ConfigDefault
	}

	server := fiber.New()

	if config.Core == nil {
		server = fiber.New(fiber.Config{})
	} else {
		server = fiber.New(*config.Core)
	}

	if config.Recover != nil {
		server.Use(recover.New(*config.Recover))
	}

	if config.Requestid != nil {
		server.Use(requestid.New(*config.Requestid))
	}

	if config.Compress != nil {
		server.Use(compress.New(*config.Compress))
	}

	if config.Helmet != nil {
		server.Use(helmet.New(*config.Helmet))
	}

	if config.Cors != nil {
		server.Use(cors.New(*config.Cors))
	}

	if config.Limiter != nil {
		server.Use(limiter.New(*config.Limiter))
	}

	return &Server{
		SRV:    server,
		Router: server,
	}, "", nil
}

//nolint:revive
func StartServer(
	server *Server,
	config *Config,
	log *log.Log,
	appName, version, timezone, environment string,
	modules []string,
	routes ...RouterRegistrar,
) (string, error) {
	port := config.Port
	portStr := strconv.Itoa(port)

	printStartupBanner(server, config, log, appName, version, timezone, environment, modules, routes...)

	if err := server.SRV.Listen(":"+portStr, fiber.ListenConfig{DisableStartupMessage: true}); err != nil {
		const message = "ensure the server configuration is set up correctly and " +
			"the host and port is configured"

		return message, fmt.Errorf("%w", err)
	}

	return "", nil
}
