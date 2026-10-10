package gorest

import (
	"context"
	"os"
	"os/signal"
	"syscall"
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

	// Set timezone
	// to ensure consistent time parsing, logging, and timestamps.
	app.loadTimezone()

	// initialize the global logger
	app.logInit()

	// initialize the server
	app.serverInit()

	return app
}

func (app *App) Start() *App {
	app.Log(nil, true).Info("starting gorest")

	app.runMigration()

	quit := make(chan os.Signal, 1)

	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)

	go func() {
		app.startServer()
	}()

	sig := <-quit
	app.Log(map[string]any{"signal": sig.String()}, true).Info("Gorest engine received shutdown signal...")

	if _, _, err := app.Close(); err != nil {
		app.Log(map[string]any{LogFieldKeyError: err}, true).Error("Failed to shutdown Gorest gracefully.")
	} else {
		app.Log(nil, true).Info("Gorest engine shutdown successfully. Clean exit.")
	}

	return app
}
