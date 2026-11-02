package gorest

import (
	"fmt"

	"github.com/elsyahtech/gorest/server"
	"github.com/gofiber/fiber/v3"
)

func (app *App) newServerWithHTTPMetrics(config *server.Config) (*server.Server, string, error) {
	var (
		serverInitialized *server.Server
		message           string
		err               error
	)

	if app.httpMetrics != nil && !app.config.DisableMonitoringEndpoints {
		serverInitialized, message, err = server.NewWithMiddleware(config, app.httpMetrics.Middleware)
	} else {
		serverInitialized, message, err = server.New(config)
	}

	if err != nil {
		err = fmt.Errorf("initialize HTTP server: %w", err)
	}

	return serverInitialized, message, err
}

func (app *App) serverConfigWithHTTPMetrics() server.Config {
	if app == nil {
		return server.Run()
	}

	var config server.Config
	if app.config.server == nil {
		config = server.Run()
	} else {
		config = server.Run(*app.config.server)
	}
	if app.httpMetrics == nil || app.config.DisableMonitoringEndpoints {
		return config
	}

	core := *config.Core

	errorHandler := core.ErrorHandler
	if errorHandler == nil {
		errorHandler = fiber.DefaultErrorHandler
	}
	core.ErrorHandler = app.httpMetrics.WrapErrorHandler(errorHandler)
	config.Core = &core
	return config
}
