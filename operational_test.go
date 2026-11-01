package gorest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/redis"
)

func TestFormatUptime(t *testing.T) {
	tests := []struct {
		name string
		got  time.Duration
		want string
	}{
		{name: "days", got: 4320 * time.Hour, want: "180 days"},
		{name: "mixed units", got: 26*time.Hour + 3*time.Minute + 5*time.Second, want: "1 day, 2 hours, 3 minutes, 5 seconds"},
		{name: "less than a second", got: 500 * time.Millisecond, want: "less than a second"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := formatUptime(test.got); got != test.want {
				t.Errorf("formatUptime(%s) = %q, want %q", test.got, got, test.want)
			}
		})
	}
}

func TestRegisterOperationalRoutesCanBeDisabled(t *testing.T) {
	app := New(Config{DisableMonitoringEndpoints: true})
	t.Cleanup(func() {
		if _, _, err := app.Close(); err != nil {
			t.Errorf("close application: %v", err)
		}
	})

	app.registerOperationalRoutes()
	for _, path := range []string{"/gorest/live", "/gorest/ready", "/gorest/metrics", "/gorest/storage"} {
		response, err := app.server.SRV.Test(httptest.NewRequest(http.MethodGet, path, nil))
		if err != nil {
			t.Fatalf("request disabled monitoring endpoint %s: %v", path, err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusNotFound {
			t.Errorf("disabled monitoring endpoint %s status = %d, want %d", path, response.StatusCode, http.StatusNotFound)
		}
	}
}

func TestReadyHealthRouteReportsRedisDependency(t *testing.T) {
	tests := []struct {
		name                string
		redis               *redis.Config
		databaseUnavailable bool
		wantStatus          int
		wantStorageStatus   int
		wantBody            []string
	}{
		{
			name:              "optional services are not configured",
			wantStatus:        http.StatusOK,
			wantStorageStatus: http.StatusOK,
			wantBody: []string{
				`"logger":"healthy"`,
				`"server":"healthy"`,
				`"database":"healthy"`,
				`"redis":"not running"`,
				`"filter":"not running"`,
				`"status":"ready"`,
			},
		},
		{
			name:              "redis service not running",
			redis:             &redis.Config{},
			wantStatus:        http.StatusServiceUnavailable,
			wantStorageStatus: http.StatusOK,
			wantBody: []string{
				`"database":"healthy"`,
				`"redis":"not running"`,
				`"status":"not_ready"`,
			},
		},
		{
			name:                "database is unhealthy",
			databaseUnavailable: true,
			wantStatus:          http.StatusServiceUnavailable,
			wantStorageStatus:   http.StatusServiceUnavailable,
			wantBody: []string{
				`"database":"unhealthy"`,
				`"redis":"not running"`,
				`"status":"not_ready"`,
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := New()
			t.Cleanup(func() {
				if _, _, err := app.Close(); err != nil {
					t.Errorf("close health-check application: %v", err)
				}
			})

			config := database.Config{
				Driver:             database.SQLITE,
				Name:               "readiness",
				DatabaseDirectory:  t.TempDir(),
				MaxOpenConnections: 1,
			}
			db, _, err := database.New(context.Background(), &config, "UTC")
			if err != nil {
				t.Fatalf("initialize SQLite health-check database: %v", err)
			}
			if db.SQL == nil {
				t.Fatal("SQLite health-check database has no SQL connection")
			}
			if err := db.SQL.PingContext(context.Background()); err != nil {
				t.Fatalf("ping SQLite health-check database: %v", err)
			}

			app.database = db
			app.config.database = &config
			app.config.redis = test.redis
			app.registerOperationalRoutes()
			if test.databaseUnavailable {
				if err := db.SQL.Close(); err != nil {
					t.Fatalf("close SQLite health-check database: %v", err)
				}
			}

			for _, path := range []string{"/health/live", "/health/ready", "/metrics"} {
				response, err := app.server.SRV.Test(httptest.NewRequest(http.MethodGet, path, nil))
				if err != nil {
					t.Fatalf("request removed operational endpoint %s: %v", path, err)
				}
				response.Body.Close()
				if response.StatusCode != http.StatusNotFound {
					t.Errorf("removed operational endpoint %s status = %d, want %d", path, response.StatusCode, http.StatusNotFound)
				}
			}

			for _, path := range []string{"/gorest/ready"} {
				response, err := app.server.SRV.Test(httptest.NewRequest(http.MethodGet, path, nil))
				if err != nil {
					t.Fatalf("request readiness endpoint %s: %v", path, err)
				}

				body, err := io.ReadAll(response.Body)
				response.Body.Close()
				if err != nil {
					t.Fatalf("read readiness response from %s: %v", path, err)
				}
				if response.StatusCode != test.wantStatus {
					t.Fatalf("readiness status for %s = %d, want %d; body = %s", path, response.StatusCode, test.wantStatus, body)
				}
				for _, wantBody := range test.wantBody {
					if !strings.Contains(string(body), wantBody) {
						t.Errorf("readiness body from %s = %s, want it to contain %s", path, body, wantBody)
					}
				}
			}

			response, err := app.server.SRV.Test(httptest.NewRequest(http.MethodGet, "/gorest/storage", nil))
			if err != nil {
				t.Fatalf("request storage endpoint: %v", err)
			}
			body, err := io.ReadAll(response.Body)
			response.Body.Close()
			if err != nil {
				t.Fatalf("read storage response: %v", err)
			}
			if response.StatusCode != test.wantStorageStatus {
				t.Errorf("storage status = %d, want %d; body = %s", response.StatusCode, test.wantStorageStatus, body)
			}
		})
	}
}
