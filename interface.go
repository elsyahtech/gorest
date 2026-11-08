package gorest

import (
	"context"
	"database/sql"
	"sync"
	"sync/atomic"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/filter"
	"github.com/elsyahtech/gorest/hardening"
	"github.com/elsyahtech/gorest/log"
	"github.com/elsyahtech/gorest/monitoring"
	"github.com/elsyahtech/gorest/redis"
	"github.com/elsyahtech/gorest/server"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type App struct {
	context       context.Context
	updateCheckWG sync.WaitGroup
	log           *log.Log
	server        *server.Server
	redis         *redis.Redis
	database      *database.Database
	services      *service
	filter        *filter.Filter
	monitoring    *monitoring.Monitoring
	hardening     *hardening.Hardening
	config        Config
	configured    Config
	modules       []string
}

type service struct {
	routers []RouterRegistrar
}

type RouterRegistrar func(*App)

type DBSession struct {
	database     *database.Database
	config       *database.Config
	Tx           *sql.Tx
	mongoSession *mongo.Session
	txContext    context.Context
	txCancel     context.CancelFunc
	txDone       atomic.Bool
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
