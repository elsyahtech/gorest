package gorest

import (
	"errors"
	"fmt"
	golog "log"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/log"
	"github.com/elsyahtech/gorest/redis"
	"github.com/elsyahtech/gorest/server"
	"github.com/gofiber/fiber/v3"
)

func (app *App) registerService(name string) {
	for _, m := range app.modules {
		if m == name {
			return // has been registered, ignore!
		}
	}

	app.modules = append(app.modules, name)
}

func (app *App) Use(args ...any) *App {
	for idx := range args {
		switch arg := args[idx].(type) {
		// LOG
		case log.Config, *log.Config:
			if argType, logOk := args[idx].(log.Config); logOk {
				app.config.log = &argType
			} else if argPtr, logOk := args[idx].(*log.Config); logOk {
				app.config.log = argPtr
			}

			app.logInit()

		// REDIS
		case redis.Config, *redis.Config:
			app.redis = &redis.Redis{}

			if argType, redisOk := args[idx].(redis.Config); redisOk {
				app.config.redis = &argType
			} else if argPtr, redisOk := args[idx].(*redis.Config); redisOk {
				app.config.redis = argPtr
			}

			app.registerService("Redis")

			app.redisInit()

		// DATABASE
		case database.Config, *database.Config:
			app.database = &database.Database{}

			if argType, dbOk := args[idx].(database.Config); dbOk {
				app.config.database = &argType
			} else if argPtr, dbOk := args[idx].(*database.Config); dbOk {
				app.config.database = argPtr
			}

			msg := fmt.Sprintf("Database (%s)", database.NormalizeDatabaseDriver(app.config.database.Driver))

			app.registerService(msg)

			app.databaseInit()

		// SERVER
		case server.Config, *server.Config:
			if argType, serverOk := args[idx].(server.Config); serverOk {
				app.config.server = &argType
			} else if argPtr, serverOk := args[idx].(*server.Config); serverOk {
				app.config.server = argPtr
			}

			app.serverInit()

		default:
			golog.Fatalf(
				"gorest error: unsupported application configuration type %T (%v); "+
					"supported types: "+
					"log.Config/log.ConfigDefault, "+
					"database.Config/database.ConfigDefault, "+
					"redis.Config/redis.ConfigDefault, "+
					"server.Config/server.ConfigDefault; "+
					"example: app.Use(database.Config{...})"+
					"example: app.Use(database.ConfigDefault)",
				arg, arg,
			)
		}
	}

	return app
}

//nolint:unparam
func (app *App) routerRegistry(args ...server.RouterRegistrar) (*fiber.App, string, error) {
	if app.server == nil {
		const message = "ensure that you have run the Server in your app"

		return nil, message, errors.New("server has not been run in your app, please run the server")
	}

	for _, registrar := range args {
		if registrar != nil {
			registrar(app.server)
		}
	}

	return app.server.SRV, "", nil
}

func (app *App) RegisterServices(args ...any) {
	var allRoutes []server.RouterRegistrar

	for idx := range args {
		switch arg := args[idx].(type) {
		case []server.RouterRegistrar:
			allRoutes = append(allRoutes, arg...)

			_, message, err := app.routerRegistry(arg...)
			if err != nil {
				app.Log(map[string]any{
					"error": err,
				}, true).Error(message)
			}
		default:
			golog.Fatalf(
				"Gorest Notice: We encountered an unsupported router configuration type (%T: %v).\n"+
					"To keep things running smoothly, Gorest expects a slice of RouterRegistrar.\n"+
					"Example usage:\n"+
					"  routerRegister := []server.RouterRegistrar{\n"+
					"      userRoutes,\n"+
					"  }\n"+
					"  app.Start(routerRegister)",
				arg, arg,
			)
		}
	}

	app.services.routers = allRoutes
}
