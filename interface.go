package gorest

import (
	"context"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/log"
	"github.com/elsyahtech/gorest/redis"
	"github.com/elsyahtech/gorest/server"
)

type App struct {
	context    context.Context
	log        *log.Log
	server     *server.Server
	redis      *redis.Redis
	database   *database.Database
	services   *service
	config     Config
	configured Config
	modules    []string
}

type service struct {
	routers []RouterRegistrar
}

type RouterRegistrar func(*App)

type DBSession struct {
	database *database.Database
	config   *database.Config
}

type Map map[string]any

type Context = server.Context
