package server

import "github.com/gofiber/fiber/v3"

// Get registers a route for GET methods that requests a representation
// of the specified resource. Requests using GET should only retrieve data.
func (app *Server) Get(path string, handler HandlerFunc) {
	app.Router.Get(path, func(ctx fiber.Ctx) error {
		gorestCtx := &Context{Ctx: ctx}

		return handler(gorestCtx)
	})
}

// Head registers a route for HEAD methods that asks for a response identical
// to that of a GET request, but without the response body.
func (app *Server) Head(path string, handler HandlerFunc) {
	app.Router.Head(path, func(ctx fiber.Ctx) error {
		gorestCtx := &Context{Ctx: ctx}

		return handler(gorestCtx)
	})
}

// Post registers a route for POST methods that is used to submit an entity to the
// specified resource, often causing a change in state or side effects on the server.
func (app *Server) Post(path string, handler HandlerFunc) {
	app.Router.Post(path, func(ctx fiber.Ctx) error {
		gorestCtx := &Context{Ctx: ctx}

		return handler(gorestCtx)
	})
}

// Put registers a route for PUT methods that replaces all current representations
// of the target resource with the request payload.
func (app *Server) Put(path string, handler HandlerFunc) {
	app.Router.Put(path, func(ctx fiber.Ctx) error {
		gorestCtx := &Context{Ctx: ctx}

		return handler(gorestCtx)
	})
}

// Delete registers a route for DELETE methods that deletes the specified resource.
func (app *Server) Delete(path string, handler HandlerFunc) {
	app.Router.Delete(path, func(ctx fiber.Ctx) error {
		gorestCtx := &Context{Ctx: ctx}

		return handler(gorestCtx)
	})
}

// Connect registers a route for CONNECT methods that establishes a tunnel to the
// server identified by the target resource.
func (app *Server) Connect(path string, handler HandlerFunc) {
	app.Router.Connect(path, func(ctx fiber.Ctx) error {
		gorestCtx := &Context{Ctx: ctx}

		return handler(gorestCtx)
	})
}

// Options registers a route for OPTIONS methods that is used to describe the
// communication options for the target resource.
func (app *Server) Options(path string, handler HandlerFunc) {
	app.Router.Options(path, func(ctx fiber.Ctx) error {
		gorestCtx := &Context{Ctx: ctx}

		return handler(gorestCtx)
	})
}

// Trace registers a route for TRACE methods that performs a message loop-back
// test along the path to the target resource.
func (app *Server) Trace(path string, handler HandlerFunc) {
	app.Router.Trace(path, func(ctx fiber.Ctx) error {
		gorestCtx := &Context{Ctx: ctx}

		return handler(gorestCtx)
	})
}

// Patch registers a route for PATCH methods that is used to apply partial
// modifications to a resource.
func (app *Server) Patch(path string, handler HandlerFunc) {
	app.Router.Patch(path, func(ctx fiber.Ctx) error {
		gorestCtx := &Context{Ctx: ctx}

		return handler(gorestCtx)
	})
}

// Query registers a route for QUERY methods that performs a safe, idempotent
// query with a request body.
func (app *Server) Query(path string, handler HandlerFunc) {
	app.Router.Query(path, func(ctx fiber.Ctx) error {
		gorestCtx := &Context{Ctx: ctx}

		return handler(gorestCtx)
	})
}

// Add allows you to specify multiple HTTP methods to register a route.
// The provided handlers are executed in order, starting with `handler` and then the variadic `handlers`.
func (app *Server) Add(methods []string, path string, handler HandlerFunc, handlers ...HandlerFunc) {
	wrappedHandler := func(ctx fiber.Ctx) error {
		gorestCtx := &Context{Ctx: ctx}
		return handler(gorestCtx)
	}

	wrappedHandlers := make([]any, 0, len(handlers))

	for _, h := range handlers {
		curr := h

		wrappedHandlers = append(wrappedHandlers, func(ctx fiber.Ctx) error {
			gorestCtx := &Context{Ctx: ctx}

			return curr(gorestCtx)
		})
	}

	app.Router.Add(methods, path, wrappedHandler, wrappedHandlers...)
}

// All will register the handler on all HTTP methods.
func (app *Server) All(path string, handler HandlerFunc) {
	app.Router.All(path, func(ctx fiber.Ctx) error {
		gorestCtx := &Context{Ctx: ctx}

		return handler(gorestCtx)
	})
}

// Group is used for Routes with common prefix to define a new sub-router with optional middleware.
//
//	api := app.Group("/api")
//	api.Get("/users", handler).
func (app *Server) Group(prefix string, handlers ...HandlerFunc) *Server {
	fiberHandlers := make([]any, 0, len(handlers))

	for _, h := range handlers {
		current := h

		fiberHandlers = append(fiberHandlers, func(ctx fiber.Ctx) error {
			gorestCtx := &Context{Ctx: ctx}
			return current(gorestCtx)
		})
	}

	subRouter := app.Router.Group(prefix, fiberHandlers...)

	return &Server{
		SRV:    app.SRV,
		Router: subRouter,
	}
}
