package gorest

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/monitoring"
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
	app := New().Use(monitoring.Config{DisableMonitoringEndpoints: true})
	for _, module := range app.modules {
		if module == "Monitoring" {
			t.Fatal("disabled monitoring service remains in the startup modules")
		}
	}
	t.Cleanup(func() {
		if _, _, err := app.Close(); err != nil {
			t.Errorf("close application: %v", err)
		}
	})

	app.registerOperationalRoutes()
	disabledHTTPApp := New().Use(monitoring.Config{DisableMonitoringEndpoints: true})
	disabledHTTPApp.registerOperationalRoutes()
	response, err := disabledHTTPApp.server.SRV.Test(httptest.NewRequest(http.MethodGet, "/gorest/http", nil))
	if err != nil {
		t.Fatalf("request disabled HTTP metrics endpoint: %v", err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusNotFound {
		t.Errorf("disabled HTTP metrics endpoint status = %d, want %d", response.StatusCode, http.StatusNotFound)
	}
	if _, _, err := disabledHTTPApp.Close(); err != nil {
		t.Errorf("close disabled HTTP metrics application: %v", err)
	}

	for _, path := range []string{"/gorest/live", "/gorest/ready", "/gorest/metrics", "/gorest/storage", "/gorest/database"} {
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

func TestOperationalEndpointsDenyAccessByDefaultAndIgnoreForwardedIP(t *testing.T) {
	for _, path := range []string{
		"/gorest/live", "/gorest/ready", "/gorest/metrics",
		"/gorest/http", "/gorest/storage", "/gorest/database",
	} {
		app := New()
		app.registerOperationalRoutes()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("X-Forwarded-For", "0.0.0.0")
		response, err := app.server.SRV.Test(request)
		if err != nil {
			t.Fatalf("request protected operational endpoint %s: %v", path, err)
		}

		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatalf("read protected operational endpoint %s response: %v", path, err)
		}
		if response.StatusCode != http.StatusForbidden {
			t.Errorf("%s status = %d, want %d", path, response.StatusCode, http.StatusForbidden)
		}
		if string(body) != `{"status":"forbidden"}` {
			t.Errorf("%s forbidden response = %s, want minimal forbidden response", path, body)
		}
		if _, _, err := app.Close(); err != nil {
			t.Errorf("close application after testing %s: %v", path, err)
		}
	}
}

func TestOperationalEndpointsUseConfiguredPeerIPAllowlist(t *testing.T) {
	for _, path := range []string{
		"/gorest/live", "/gorest/ready", "/gorest/metrics",
		"/gorest/http", "/gorest/storage", "/gorest/database",
	} {
		app := New().Use(monitoring.Config{HTTPMetricsAllowedIPs: []string{"0.0.0.0/0"}})
		app.registerOperationalRoutes()
		response, err := app.server.SRV.Test(httptest.NewRequest(http.MethodGet, path, nil))
		if err != nil {
			t.Fatalf("request allowlisted operational endpoint %s: %v", path, err)
		}

		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatalf("read allowlisted operational endpoint %s response: %v", path, err)
		}
		if response.StatusCode == http.StatusForbidden || string(body) == `{"status":"forbidden"}` {
			t.Errorf("%s was denied despite matching the configured peer IP allowlist", path)
		}
		if _, _, err := app.Close(); err != nil {
			t.Errorf("close application after testing %s: %v", path, err)
		}
	}
}

func TestOperationalRoutesAreRegisteredIndividually(t *testing.T) {
	app := New().Use(monitoring.Config{HTTPMetricsAllowedIPs: []string{"0.0.0.0/0"}})
	t.Cleanup(func() {
		if _, _, err := app.Close(); err != nil {
			t.Errorf("close application after checking operational routes: %v", err)
		}
	})
	app.registerOperationalRoutes()

	paths := map[string]bool{}
	for _, route := range app.server.SRV.GetRoutes() {
		if route.Path == "/gorest" {
			t.Fatalf("found synthetic /gorest route with method %s", route.Method)
		}
		if strings.HasPrefix(route.Path, "/gorest/") {
			paths[route.Path] = true
		}
	}

	for _, path := range []string{
		"/gorest/database",
		"/gorest/storage",
		"/gorest/http",
		"/gorest/live",
		"/gorest/ready",
		"/gorest/metrics",
	} {
		if !paths[path] {
			t.Errorf("explicit operational route %q was not registered", path)
		}
	}
}

func TestRuntimeMetricsEndpointReportsNestedRuntimeStats(t *testing.T) {
	app := New().Use(monitoring.Config{HTTPMetricsAllowedIPs: []string{"0.0.0.0/0"}})
	t.Cleanup(func() {
		if _, _, err := app.Close(); err != nil {
			t.Errorf("close runtime metrics application: %v", err)
		}
	})
	app.registerOperationalRoutes()

	response, err := app.server.SRV.Test(httptest.NewRequest(http.MethodGet, "/gorest/metrics", nil))
	if err != nil {
		t.Fatalf("request runtime metrics endpoint: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("runtime metrics status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read runtime metrics response: %v", err)
	}
	var result runtimeMetricsResponse
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("decode runtime metrics response: %v; body: %s", err, body)
	}

	if result.Service != app.config.AppName {
		t.Errorf("service = %q, want %q", result.Service, app.config.AppName)
	}
	if result.Runtime.Goroutines <= 0 || result.Runtime.GOMAXPROCS <= 0 {
		t.Errorf("runtime gauges = %#v, want positive goroutine and GOMAXPROCS counts", result.Runtime)
	}
	if result.Memory.HeapAllocBytes == 0 || result.Memory.HeapSysBytes == 0 ||
		result.Memory.HeapLiveBytes == 0 ||
		result.Memory.TotalSysBytes == 0 || result.Memory.StackSysBytes == 0 {
		t.Errorf("memory metrics are missing: %#v", result.Memory)
	}
	if result.GC.HeapGoalBytes == 0 || result.GC.HeapLiveBytes == 0 {
		t.Errorf("GC heap metrics are missing: %#v", result.GC)
	}
	if len(result.Scheduler.LatencyDistribution.BucketBoundsSeconds) == 0 ||
		len(result.Scheduler.LatencyDistribution.BucketBoundsSeconds) != len(result.Scheduler.LatencyDistribution.Counts)+1 {
		t.Errorf("scheduler latency histogram has invalid bounds/counts: %#v", result.Scheduler.LatencyDistribution)
	}
	if result.Scheduler.MutexWaitTotal < 0 {
		t.Errorf("mutex_wait_total_seconds = %v, want non-negative", result.Scheduler.MutexWaitTotal)
	}
}

func TestHTTPMetricsEndpointUsesApplicationSnapshotAndRouteTemplates(t *testing.T) {
	app := New().Use(monitoring.Config{
		HTTPMetricsAllowedIPs:     []string{"0.0.0.0/0"},
		EnablePerRouteHTTPMetrics: true,
	})
	t.Cleanup(func() {
		if _, _, err := app.Close(); err != nil {
			t.Errorf("close HTTP metrics application: %v", err)
		}
	})

	app.Get("/users/:id", func(ctx *Context) error {
		return ctx.SendStatus(http.StatusNoContent)
	})
	compressedPayload := strings.Repeat("compressible-response-", 100)
	app.Get("/compressed", func(ctx *Context) error {
		ctx.Set("Content-Type", "text/plain")
		return ctx.SendString(compressedPayload)
	})
	app.registerOperationalRoutes()

	response, err := app.server.SRV.Test(httptest.NewRequest(http.MethodGet, "/users/user-1", nil))
	if err != nil {
		t.Fatalf("request user route before metrics snapshot: %v", err)
	}
	response.Body.Close()
	compressedRequest := httptest.NewRequest(http.MethodGet, "/compressed", nil)
	compressedRequest.Header.Set("Accept-Encoding", "gzip")
	response, err = app.server.SRV.Test(compressedRequest)
	if err != nil {
		t.Fatalf("request compressed response route: %v", err)
	}
	if response.Header.Get("Content-Encoding") != "gzip" {
		t.Errorf("compressed route Content-Encoding = %q, want gzip", response.Header.Get("Content-Encoding"))
	}
	response.Body.Close()

	readSnapshot := func() struct {
		Status  string `json:"status"`
		Metrics struct {
			RequestTotal       uint64  `json:"request_total"`
			ResponseBytesTotal *uint64 `json:"response_bytes_total"`
			Routes             []struct {
				Method  string `json:"method"`
				Route   string `json:"route"`
				Metrics struct {
					RequestTotal uint64 `json:"request_total"`
				} `json:"metrics"`
			} `json:"routes"`
		} `json:"metrics"`
	} {
		t.Helper()
		request := httptest.NewRequest(http.MethodGet, "/gorest/http", nil)
		request.Header.Set("X-Forwarded-For", "192.0.2.9")
		response, requestErr := app.server.SRV.Test(request)
		if requestErr != nil {
			t.Fatalf("request HTTP metrics endpoint: %v", requestErr)
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			body, readErr := io.ReadAll(response.Body)
			if readErr != nil {
				t.Fatalf("read failed HTTP metrics response: %v", readErr)
			}
			t.Fatalf("HTTP metrics status = %d, want %d; response: %s", response.StatusCode, http.StatusOK, body)
		}

		body, readErr := io.ReadAll(response.Body)
		if readErr != nil {
			t.Fatalf("read HTTP metrics response: %v", readErr)
		}
		var result struct {
			Status  string `json:"status"`
			Metrics struct {
				RequestTotal       uint64  `json:"request_total"`
				ResponseBytesTotal *uint64 `json:"response_bytes_total"`
				Routes             []struct {
					Method  string `json:"method"`
					Route   string `json:"route"`
					Metrics struct {
						RequestTotal uint64 `json:"request_total"`
					} `json:"metrics"`
				} `json:"routes"`
			} `json:"metrics"`
		}
		if err := json.Unmarshal(body, &result); err != nil {
			t.Fatalf("decode HTTP metrics response: %v; body: %s", err, body)
		}
		return result
	}

	first := readSnapshot()
	if first.Status != "available" {
		t.Errorf("HTTP metrics status = %q, want available", first.Status)
	}
	if first.Metrics.RequestTotal != 2 {
		t.Errorf("first request_total = %d, want 2; current metrics request is observed after snapshot", first.Metrics.RequestTotal)
	}
	if first.Metrics.ResponseBytesTotal == nil || *first.Metrics.ResponseBytesTotal != uint64(len(compressedPayload)) {
		t.Errorf("pre-compression response_bytes_total = %v, want %d",
			first.Metrics.ResponseBytesTotal, len(compressedPayload))
	}
	var userRouteCount uint64
	for _, route := range first.Metrics.Routes {
		if route.Method == http.MethodGet && route.Route == "/users/:id" {
			userRouteCount = route.Metrics.RequestTotal
		}
		if strings.Contains(route.Route, "user-1") {
			t.Errorf("route label leaked dynamic path value: %q", route.Route)
		}
	}
	if userRouteCount != 1 {
		t.Errorf("GET /users/:id request_total = %d, want 1", userRouteCount)
	}

	second := readSnapshot()
	if second.Metrics.RequestTotal != 3 {
		t.Errorf("second request_total = %d, want 3; counters should include the previous metrics request", second.Metrics.RequestTotal)
	}

	legacyResponse, legacyErr := app.server.SRV.Test(httptest.NewRequest(http.MethodGet, "/gorest/metrics", nil))
	if legacyErr != nil {
		t.Fatalf("request existing runtime metrics endpoint: %v", legacyErr)
	}
	legacyBody, readErr := io.ReadAll(legacyResponse.Body)
	legacyResponse.Body.Close()
	if readErr != nil {
		t.Fatalf("read existing runtime metrics response: %v", readErr)
	}
	if legacyResponse.StatusCode != http.StatusOK || !strings.Contains(string(legacyBody), `"goroutines"`) {
		t.Errorf("existing runtime metrics endpoint changed: status=%d body=%s",
			legacyResponse.StatusCode, legacyBody)
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
			app := New().Use(monitoring.Config{HTTPMetricsAllowedIPs: []string{"0.0.0.0/0"}})
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
