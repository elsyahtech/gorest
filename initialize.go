package gorest

import (
	"context"
	golog "log"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/filter"
	"github.com/elsyahtech/gorest/log"
	"github.com/elsyahtech/gorest/redis"
	"github.com/elsyahtech/gorest/server"
)

func (app *App) logInit() {
	if app == nil {
		golog.Fatalf("failed to run logger: application is nil")
	}

	app.registerService("Logger")

	logInitialized, message, err := log.New(app.config.log, app.config.AppName)
	if err != nil {
		golog.Fatalf("failed to run logger: %v. message: %v", err, message)
	}

	app.log = logInitialized
}

func (app *App) redisInit() {
	app.Log(nil, true).Info("starting Redis")

	if app.log == nil {
		golog.Fatalf("failed to run Redis: logger is not running")
	}

	if app.config.redis == nil {
		app.Log(nil, true).Info("Redis is disabled. To enable it, register its config with app.Use(redis.Run(...)).")

		return
	}

	if isProductionEnvironment(app.config.Environment) && !app.config.redis.TLS {
		approved, err := app.requestInsecureProductionApproval("Redis", app.config.redis.AllowInsecureProduction)
		if err != nil {
			golog.Fatal(err)
		}
		if approved {
			app.config.redis.AllowInsecureProduction = true
		}
	}

	ctx, cancel := app.NewContext(app.config.redis.Timeout)
	defer cancel()

	redisInitialized, message, err := retryStartup(ctx, func(attemptCtx context.Context) (*redis.Redis, string, error) {
		return redis.New(attemptCtx, app.config.redis)
	})
	if err != nil {
		app.Log(map[string]any{
			LogFieldKeyError:   err,
			LogFieldKeyMessage: message,
		}, true).Error("failed to run Redis")

		golog.Fatalf("failed to run Redis: %v. message: %v", err, message)
	}

	app.redis = redisInitialized

	app.Log(nil, true).Info("Redis is running")
}

func (app *App) databaseInit() {
	if app.log == nil {
		golog.Fatalf("failed to run database: logger is not running")
	}

	app.Log(nil, true).Info("starting database")

	if app.config.database == nil {
		app.Log(nil, true).Info("Database is disabled. To enable it, register its config with app.Use(database.Run(...)).")

		return
	}

	if isProductionEnvironment(app.config.Environment) && database.NormalizeDatabaseDriver(app.config.database.Driver) == database.SQLITE {
		if err := validateProductionSQLiteHost(app.config.database.Host); err != nil {
			golog.Fatal(err)
		}
	}

	if isProductionEnvironment(app.config.Environment) {
		if _, insecure := productionDatabaseTLSWarning(app.config.database); insecure {
			approved, err := app.requestInsecureProductionApproval(database.NormalizeDatabaseDriver(app.config.database.Driver)+" database", app.config.database.AllowInsecureProduction)
			if err != nil {
				golog.Fatal(err)
			}
			if approved {
				app.config.database.AllowInsecureProduction = true
			}
		}
	}

	ctx, cancel := app.NewContext(app.config.database.Timeout)
	defer cancel()

	databaseInitialized, message, err := retryStartup(ctx, func(attemptCtx context.Context) (*database.Database, string, error) {
		return database.New(attemptCtx, app.config.database, app.config.Timezone)
	})
	if err != nil {
		app.Log(map[string]any{
			LogFieldKeyError:   err,
			LogFieldKeyMessage: message,
		}, true).Error("failed to run database")

		golog.Fatalf("failed to run database: %v. message: %v", err, message)
	}

	app.database = databaseInitialized

	app.Log(nil, true).Info("database is running")
}

func (app *App) newServer() {
	if app.log == nil {
		golog.Fatalf("failed to run server: logger is not running")
	}

	app.registerService("Server")

	serverInitialized, message, err := server.New(app.config.server)
	if err != nil {
		app.Log(map[string]any{
			LogFieldKeyError:   err,
			LogFieldKeyMessage: message,
		}, true).Error("failed to run server")

		golog.Fatalf("failed to run server: %v. message: %v", err, message)
	}

	app.server = serverInitialized

	app.Log(nil, true).Info("server configuration is ready")
}

func (app *App) reconfigureServer() {
	if app.log == nil {
		golog.Fatalf("failed to run server: logger is not running")
	}

	app.registerService("Server")

	serverInitialized, message, err := server.New(app.config.server)
	if err != nil {
		app.Log(map[string]any{
			LogFieldKeyError:   err,
			LogFieldKeyMessage: message,
		}, true).Error("failed to run server")

		golog.Fatalf("failed to run server: %v. message: %v", err, message)
	}

	app.server = serverInitialized
}

// serverInit runs the server once, using the handler's config if provided,
// otherwise the default config. Safe to call multiple times.
func (app *App) serverInit() {
	if app.config.server == nil {
		def := server.ConfigDefault

		app.config.server = &def

		app.Log(nil, true).Info("configuring server")

		app.newServer()
	} else {
		app.reconfigureServer()
	}
}

func (app *App) filterInit(config *filter.Config) {
	if app == nil {
		golog.Fatalf("failed to run filter: application is nil")
	}

	if app.log == nil {
		golog.Fatalf("failed to run filter: logger is not running")
	}

	if config == nil {
		app.config.filter = nil
		app.filter = nil
		app.Log(nil, true).Info("JWT filter is disabled.")

		return
	}

	filterConfig := filter.Run(*config)
	if filterConfig.SignatureKey == nil || *filterConfig.SignatureKey == "" {
		app.Log(map[string]any{
			LogFieldKeyError:   "signature key in invalid or empty",
			LogFieldKeyMessage: "filter cannot run without a valid signature key or empty",
		}, true).Info("Filter is ignored")

		app.config.filter = nil
		app.filter = nil

		return
	}

	filterConfig.Issuer = app.config.AppName

	jwtInitialized, message, err := filter.New(&filterConfig)
	if err != nil {
		app.Log(map[string]any{
			LogFieldKeyError:   err,
			LogFieldKeyMessage: message,
		}, true).Error("failed to run filter")

		app.config.filter = nil
		app.filter = nil

		return
	}

	app.config.filter = &filterConfig
	app.filter = &filter.Filter{
		JWT:    jwtInitialized,
		Config: &filterConfig,
		Logger: app.log.Log(nil, true),
	}

	app.registerService("Filter")
}
