package server

import (
	golog "log"
)

// Get registers a route for GET methods that requests a representation
// of the specified resource. Requests using GET should only retrieve data.
func (app *Server) Get(path string, handler HandlerFunc, handlers ...HandlerFunc) {
	app.validateRegistration("Get", handler, handlers...)
	app.Router.Get(path, wrapHandler(handler), wrapHandlers(handlers...)...)
}

// Head registers a route for HEAD methods that asks for a response identical
// to that of a GET request, but without the response body.
func (app *Server) Head(path string, handler HandlerFunc, handlers ...HandlerFunc) {
	app.validateRegistration("Head", handler, handlers...)
	app.Router.Head(path, wrapHandler(handler), wrapHandlers(handlers...)...)
}

// Post registers a route for POST methods that is used to submit an entity to the
// specified resource, often causing a change in state or side effects on the server.
func (app *Server) Post(path string, handler HandlerFunc, handlers ...HandlerFunc) {
	app.validateRegistration("Post", handler, handlers...)
	app.Router.Post(path, wrapHandler(handler), wrapHandlers(handlers...)...)
}

// Put registers a route for PUT methods that replaces all current representations
// of the target resource with the request payload.
func (app *Server) Put(path string, handler HandlerFunc, handlers ...HandlerFunc) {
	app.validateRegistration("Put", handler, handlers...)
	app.Router.Put(path, wrapHandler(handler), wrapHandlers(handlers...)...)
}

// Delete registers a route for DELETE methods that deletes the specified resource.
func (app *Server) Delete(path string, handler HandlerFunc, handlers ...HandlerFunc) {
	app.validateRegistration("Delete", handler, handlers...)
	app.Router.Delete(path, wrapHandler(handler), wrapHandlers(handlers...)...)
}

// Connect registers a route for CONNECT methods that establishes a tunnel to the
// server identified by the target resource.
func (app *Server) Connect(path string, handler HandlerFunc, handlers ...HandlerFunc) {
	app.validateRegistration("Connect", handler, handlers...)
	app.Router.Connect(path, wrapHandler(handler), wrapHandlers(handlers...)...)
}

// Options registers a route for OPTIONS methods that is used to describe the
// communication options for the target resource.
func (app *Server) Options(path string, handler HandlerFunc, handlers ...HandlerFunc) {
	app.validateRegistration("Options", handler, handlers...)
	app.Router.Options(path, wrapHandler(handler), wrapHandlers(handlers...)...)
}

// Trace registers a route for TRACE methods that performs a message loop-back
// test along the path to the target resource.
func (app *Server) Trace(path string, handler HandlerFunc, handlers ...HandlerFunc) {
	app.validateRegistration("Trace", handler, handlers...)
	app.Router.Trace(path, wrapHandler(handler), wrapHandlers(handlers...)...)
}

// Patch registers a route for PATCH methods that is used to apply partial
// modifications to a resource.
func (app *Server) Patch(path string, handler HandlerFunc, handlers ...HandlerFunc) {
	app.validateRegistration("Patch", handler, handlers...)
	app.Router.Patch(path, wrapHandler(handler), wrapHandlers(handlers...)...)
}

// Query registers a route for QUERY methods that performs a safe, idempotent
// query with a request body.
func (app *Server) Query(path string, handler HandlerFunc, handlers ...HandlerFunc) {
	app.validateRegistration("Query", handler, handlers...)
	app.Router.Query(path, wrapHandler(handler), wrapHandlers(handlers...)...)
}

// Add allows you to specify multiple HTTP methods to register a route.
// The provided handlers are executed in order, starting with `handler` and then the variadic `handlers`.
func (app *Server) Add(methods []string, path string, handler HandlerFunc, handlers ...HandlerFunc) {
	app.validateRegistration("Add", handler, handlers...)
	app.Router.Add(methods, path, wrapHandler(handler), wrapHandlers(handlers...)...)
}

// All will register the handler on all HTTP methods.
func (app *Server) All(path string, handler HandlerFunc, handlers ...HandlerFunc) {
	app.validateRegistration("All", handler, handlers...)
	app.Router.All(path, wrapHandler(handler), wrapHandlers(handlers...)...)
}

// Group is used for Routes with common prefix to define a new sub-router with optional middleware.
//
//	api := app.Group("/api")
//	api.Get("/users", handler).
func (app *Server) Group(prefix string, handlers ...HandlerFunc) *Server {
	app.validateGroup("Group", handlers...)
	subRouter := app.Router.Group(prefix, wrapHandlers(handlers...)...)

	return &Server{
		SRV:    app.SRV,
		Router: subRouter,
	}
}

func (app *Server) validateServer(method string) {
	if app == nil {
		golog.Fatalf(
			"cannot register %s route: server is not running. Ensure server.Run(...) is registered with app.Use(...) before registering routes.",
			method,
		)
	}
	if app.Router == nil || app.SRV == nil {
		golog.Fatalf(
			"cannot register %s route: router is not running. Ensure server.Run(...) is registered with app.Use(...) before registering routes.",
			method,
		)
	}
}

func (app *Server) validateRegistration(method string, handler HandlerFunc, handlers ...HandlerFunc) {
	app.validateServer(method)
	if handler == nil {
		golog.Fatalf("cannot register %s route: route handler is nil", method)
	}
	validateHandlers(method, handlers...)
}

func (app *Server) validateGroup(method string, handlers ...HandlerFunc) {
	app.validateServer(method)
	validateHandlers(method, handlers...)
}

func validateHandlers(method string, handlers ...HandlerFunc) {
	for index, handler := range handlers {
		if handler == nil {
			golog.Fatalf("cannot register %s route: handler %d in the route chain is nil", method, index+1)
		}
	}
}

func wrapHandlers(handlers ...HandlerFunc) []any {
	fiberHandlers := make([]any, 0, len(handlers))
	for _, handler := range handlers {
		fiberHandlers = append(fiberHandlers, wrapHandler(handler))
	}

	return fiberHandlers
}

func wrapHandler(handler HandlerFunc) func(Context) error {
	current := handler
	return func(ctx Context) error {
		return current(&Context{Ctx: ctx.Ctx})
	}
}
