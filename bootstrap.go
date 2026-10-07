package gorest

import (
	golog "log"
	"time"

	"github.com/elsyahtech/gorest/server"
)

func (app *App) loadTimezone() {
	location, err := time.LoadLocation(app.config.Timezone)
	if err != nil {
		golog.Fatalf("failed to load timezone %q: %v. Please check timezone configuration.", app.config.Timezone, err)
	}

	time.Local = location
}

func (app *App) runMigration() {
	if app.log == nil {
		golog.Fatalf("run migration is failed: logger is not running. message: " +
			"Ensure the logger is running before start the migration. Run log.Run(log.Config{...}) in your app")
	}

	logger := app.Log(nil)

	if app.database == nil && app.config.database == nil {
		return
	}

	// Check if migration configured in status enabled
	if !app.config.database.Migration {
		return
	}

	// Run migration
	ctx, cancel := app.NewContext(app.config.database.Timeout)
	defer cancel()

	message, err := app.database.Migration(ctx, app.config.database, logger)
	if err != nil {
		app.Log(map[string]any{
			LogFieldKeyError:   err,
			LogFieldKeyMessage: message,
		}, true).Error("failed to run migration")
	}

	// Check if seeder configured in status enabled
	if !app.config.database.Seeder {
		return
	}

	// Run seeder
	message, err = app.database.Seeder(ctx, app.config.database, logger)
	if err != nil {
		app.Log(map[string]any{
			LogFieldKeyError:   err,
			LogFieldKeyMessage: message,
		}, true).Error("failed to run seeder")
	}
}

func (app *App) startServer() {
	if app.log == nil {
		golog.Fatalf("failed to start server: logger is not running. message: " +
			"Ensure the logger is running before start the server. Run log.Run(log.Config{...}) in your app")
	}

	app.Log(nil, true).Info("starting server")

	if app.config.server == nil || app.server == nil {
		return
	}

	if !app.config.Debug {
		publicRoutes := RegisterDefaultRoutes
		modulesRunning := app.modules

		app.services.routers = append(app.services.routers, publicRoutes)

		_, message, err := app.routerRegistry(publicRoutes)
		if err != nil {
			app.Log(map[string]any{
				LogFieldKeyError:   err,
				LogFieldKeyMessage: message,
			}, true).Error("failed to start server")

			golog.Fatalf("%v", err)
		}

		message, err = server.StartServer(
			app.server,
			app.config.server,
			app.log,
			app.config.AppName,
			app.config.Version,
			app.config.Timezone,
			ConfigDefault.Environment,
			modulesRunning,
			app.services.routers,
		)
		if err != nil {
			app.Log(map[string]any{
				LogFieldKeyError:   err,
				LogFieldKeyMessage: message,
			}, true).Error("starting server is failed")

			golog.Fatalf("starting server is failed: %v. message: %v", err, message)
		}
	}

	app.Log(nil, true).Info("gorest is running")
}
