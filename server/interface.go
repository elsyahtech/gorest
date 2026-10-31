package server

import (
	"sync"

	"github.com/gofiber/fiber/v3"
)

type Server struct {
	Router          fiber.Router
	SRV             *fiber.App
	listening       chan struct{}
	listenDone      chan struct{}
	listeningOnce   sync.Once
	listenDoneOnce  sync.Once
	lifecycleMu     sync.Mutex
	listenScheduled bool
}

type Context struct {
	fiber.Ctx
}

type Cookie = fiber.Cookie

type Handler = func(*Context) error

type HandlerFunc func(*Context) error
