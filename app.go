package gorest

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	golog "log"

	"github.com/elsyahtech/gorest/database"
)

func New(config ...Config) *App {
	app := &App{
		config:   configDefault(config...),
		context:  context.Background(),
		services: &service{},
		modules:  make([]string, 0),
	}

	// Preserve original configuration before defaults are applied.
	app.configured = app.config

	// Validate the configured timezone without mutating process-global time.Local.
	app.loadTimezone()

	// initialize the global logger
	app.logInit()

	// initialize the server
	app.serverInit()

	return app
}

func (app *App) Start() *App {
	app.Log(nil, true).Info("starting gorest")

	if isProductionEnvironment(app.config.Environment) {
		if warning, shouldWarn := productionDatabaseTLSWarning(app.config.database); shouldWarn {
			app.Log(Map{
				"environment": app.config.Environment,
				"driver":      database.NormalizeDatabaseDriver(app.config.database.Driver),
				"tls_mode":    app.config.database.TLS,
				"message":     warning,
			}, true).Warn("database transport encryption (TLS/SSL) is disabled or not guaranteed in a production environment")
		}
	}

	startServer, err := prepareStartup(app.runMigration, nil, app.startServer)
	if err != nil {
		app.Log(map[string]any{LogFieldKeyError: err}, true).Error("database startup tasks failed")

		golog.Fatalf("application startup aborted: %v", err)
	}

	quit := make(chan os.Signal, 1)

	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(quit)

	if !app.config.Debug && app.server != nil && app.config.server != nil {
		app.server.ExpectListen()
	}

	startServer()

	sig := <-quit

	app.Log(map[string]any{"signal": sig.String()}, true).Info("Application received shutdown signal...")

	if message, _, err := app.Close(); err != nil {
		golog.Printf("application shutdown failed: %s: %v", message, err)
	} else {
		golog.Print(message)
	}

	return app
}
