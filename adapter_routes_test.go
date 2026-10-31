package gorest

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/log"
	"github.com/elsyahtech/gorest/server"
	"github.com/gofiber/fiber/v3/middleware/limiter"
)

func TestAppRouteRegistrationMethods(t *testing.T) {
	httpServer, _, err := server.New(&server.Config{
		Limiter: &limiter.Config{Max: 100, Expiration: time.Minute},
	})
	if err != nil {
		t.Fatalf("server.New() error = %v", err)
	}
	app := &App{server: httpServer}
	handler := func(ctx *Context) error {
		return ctx.SendString("registered")
	}

	registrations := []struct {
		name   string
		method string
		path   string
		add    func()
	}{
		{name: "GET", method: http.MethodGet, path: "/get", add: func() { app.Get("/get", handler) }},
		{name: "HEAD", method: http.MethodHead, path: "/head", add: func() { app.Head("/head", handler) }},
		{name: "POST", method: http.MethodPost, path: "/post", add: func() { app.Post("/post", handler) }},
		{name: "PUT", method: http.MethodPut, path: "/put", add: func() { app.Put("/put", handler) }},
		{name: "DELETE", method: http.MethodDelete, path: "/delete", add: func() { app.Delete("/delete", handler) }},
		{name: "CONNECT", method: http.MethodConnect, path: "/connect", add: func() { app.Connect("/connect", handler) }},
		{name: "OPTIONS", method: http.MethodOptions, path: "/options", add: func() { app.Options("/options", handler) }},
		{name: "TRACE", method: http.MethodTrace, path: "/trace", add: func() { app.Trace("/trace", handler) }},
		{name: "PATCH", method: http.MethodPatch, path: "/patch", add: func() { app.Patch("/patch", handler) }},
		{name: "QUERY", method: "QUERY", path: "/query", add: func() { app.Query("/query", handler) }},
		{name: "Add", method: http.MethodGet, path: "/add", add: func() {
			app.Add([]string{http.MethodGet}, "/add", handler)
		}},
		{name: "All", method: http.MethodGet, path: "/all", add: func() { app.All("/all", handler) }},
	}
	for _, registration := range registrations {
		registration := registration
		t.Run(registration.name, func(t *testing.T) {
			registration.add()
			request := httptest.NewRequest(registration.method, registration.path, nil)
			response, err := httpServer.SRV.Test(request)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
			}
			if registration.method != http.MethodHead {
				body, err := io.ReadAll(response.Body)
				if err != nil {
					t.Fatalf("read response: %v", err)
				}
				if string(body) != "registered" {
					t.Fatalf("body = %q, want %q", body, "registered")
				}
			}
		})
	}

	group := app.Group("/group")
	group.Get("/child", handler)
	response, err := httpServer.SRV.Test(httptest.NewRequest(http.MethodGet, "/group/child", nil))
	if err != nil {
		t.Fatalf("group request failed: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("group status = %d, want %d", response.StatusCode, http.StatusOK)
	}
}

func TestAppConfigurationGettersAndContext(t *testing.T) {
	app := &App{
		context: httptest.NewRequest(http.MethodGet, "/", nil).Context(),
		config: Config{
			AppName:     "coverage",
			Version:     "1.2.3",
			Environment: "testing",
			Timezone:    "UTC",
			Debug:       true,
			database:    &database.Config{Driver: database.SQLITE},
			log:         &log.Config{LogFile: "coverage.log"},
		},
	}

	if app.Name() != "coverage" || app.Version() != "1.2.3" ||
		app.Environment() != "testing" || app.Timezone() != "UTC" || !app.IsDebug() {
		t.Fatalf("application getters returned unexpected configuration: %#v", app.config)
	}
	if app.GetDBConfig().Driver != database.SQLITE || app.GetLogConfig().LogFile != "coverage.log" {
		t.Fatalf("configuration getters returned database=%#v log=%#v", app.GetDBConfig(), app.GetLogConfig())
	}
	if got := app.GetServerConfig(); got != (server.Config{}) {
		t.Errorf("GetServerConfig() = %#v, want zero config", got)
	}

	ctx, cancel := app.NewContext(0)
	defer cancel()
	if ctx != app.context {
		t.Fatal("NewContext() replaced a context that already had no timeout requested")
	}

	dbSession := app.Database()
	if dbSession == nil || dbSession.config == nil || dbSession.config.Driver != database.SQLITE {
		t.Fatalf("Database() = %#v, want session with configured database", dbSession)
	}
	if (*App)(nil).Database() == nil || (*App)(nil).GetDBConfig() != (database.Config{}) {
		t.Fatal("nil application accessors did not return their documented empty values")
	}
	if got := (*App)(nil).GetServerConfig(); got != (server.Config{}) {
		t.Errorf("nil GetServerConfig() = %#v, want zero config", got)
	}
	if got := (*App)(nil).GetLogConfig(); got != (log.Config{}) {
		t.Errorf("nil GetLogConfig() = %#v, want zero config", got)
	}
}

func TestRegisterDefaultRoute(t *testing.T) {
	httpServer, _, err := server.New(&server.Config{
		Limiter: &limiter.Config{Max: 10, Expiration: time.Minute},
	})
	if err != nil {
		t.Fatalf("server.New() error = %v", err)
	}
	RegisterDefaultRoutes(&App{server: httpServer})

	response, err := httpServer.SRV.Test(httptest.NewRequest(http.MethodGet, "/", nil))
	if err != nil {
		t.Fatalf("default route request failed: %v", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read default route response: %v", err)
	}
	if response.StatusCode != http.StatusOK || !strings.Contains(string(body), "<title>GOREST</title>") {
		t.Fatalf("default route response status=%d body lacks GOREST title", response.StatusCode)
	}
	if contentType := response.Header.Get("Content-Type"); !strings.Contains(contentType, "text/html") {
		t.Fatalf("content type = %q, want HTML", contentType)
	}
}
