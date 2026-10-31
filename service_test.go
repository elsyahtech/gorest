package gorest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/filter"
	"github.com/elsyahtech/gorest/log"
	"github.com/elsyahtech/gorest/redis"
	"github.com/elsyahtech/gorest/server"
)

func TestUseConfiguresDisabledServicesServerAndFilter(t *testing.T) {
	app := &App{
		context:  context.Background(),
		services: &service{},
		modules:  []string{},
		config:   Config{AppName: "service-test"},
	}
	logConfig := log.Config{LogDir: t.TempDir(), LogFile: "service.log"}
	disabledRedis := &redis.Config{Disable: true}
	serverConfig := &server.Config{}

	if got := app.Use(
		logConfig,
		database.Config{Disable: true, Driver: database.SQLITE},
		disabledRedis,
		serverConfig,
	); got != app {
		t.Fatal("Use() did not return the application")
	}
	if app.log == nil || app.server == nil {
		t.Fatal("Use() did not initialize the logger and server")
	}
	if app.config.database != nil || app.database != nil || app.config.redis != nil || app.redis != nil {
		t.Fatal("disabled database or Redis configuration was retained")
	}
	if app.config.server != serverConfig || app.config.filter != nil || app.filter != nil {
		t.Fatal("server or invalid filter configuration was not handled as expected")
	}
	if len(app.modules) != 2 || app.modules[0] != "Logger" || app.modules[1] != "Server" {
		t.Fatalf("registered modules = %#v, want Logger and Server", app.modules)
	}

	secret := "unit-test-signing-key-with-sufficient-length"
	filterConfig := &filter.Config{SignatureKey: &secret}
	app.Use(filterConfig)
	if app.filter == nil || app.config.filter == nil || app.config.filter.Issuer != app.config.AppName {
		t.Fatal("valid pointer filter configuration was not applied with the application issuer")
	}
	if len(app.modules) != 3 || app.modules[2] != "Filter" {
		t.Fatalf("registered modules = %#v, want Logger, Server, and Filter", app.modules)
	}
	if _, _, err := app.log.Close(); err != nil {
		t.Fatalf("closing test logger: %v", err)
	}
}

func TestRegisterServiceAndRouterRegistry(t *testing.T) {
	app := &App{
		services: &service{},
		modules:  []string{},
	}
	app.registerService("Logger")
	app.registerService("Logger")
	app.registerService("Database")
	if len(app.modules) != 2 || app.modules[0] != "Logger" || app.modules[1] != "Database" {
		t.Fatalf("registered modules = %#v, want unique services in order", app.modules)
	}

	if srv, message, err := app.routerRegistry(); err == nil || srv != nil || message == "" {
		t.Fatalf("routerRegistry() without server = (%v, %q, %v), want an initialization error", srv, message, err)
	}

	httpServer, _, err := server.New(nil)
	if err != nil {
		t.Fatalf("server.New() error = %v", err)
	}
	app.server = httpServer
	registrar := RouterRegistrar(func(app *App) {
		app.Get("/registered", func(ctx *Context) error {
			return ctx.SendString("ready")
		})
	})
	srv, message, err := app.routerRegistry(nil, registrar)
	if err != nil || message != "" || srv != httpServer.SRV {
		t.Fatalf("routerRegistry() = (%v, %q, %v), want initialized server", srv, message, err)
	}

	response, err := srv.Test(httptest.NewRequest(http.MethodGet, "/registered", nil))
	if err != nil {
		t.Fatalf("registered route request failed: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("registered route status = %d, want %d", response.StatusCode, http.StatusOK)
	}
}

func TestRegisterServicesStoresAndDispatchesRoutes(t *testing.T) {
	httpServer, _, err := server.New(nil)
	if err != nil {
		t.Fatalf("server.New() error = %v", err)
	}
	app := &App{
		server:   httpServer,
		services: &service{},
	}
	registrar := RouterRegistrar(func(app *App) {
		app.Get("/service-route", func(ctx *Context) error {
			return ctx.SendString("registered")
		})
	})

	app.RegisterServices([]RouterRegistrar{nil, registrar})
	if len(app.services.routers) != 2 || app.services.routers[0] != nil {
		t.Fatalf("stored route registrars = %#v, want both provided entries", app.services.routers)
	}

	response, err := httpServer.SRV.Test(httptest.NewRequest(http.MethodGet, "/service-route", nil))
	if err != nil {
		t.Fatalf("registered service route request failed: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("registered service route status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	app.RegisterServices()
	if len(app.services.routers) != 0 {
		t.Fatalf("RegisterServices() with no routes retained %d routes", len(app.services.routers))
	}
}
