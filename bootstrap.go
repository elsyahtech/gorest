package gorest

import (
	"errors"
	"fmt"
	golog "log"
	"time"

	"github.com/elsyahtech/gorest/server"
)

func (app *App) loadTimezone() {
	if _, err := time.LoadLocation(app.config.Timezone); err != nil {
		golog.Fatalf("failed to load timezone %q: %v. Please check timezone configuration.", app.config.Timezone, err)
	}
}

func (app *App) runMigration() error {
	if app.config.database == nil || (!app.config.database.Migration && !app.config.database.Seeder) {
		return nil
	}

	if app.log == nil {
		return errors.New("run database startup tasks: logger is not running")
	}

	if app.database == nil {
		return errors.New("run database startup tasks: database is not initialized")
	}

	logger := app.Log(nil)

	ctx, cancel := app.NewContext(app.config.database.Timeout)
	defer cancel()

	var migration func() error
	if app.config.database.Migration {
		migration = func() error {
			message, err := app.database.Migration(ctx, app.config.database, logger)
			if err != nil {
				return fmt.Errorf("failed to run migration: %s: %w", message, err)
			}

			return nil
		}
	}

	var seeder func() error
	if app.config.database.Seeder {
		seeder = func() error {
			message, err := app.database.Seeder(ctx, app.config.database, logger)
			if err != nil {
				return fmt.Errorf("failed to run seeder: %s: %w", message, err)
			}

			return nil
		}
	}

	return runMigrationStartup(migration, seeder)
}

func runMigrationStartup(migration, seeder func() error) error {
	if migration != nil {
		if err := migration(); err != nil {
			return err
		}
	}

	if seeder != nil {
		return seeder()
	}

	return nil
}

func prepareStartup(migration, seeder func() error, startServer func()) (func(), error) {
	if err := runMigrationStartup(migration, seeder); err != nil {
		return nil, err
	}

	return func() {
		go startServer()
	}, nil
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
