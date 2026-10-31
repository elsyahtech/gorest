package gorest

import (
	"errors"
	"fmt"
	golog "log"
	"slices"
	"strings"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/filter"
	"github.com/elsyahtech/gorest/log"
	"github.com/elsyahtech/gorest/redis"
	"github.com/elsyahtech/gorest/server"
	"github.com/gofiber/fiber/v3"
)

func (app *App) registerService(name string) {
	if slices.Contains(app.modules, name) {
		return // has been registered, ignore!
	}

	app.modules = append(app.modules, name)
}

func (app *App) Use(args ...any) *App {
	for idx := range args {
		switch arg := args[idx].(type) {
		// LOG
		case log.Config, *log.Config:
			app.useCaseLog(args, idx)

		// REDIS
		case redis.Config, *redis.Config:
			app.useCaseRedis(args, idx)

		// DATABASE
		case database.Config, *database.Config:
			app.useCaseDatabase(args, idx)

		// SERVER
		case server.Config, *server.Config:
			app.useCaseServer(args, idx)

		// FILTER
		case filter.Config, *filter.Config:
			app.useCaseFilter(args, idx)

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

func (app *App) useCaseLog(args []any, idx int) {
	if argType, logOk := args[idx].(log.Config); logOk {
		app.config.log = &argType
	} else if argPtr, logOk := args[idx].(*log.Config); logOk {
		app.config.log = argPtr
	}

	app.logInit()
}

func (app *App) useCaseRedis(args []any, idx int) {
	app.redis = &redis.Redis{}

	if argType, redisOk := args[idx].(redis.Config); redisOk {
		app.config.redis = &argType
	} else if argPtr, redisOk := args[idx].(*redis.Config); redisOk {
		app.config.redis = argPtr
	}

	if !app.config.redis.Disable {
		app.registerService("Redis")

		app.redisInit()
	} else {
		app.config.redis = nil
		app.redis = nil
	}
}

func (app *App) useCaseDatabase(args []any, idx int) {
	app.database = &database.Database{}

	if argType, dbOk := args[idx].(database.Config); dbOk {
		app.config.database = &argType
	} else if argPtr, dbOk := args[idx].(*database.Config); dbOk {
		app.config.database = argPtr
	}

	if !app.config.database.Disable {
		msg := fmt.Sprintf("Database (%s)", database.NormalizeDatabaseDriver(app.config.database.Driver))

		app.registerService(msg)

		app.databaseInit()
	} else {
		app.config.database = nil
		app.database = nil
	}
}

func (app *App) useCaseServer(args []any, idx int) {
	if argType, serverOk := args[idx].(server.Config); serverOk {
		app.config.server = &argType
	} else if argPtr, serverOk := args[idx].(*server.Config); serverOk {
		app.config.server = argPtr
	}

	app.serverInit()
}

func (app *App) useCaseFilter(args []any, idx int) {
	if argType, filterOk := args[idx].(filter.Config); filterOk {
		app.initializeConfiguredFilter(&argType)
	} else if argPtr, filterOk := args[idx].(*filter.Config); filterOk {
		if argPtr == nil {
			app.filterInit(nil)
			return
		}
		app.initializeConfiguredFilter(argPtr)
	}
}

func (app *App) initializeConfiguredFilter(config *filter.Config) {
	filterConfig := filter.Run(*config)
	if filterConfig.SignatureKey == nil || strings.TrimSpace(*filterConfig.SignatureKey) == "" {
		app.abortStartup(errors.New("JWT filter was configured without a signature key"))
	}

	app.filterInit(&filterConfig)
	if app.filter == nil {
		app.abortStartup(errors.New("failed to run JWT filter; check its signing method and token durations"))
	}
}

//nolint:unparam
func (app *App) routerRegistry(args ...RouterRegistrar) (*fiber.App, string, error) {
	if app.server == nil {
		const message = "Ensure server.Run(...) is registered with app.Use(...) before registering routes."

		return nil, message, errors.New("server is not running")
	}

	for _, registrar := range args {
		if registrar != nil {
			registrar(app)
		}
	}

	return app.server.SRV, "", nil
}

func (app *App) RegisterServices(args ...any) {
	var allRoutes []RouterRegistrar

	for idx := range args {
		switch arg := args[idx].(type) {
		case []RouterRegistrar:
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
