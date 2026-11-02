package gorest

import (
	"fmt"

	"github.com/elsyahtech/gorest/server"
	"github.com/gofiber/fiber/v3"
)

func (app *App) newServerWithMonitoring(config *server.Config) (*server.Server, string, error) {
	var (
		serverInitialized *server.Server
		message           string
		err               error
	)

	if app.monitoring != nil && app.monitoring.EndpointsEnabled() {
		serverInitialized, message, err = server.NewWithMiddleware(config, app.monitoring.Middleware)
	} else {
		serverInitialized, message, err = server.New(config)
	}

	if err != nil {
		err = fmt.Errorf("initialize HTTP server: %w", err)
	}

	return serverInitialized, message, err
}

func (app *App) serverConfigWithMonitoring() server.Config {
	if app == nil {
		return server.Run()
	}

	var config server.Config
	if app.config.server == nil {
		config = server.Run()
	} else {
		config = server.Run(*app.config.server)
	}
	if app.monitoring == nil || !app.monitoring.EndpointsEnabled() {
		return config
	}

	core := *config.Core

	errorHandler := core.ErrorHandler
	if errorHandler == nil {
		errorHandler = fiber.DefaultErrorHandler
	}
	core.ErrorHandler = app.monitoring.WrapErrorHandler(errorHandler)
	config.Core = &core
	return config
}
