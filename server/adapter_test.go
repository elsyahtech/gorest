package server

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestRouteRegistrationRejectsInvalidInputs(t *testing.T) {
	const (
		childEnv = "GOREST_SERVER_ADAPTER_CHILD"
		modeEnv  = "GOREST_SERVER_ADAPTER_MODE"
	)

	if os.Getenv(childEnv) == "1" {
		runInvalidRouteRegistration(os.Getenv(modeEnv))
		t.Fatal("invalid route registration returned without terminating")
	}

	tests := []struct {
		name string
		mode string
		want string
	}{
		{name: "nil server receiver", mode: "nil-server", want: "server is not running"},
		{name: "nil router", mode: "nil-router", want: "router is not running"},
		{name: "nil Fiber app", mode: "nil-fiber-app", want: "router is not running"},
		{name: "nil required handler", mode: "nil-handler", want: "route handler is nil"},
		{name: "nil chained handler", mode: "nil-chain-handler", want: "handler 1 in the route chain is nil"},
		{name: "nil group middleware", mode: "nil-group-handler", want: "handler 1 in the route chain is nil"},
	}
	for _, method := range []string{
		"Get", "Head", "Post", "Put", "Delete", "Connect",
		"Options", "Trace", "Patch", "Query", "Add", "All",
	} {
		tests = append(tests, struct {
			name string
			mode string
			want string
		}{
			name: method + " rejects nil handler",
			mode: "nil-handler-" + method,
			want: "route handler is nil",
		})
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestRouteRegistrationRejectsInvalidInputs$")
			env := setTestEnv(os.Environ(), childEnv, "1")
			env = setTestEnv(env, modeEnv, test.mode)
			command.Env = env

			output, err := command.CombinedOutput()
			exitErr, ok := err.(*exec.ExitError)
			if !ok || exitErr.ExitCode() != 1 {
				t.Fatalf("child process error = %v, output: %s; want exit status 1", err, output)
			}
			if !strings.Contains(string(output), test.want) {
				t.Fatalf("child output = %q, want it to contain %q", output, test.want)
			}
		})
	}
}

func runInvalidRouteRegistration(mode string) {
	if mode == "nil-server" {
		var app *Server
		app.Get("/", nil)
		return
	}
	if mode == "nil-router" {
		(&Server{}).Get("/", func(*Context) error { return nil })
		return
	}

	app, _, err := New(nil)
	if err != nil {
		panic(err)
	}
	if mode == "nil-fiber-app" {
		app.SRV = nil
		app.Get("/", func(*Context) error { return nil })
		return
	}

	validHandler := HandlerFunc(func(*Context) error { return nil })
	switch mode {
	case "nil-handler":
		app.Get("/", nil)
	case "nil-chain-handler":
		app.Get("/", validHandler, nil)
	case "nil-group-handler":
		app.Group("/", nil)
	default:
		registerRouteWithNilHandler(app, strings.TrimPrefix(mode, "nil-handler-"))
	}
}

func registerRouteWithNilHandler(app *Server, method string) {
	switch method {
	case "Get":
		app.Get("/", nil)
	case "Head":
		app.Head("/", nil)
	case "Post":
		app.Post("/", nil)
	case "Put":
		app.Put("/", nil)
	case "Delete":
		app.Delete("/", nil)
	case "Connect":
		app.Connect("/", nil)
	case "Options":
		app.Options("/", nil)
	case "Trace":
		app.Trace("/", nil)
	case "Patch":
		app.Patch("/", nil)
	case "Query":
		app.Query("/", nil)
	case "Add":
		app.Add([]string{"GET"}, "/", nil)
	case "All":
		app.All("/", nil)
	default:
		panic("unknown route method: " + method)
	}
}

func setTestEnv(env []string, key, value string) []string {
	prefix := key + "="
	for index, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[index] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}
