package gorest

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/redis"
)

func TestReadyHealthRouteReportsRedisDependency(t *testing.T) {
	tests := []struct {
		name       string
		redis      *redis.Config
		wantStatus int
		wantBody   string
	}{
		{
			name:       "redis disabled",
			wantStatus: http.StatusOK,
			wantBody:   `"redis":"disabled"`,
		},
		{
			name:       "redis service not running",
			redis:      &redis.Config{},
			wantStatus: http.StatusServiceUnavailable,
			wantBody:   `"redis":"not_initialized"`,
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

			response, err := app.server.SRV.Test(httptest.NewRequest(http.MethodGet, "/health/ready", nil))
			if err != nil {
				t.Fatalf("request readiness endpoint: %v", err)
			}
			defer response.Body.Close()

			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatalf("read readiness response: %v", err)
			}
			if response.StatusCode != test.wantStatus {
				t.Fatalf("readiness status = %d, want %d; body = %s", response.StatusCode, test.wantStatus, body)
			}
			if !strings.Contains(string(body), test.wantBody) {
				t.Fatalf("readiness body = %s, want it to contain %s", body, test.wantBody)
			}
		})
	}
}
