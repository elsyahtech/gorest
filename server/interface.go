package server

import "github.com/gofiber/fiber/v3"

type Server struct {
	SRV    *fiber.App
	Router fiber.Router
}

type Context struct {
	fiber.Ctx
}

type RouterRegistrar func(*Server)

type HandlerFunc func(*Context) error
