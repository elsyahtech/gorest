package server

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"

	"github.com/elsyahtech/gorest/log"
	"github.com/gofiber/fiber/v3"
)

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

	cfg := configDefault(*config)
	if err := validateListenerConfig(cfg.Listener); err != nil {
		return "Correct the server listener configuration before starting the server.", err
	}

	server.setShutdownTimeout(cfg.Listener.ShutdownTimeout)
	defer server.markListenDone()

	address := listenerAddress(cfg)
	isPreforkWorker := cfg.Listener.EnablePrefork && isManagedPreforkWorker()

	var supervisor *preforkSupervisor

	if cfg.Listener.EnablePrefork && !isPreforkWorker {
		var err error

		supervisor, err = newPreforkSupervisor(server, cfg.Listener)
		if err != nil {
			return "unable to initialize the GOREST prefork supervisor", err
		}

		server.setPreforkSupervisor(supervisor)
	}

	if !isPreforkWorker {
		printStartupBanner(server, &cfg, log, appName, version, timezone, environment, modules, routes...)
	}

	if cfg.Listener.EnablePrefork && !isPreforkWorker {
		if err := supervisor.run(); err != nil {
			return "GOREST prefork supervisor stopped unexpectedly", err
		}

		return "", nil
	}

	listenConfig := cfg.Listener.fiberListenConfig()
	if cfg.Listener.EnablePrefork && isPreforkWorker {
		listenConfig.ListenerAddrFunc = func(net.Addr) {
			if err := announcePreforkWorker(); err != nil {
				fmt.Fprintf(os.Stderr, "gorest prefork worker could not register with its supervisor: %v\n", err)
				os.Exit(1)
			}

			server.markListening()
		}
	}

	if err := server.Listen(address, listenConfig); err != nil {
		if cfg.Listener.Network == fiber.NetworkUnix {
			return "ensure the Unix socket path is valid and available", fmt.Errorf("start server listener at %q: %w", address, err)
		}

		message := fmt.Sprintf("ensure the server configuration is set up correctly, "+
			"the host and port are configured, and the port '%s' is not already in use by another service", strconv.Itoa(cfg.Port))

		return message, fmt.Errorf("start server listener at %q: %w", address, err)
	}

	return "", nil
}

func listenerAddress(config Config) string {
	if config.Listener.Network == fiber.NetworkUnix {
		return config.Listener.UnixSocketPath
	}

	return serverAddress(config.Host, config.Port)
}

func serverAddress(host string, port int) string {
	addressHost := host
	if addressHost == "" {
		addressHost = DefaultServerHost
	}

	return net.JoinHostPort(addressHost, strconv.Itoa(port))
}
