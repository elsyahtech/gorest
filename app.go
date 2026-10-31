package gorest

import (
	"context"
	"os"

	golog "log"
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
	app.registerOperationalRoutes()

	// initialize the filter
	app.filterInit(app.config.filter)

	return app
}

func (app *App) Start() *App {
	if app == nil {
		golog.Fatal("application startup aborted: app is nil")
		return nil
	}

	app.Log(nil, true).Info("starting gorest")

	app.logProductionWarnings()
	app.startLifecycle()

	return app
}

func (app *App) abortStartup(err error) {
	app.Log(map[string]any{LogFieldKeyError: err}, true).Error("application startup aborted")

	app.closeAndReport()

	golog.Fatalf("application startup aborted: %v", err)
}

func (app *App) shutdownAfterSignal(sig os.Signal) {
	app.Log(map[string]any{"signal": sig.String()}, true).Info("Application received shutdown signal...")

	app.closeAndReport()
}

func (app *App) closeAndReport() {
	if message, _, err := app.Close(); err != nil {
		golog.Printf("application shutdown failed: %s: %v", message, err)
	} else {
		golog.Print(message)
	}
}
