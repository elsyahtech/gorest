package gorest

import (
	"reflect"
	"testing"

	"github.com/elsyahtech/gorest/server"
)

func TestOptionalServiceInitializersSkipUnconfiguredServices(t *testing.T) {
	app := &App{
		log:      newLifecycleTestLogger(t),
		config:   Config{AppName: "initializer-test", Timezone: "UTC"},
		services: &service{},
	}

	app.redisInit()
	if app.redis != nil {
		t.Fatal("redisInit() initialized Redis without a client configuration")
	}

	app.databaseInit()
	if app.database != nil {
		t.Fatal("databaseInit() initialized a database without a client configuration")
	}
}

func TestNewServerUsesConfiguredDefaultsAndRegistersService(t *testing.T) {
	app := &App{
		log:      newLifecycleTestLogger(t),
		config:   Config{AppName: "server-init-test"},
		services: &service{},
	}
	app.newServer()
	if app.server == nil {
		t.Fatal("newServer() did not initialize the server")
	}
	if app.config.server != nil {
		t.Fatal("newServer() unexpectedly changed the application server configuration")
	}

	app.config.server = nil
	app.serverInit()
	if app.server == nil || app.config.server == nil {
		t.Fatal("serverInit() did not install default server configuration")
	}
	if app.config.server.Port != server.ConfigDefault.Port {
		t.Fatalf("server default port = %d, want %d", app.config.server.Port, server.ConfigDefault.Port)
	}

	if !reflect.DeepEqual(app.modules, []string{"Server"}) {
		t.Fatalf("server initialization registered modules = %v, want [Server]", app.modules)
	}
}
