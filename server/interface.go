package server

import (
	"sync"

	"github.com/gofiber/fiber/v3"
)

type Server struct {
	SRV    *fiber.App
	Router fiber.Router

	lifecycleMu     sync.Mutex
	listenScheduled bool
	listening       chan struct{}
	listenDone      chan struct{}
	listeningOnce   sync.Once
	listenDoneOnce  sync.Once
}

type Context struct {
	fiber.Ctx
}

type HandlerFunc func(*Context) error
