package gorest

import (
	golog "log"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/log"
	"github.com/elsyahtech/gorest/redis"
	"github.com/elsyahtech/gorest/server"
)

func (app *App) logInit() {
	app.registerService("Logger")

	logInitialized, message, err := log.New(app.config.log, app.config.AppName)
	if err != nil {
		golog.Fatalf("failed to run logger: %v. message: %v", err, message)
	}

	app.log = logInitialized
}

func (app *App) redisInit() {
	app.Log(nil, true).Info("initializing redis")

	if app.log == nil {
		golog.Fatalf("failed initializing redis: logger is not running. " +
			"message: Ensure the logger is running before initializing the redis, or run redis.Run(redis.Config{...}) in your app")
	}

	if app.config.redis == nil {
		app.Log(nil, true).Info("redis is not run by the client. Redis is ignored. " +
			"If you want to enable Redis, you can run redis.Run(redis.Config{...}) in your app")

		return
	}

	ctx, cancel := app.NewContext(app.config.redis.Timeout)
	defer cancel()

	redisInitialized, message, err := redis.New(ctx, app.config.redis)
	if err != nil {
		app.Log(map[string]any{
			LogFieldKeyError:   err,
			LogFieldKeyMessage: message,
		}, true).Error("failed initializing redis")

		golog.Fatalf("failed initializing redis: %v. message: %v", err, message)
	}

	app.redis = redisInitialized

	app.Log(nil, true).Info("redis initialized successfully")
}

func (app *App) databaseInit() {
	if app.log == nil {
		golog.Fatalf("failed initializing database: logger is not running. " +
			"message: Ensure the logger is running before initializing the database, or run redis.Run(redis.Config{...}) in your app")
	}

	app.Log(nil, true).Info("initializing database")

	if app.config.database == nil {
		app.Log(map[string]any{
			LogFieldKeyMessage: "If you want to enable Database, you can run database.Run(database.Config{...}) in your app",
		}, true).Info("database is not run by the client. Database is ignored")

		return
	}

	ctx, cancel := app.NewContext(app.config.database.Timeout)
	defer cancel()

	databaseInitialized, message, err := database.New(ctx, app.config.database, app.config.Timezone)
	if err != nil {
		app.Log(map[string]any{
			LogFieldKeyError:   err,
			LogFieldKeyMessage: message,
		}, true).Error("failed initializing database")

		golog.Fatalf("failed initializing database: %v. message: %v", err, message)
	}

	app.database = databaseInitialized

	app.Log(nil, true).Info("database initialized successfully")
}

func (app *App) serverInit() {
	app.Log(nil, true).Info("initializing server")

	if app.log == nil {
		golog.Fatalf("failed initializing server: " +
			"logger is not running. message: Ensure the logger is running before initializing the server, " +
			"or run redis.Run(redis.Config{...}) in your app")
	}

	app.registerService("Server")

	serverInitialized, message, err := server.New(app.config.server)
	if err != nil {
		app.Log(map[string]any{
			LogFieldKeyError:   err,
			LogFieldKeyMessage: message,
		}, true).Error("failed to initializing server")

		golog.Fatalf("failed initializing server: %v. message: %v", err, message)
	}

	app.server = serverInitialized

	app.Log(nil, true).Info("server initialized successfully")
}
