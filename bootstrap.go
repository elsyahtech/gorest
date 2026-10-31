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
		return errors.New("run database startup tasks: database service is not running")
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

// RunDatabaseStartupTasks synchronously runs the configured migrations and
// seeders without starting the HTTP server. It is intended for a single
// deployment job that runs before application replicas are started.
func (app *App) RunDatabaseStartupTasks() error {
	if app == nil {
		return errors.New("run database startup tasks: app is nil")
	}

	if app.config.database == nil || app.database == nil {
		return errors.New("run database startup tasks: database service is not running; register an enabled database config with app.Use(database.Run(...))")
	}

	return app.runMigration()
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

func (app *App) startServer() error {
	if app.log == nil {
		return errors.New("failed to start server: logger is not running")
	}

	app.Log(nil, true).Info("starting server")

	if app.config.server == nil || app.server == nil {
		return errors.New("failed to start server: server is not running")
	}

	publicRoutes := RegisterDefaultRoutes
	app.services.routers = append(app.services.routers, publicRoutes)

	_, message, err := app.routerRegistry(publicRoutes)
	if err != nil {
		return fmt.Errorf("register default routes: %s: %w", message, err)
	}

	message, err = server.StartServer(
		app.server,
		app.config.server,
		app.log,
		app.config.AppName,
		app.config.Version,
		app.config.Timezone,
		app.config.Environment,
		app.modules,
	)
	if err != nil {
		return fmt.Errorf("failed to start server: %s: %w", message, err)
	}

	return nil
}
