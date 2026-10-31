package gorest

import (
	"context"
	"database/sql"
	"sync/atomic"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/filter"
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
	filter     *filter.Filter
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
	Tx       *sql.Tx
	txCancel context.CancelFunc
	txDone   atomic.Bool
}

type RedisSession struct {
	redis  *redis.Redis
	config *redis.Config
}

type Filter struct {
	filter *filter.Filter
}

type Map map[string]any

type Context = server.Context

type Cookie = server.Cookie
