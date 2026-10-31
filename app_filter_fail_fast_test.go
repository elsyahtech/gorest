package gorest

import (
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/elsyahtech/gorest/filter"
	logimpl "github.com/elsyahtech/gorest/log"
	"github.com/elsyahtech/gorest/server"
)

func TestInvalidJWTFilterUsageAbortsStartup(t *testing.T) {
	const (
		childEnv  = "GOREST_INVALID_FILTER_CHILD"
		modeEnv   = "GOREST_INVALID_FILTER_MODE"
		logDirEnv = "GOREST_INVALID_FILTER_LOG_DIR"
	)

	if os.Getenv(childEnv) == "1" {
		logger, _, err := logimpl.New(&logimpl.Config{LogDir: os.Getenv(logDirEnv)}, "filter-fail-fast-test")
		if err != nil {
			t.Fatalf("initialize child test logger: %v", err)
		}
		app := &App{
			config:  Config{AppName: "filter-fail-fast-test"},
			log:     logger,
			modules: []string{},
		}

		switch os.Getenv(modeEnv) {
		case "missing-key":
			app.Use(filter.Config{})
		case "blank-key":
			key := " \t "
			app.Use(filter.Config{SignatureKey: &key})
		case "unsupported-signing-method":
			key := "test-signature-key"
			app.Use(filter.Config{SignatureKey: &key, SigningMethod: "unsupported"})
		case "invalid-token-duration":
			key := "test-signature-key"
			app.Use(filter.Config{SignatureKey: &key, AccessTokenDuration: -time.Second})
		case "filter-accessor":
			httpServer, _, err := server.New(nil)
			if err != nil {
				t.Fatalf("initialize child test server: %v", err)
			}
			app.server = httpServer
			app.RegisterServices([]RouterRegistrar{
				func(app *App) {
					group := app.Group("/orm-queries")
					group.Get("/users/select-all", app.Filter().Authentication(), func(*Context) error {
						return nil
					})
				},
			})
		case "direct-authentication":
			(&filter.Filter{}).Authentication()
		default:
			t.Fatalf("unknown child test mode %q", os.Getenv(modeEnv))
		}
		t.Fatal("invalid JWT filter usage returned without aborting startup")
	}

	tests := []struct {
		name string
		mode string
		want string
	}{
		{
			name: "missing signing key",
			mode: "missing-key",
			want: "JWT filter was configured without a signature key",
		},
		{
			name: "blank signing key",
			mode: "blank-key",
			want: "JWT filter was configured without a signature key",
		},
		{
			name: "unsupported signing method",
			mode: "unsupported-signing-method",
			want: "failed to run JWT filter",
		},
		{
			name: "invalid token duration",
			mode: "invalid-token-duration",
			want: "failed to run JWT filter",
		},
		{
			name: "protected route registration without JWT aborts before server start",
			mode: "filter-accessor",
			want: "JWT filter is not running",
		},
		{
			name: "creating middleware from uninitialized filter",
			mode: "direct-authentication",
			want: "JWT filter is not running",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			command := exec.Command(os.Args[0], "-test.run=^TestInvalidJWTFilterUsageAbortsStartup$")
			env := os.Environ()
			env = replaceTestEnv(env, childEnv, "1")
			env = replaceTestEnv(env, modeEnv, test.mode)
			env = replaceTestEnv(env, logDirEnv, t.TempDir())
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

func replaceTestEnv(env []string, key, value string) []string {
	prefix := key + "="
	for index, entry := range env {
		if strings.HasPrefix(entry, prefix) {
			env[index] = prefix + value
			return env
		}
	}
	return append(env, prefix+value)
}
