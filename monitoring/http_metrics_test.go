package monitoring

import (
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/recover"
)

func TestHTTPMetricsClassifyFinalStatusesAndRecoveredErrors(t *testing.T) {
	metrics, err := NewHTTPMetrics(HTTPMetricsOptions{})
	if err != nil {
		t.Fatalf("create HTTP metrics: %v", err)
	}

	app := newHTTPMetricsTestApp(metrics)
	app.Get("/success", func(ctx fiber.Ctx) error {
		return ctx.SendString("ok")
	})
	app.Get("/redirect", func(ctx fiber.Ctx) error {
		return ctx.Redirect().Status(http.StatusFound).To("/success")
	})
	app.Get("/client-error", func(ctx fiber.Ctx) error {
		return ctx.SendStatus(http.StatusNotFound)
	})
	app.Get("/server-error", func(fiber.Ctx) error {
		return fiber.ErrInternalServerError
	})
	app.Get("/panic", func(fiber.Ctx) error {
		panic("recovered panic")
	})
	app.Get("/timeout", func(fiber.Ctx) error {
		return fiber.ErrRequestTimeout
	})
	app.Get("/empty", func(fiber.Ctx) error {
		return nil
	})

	for _, path := range []string{
		"/success", "/redirect", "/client-error", "/server-error", "/panic", "/timeout", "/empty",
	} {
		response := requestHTTPMetricsTest(t, app, http.MethodGet, path, nil)
		response.Body.Close()
	}

	snapshot := metrics.Snapshot()
	if snapshot.RequestTotal != 7 {
		t.Fatalf("request_total = %d, want 7", snapshot.RequestTotal)
	}
	if snapshot.RequestTotal != snapshot.RequestSuccessTotal+snapshot.RequestClientErrorTotal+snapshot.RequestServerErrorTotal {
		t.Fatalf("request category invariant failed: %#v", snapshot)
	}
	if snapshot.RequestSuccessTotal != 3 || snapshot.RequestClientErrorTotal != 2 || snapshot.RequestServerErrorTotal != 2 {
		t.Errorf("request categories = (%d, %d, %d), want (3, 2, 2)",
			snapshot.RequestSuccessTotal, snapshot.RequestClientErrorTotal, snapshot.RequestServerErrorTotal)
	}
	if snapshot.RequestErrorTotal != 4 {
		t.Errorf("request_error_total = %d, want 4", snapshot.RequestErrorTotal)
	}
	if snapshot.StatusCodeTotal["200"] != 2 ||
		snapshot.StatusCodeTotal["302"] != 1 ||
		snapshot.StatusCodeTotal["404"] != 1 ||
		snapshot.StatusCodeTotal["408"] != 1 ||
		snapshot.StatusCodeTotal["500"] != 2 {
		t.Errorf("status_code_total = %#v", snapshot.StatusCodeTotal)
	}
	if snapshot.RequestDurationAvg == nil || snapshot.RequestDurationMax == nil ||
		snapshot.RequestDurationP50 == nil || snapshot.RequestDurationP95 == nil || snapshot.RequestDurationP99 == nil {
		t.Errorf("latency fields are unexpectedly unavailable: %#v", snapshot.UnavailableMetrics)
	}
	if snapshot.RequestInFlight == nil || *snapshot.RequestInFlight != 0 {
		t.Errorf("request_in_flight = %v, want 0", snapshot.RequestInFlight)
	}
}

func TestHTTPMetricsLatencyAverageMaximumAndHistogramBounds(t *testing.T) {
	counters := newHTTPMetricCounters()
	for _, duration := range []time.Duration{
		1500 * time.Microsecond,
		8 * time.Millisecond,
		900 * time.Millisecond,
		10 * time.Second,
	} {
		counters.record(
			http.StatusOK,
			duration,
			bodyByteMeasurement{known: true},
			bodyByteMeasurement{known: true},
		)
	}

	snapshot := counters.snapshot()
	if snapshot.RequestDurationAvg == nil || *snapshot.RequestDurationAvg != 2727.375 {
		t.Errorf("request_duration_avg = %v, want 2727.375 ms", snapshot.RequestDurationAvg)
	}
	if snapshot.RequestDurationMax == nil || *snapshot.RequestDurationMax != 10000 {
		t.Errorf("request_duration_max = %v, want 10000 ms", snapshot.RequestDurationMax)
	}
	if snapshot.RequestDurationP50 == nil || *snapshot.RequestDurationP50 != 10 {
		t.Errorf("request_duration_p50 = %v, want 10 ms bucket upper bound", snapshot.RequestDurationP50)
	}
	if snapshot.RequestDurationP95 == nil || *snapshot.RequestDurationP95 != 10000 {
		t.Errorf("request_duration_p95 = %v, want 10000 ms bucket upper bound", snapshot.RequestDurationP95)
	}

	counters.record(
		http.StatusOK,
		6*time.Minute,
		bodyByteMeasurement{known: true},
		bodyByteMeasurement{known: true},
	)
	snapshot = counters.snapshot()
	if snapshot.RequestDurationP99 != nil || snapshot.UnavailableMetrics["request_duration_p99"] != "histogram_overflow" {
		t.Errorf("overflow percentile = %v, unavailable = %q; want unavailable",
			snapshot.RequestDurationP99, snapshot.UnavailableMetrics["request_duration_p99"])
	}
}

func TestHTTPMetricsBodyBytesAreExactOrPermanentlyUnavailable(t *testing.T) {
	metrics, err := NewHTTPMetrics(HTTPMetricsOptions{})
	if err != nil {
		t.Fatalf("create HTTP metrics: %v", err)
	}

	app := newHTTPMetricsTestApp(metrics)
	app.Post("/body", func(ctx fiber.Ctx) error {
		return ctx.SendStatus(http.StatusNoContent)
	})
	app.Get("/stream", func(ctx fiber.Ctx) error {
		ctx.Response().SetBodyStream(strings.NewReader("streamed"), -1)
		return nil
	})

	req := httptest.NewRequest(http.MethodPost, "/body", strings.NewReader("payload"))
	response := requestHTTPMetricsTestWithRequest(t, app, req)
	response.Body.Close()

	snapshot := metrics.Snapshot()
	if snapshot.RequestBytesTotal == nil || *snapshot.RequestBytesTotal != uint64(len("payload")) {
		t.Errorf("request_bytes_total = %v, want %d", snapshot.RequestBytesTotal, len("payload"))
	}
	if snapshot.ResponseBytesTotal == nil || *snapshot.ResponseBytesTotal != 0 {
		t.Errorf("response_bytes_total = %v, want 0 for empty responses", snapshot.ResponseBytesTotal)
	}

	response = requestHTTPMetricsTest(t, app, http.MethodGet, "/stream", nil)
	response.Body.Close()
	snapshot = metrics.Snapshot()
	if snapshot.ResponseBytesTotal != nil || snapshot.UnavailableMetrics["response_bytes_total"] != "incomplete_measurement" {
		t.Errorf("stream response bytes = %v, unavailable = %q; want cumulative total unavailable",
			snapshot.ResponseBytesTotal, snapshot.UnavailableMetrics["response_bytes_total"])
	}
	if snapshot.RequestTotal != 2 {
		t.Errorf("request_total = %d, want 2", snapshot.RequestTotal)
	}
}

func TestHTTPMetricsCountsNoContentAndHeadResponsesAsZeroBytes(t *testing.T) {
	metrics, err := NewHTTPMetrics(HTTPMetricsOptions{})
	if err != nil {
		t.Fatalf("create HTTP metrics: %v", err)
	}

	app := newHTTPMetricsTestApp(metrics)
	for _, status := range []int{http.StatusNoContent, http.StatusResetContent, http.StatusNotModified} {
		status := status
		app.Get("/status-"+strconv.Itoa(status), func(ctx fiber.Ctx) error {
			ctx.Response().SetBodyString("must not be counted")
			ctx.Status(status)
			return nil
		})
	}
	app.Head("/head", func(ctx fiber.Ctx) error {
		return ctx.SendString("must not be counted")
	})

	for _, path := range []string{
		"/status-204", "/status-205", "/status-304", "/head",
	} {
		method := http.MethodGet
		if path == "/head" {
			method = http.MethodHead
		}
		response := requestHTTPMetricsTest(t, app, method, path, nil)
		response.Body.Close()
	}

	snapshot := metrics.Snapshot()
	if snapshot.RequestTotal != 4 {
		t.Errorf("request_total = %d, want 4", snapshot.RequestTotal)
	}
	if snapshot.ResponseBytesTotal == nil || *snapshot.ResponseBytesTotal != 0 {
		t.Errorf("response_bytes_total = %v, want 0", snapshot.ResponseBytesTotal)
	}
}

func TestHTTPMetricsRequestStreamMakesCumulativeBytesUnavailable(t *testing.T) {
	metrics, err := NewHTTPMetrics(HTTPMetricsOptions{})
	if err != nil {
		t.Fatalf("create HTTP metrics: %v", err)
	}
	app := fiber.New(fiber.Config{
		StreamRequestBody: true,
		ErrorHandler:      metrics.WrapErrorHandler(fiber.DefaultErrorHandler),
	})
	app.Use(recover.New())
	app.Use(metrics.Middleware)
	app.Post("/stream-request", func(ctx fiber.Ctx) error {
		_, err := io.Copy(io.Discard, ctx.Request().BodyStream())
		if err != nil {
			return err
		}
		return ctx.SendStatus(http.StatusNoContent)
	})

	response := requestHTTPMetricsTest(t, app, http.MethodPost, "/stream-request", strings.NewReader("streamed request body"))
	response.Body.Close()

	snapshot := metrics.Snapshot()
	if snapshot.RequestBytesTotal != nil || snapshot.UnavailableMetrics["request_bytes_total"] != "incomplete_measurement" {
		t.Errorf("stream request bytes = %v; unavailable = %q, want cumulative total unavailable",
			snapshot.RequestBytesTotal, snapshot.UnavailableMetrics["request_bytes_total"])
	}
}

func TestHTTPMetricsChunkedRequestAndPerRouteTemplates(t *testing.T) {
	metrics, err := NewHTTPMetrics(HTTPMetricsOptions{EnablePerRoute: true})
	if err != nil {
		t.Fatalf("create HTTP metrics: %v", err)
	}

	app := newHTTPMetricsTestApp(metrics)
	app.Get("/users/:id", func(ctx fiber.Ctx) error {
		return ctx.SendStatus(http.StatusOK)
	})
	app.Post("/users/:id", func(ctx fiber.Ctx) error {
		return ctx.SendStatus(http.StatusCreated)
	})

	req := httptest.NewRequest(http.MethodPost, "/users/one", strings.NewReader("chunk-data"))
	req.ContentLength = 0
	req.Header.Del("Content-Length")
	req.TransferEncoding = []string{"chunked"}
	response := requestHTTPMetricsTestWithRequest(t, app, req)
	response.Body.Close()
	response = requestHTTPMetricsTest(t, app, http.MethodGet, "/users/two", nil)
	response.Body.Close()
	response = requestHTTPMetricsTest(t, app, http.MethodGet, "/not-found/123", nil)
	response.Body.Close()

	snapshot := metrics.Snapshot()
	if len(snapshot.Routes) != 3 {
		t.Fatalf("route metric count = %d, want 3: %#v", len(snapshot.Routes), snapshot.Routes)
	}
	if snapshot.RequestTotal != 3 {
		t.Errorf("request_total = %d, want 3", snapshot.RequestTotal)
	}

	var (
		getUserRouteFound  bool
		postUserRouteFound bool
		unmatchedFound     bool
		exposedRawID       bool
		chunkedRouteByte   uint64
	)
	for _, route := range snapshot.Routes {
		switch {
		case route.Route == "/users/:id" && route.Method == http.MethodGet:
			getUserRouteFound = true
			if route.Metrics.RequestTotal != 1 {
				t.Errorf("GET user route request_total = %d, want 1", route.Metrics.RequestTotal)
			}
		case route.Route == "/users/:id" && route.Method == http.MethodPost:
			postUserRouteFound = true
			chunkedRouteByte = valueOrZero(route.Metrics.RequestBytesTotal)
			if route.Metrics.RequestTotal != 1 {
				t.Errorf("POST user route request_total = %d, want 1", route.Metrics.RequestTotal)
			}
		case route.Route == "unmatched":
			unmatchedFound = true
			if route.Metrics.RequestTotal != 1 {
				t.Errorf("unmatched route request_total = %d, want 1", route.Metrics.RequestTotal)
			}
		case strings.Contains(route.Route, "/users/one") || strings.Contains(route.Route, "/users/two"):
			exposedRawID = true
		}
	}
	if !getUserRouteFound || !postUserRouteFound || !unmatchedFound || exposedRawID {
		t.Errorf("route labels GET=%t POST=%t unmatched=%t rawID=%t",
			getUserRouteFound, postUserRouteFound, unmatchedFound, exposedRawID)
	}
	if chunkedRouteByte != uint64(len("chunk-data")) {
		t.Errorf("chunked request bytes = %d, want %d", chunkedRouteByte, len("chunk-data"))
	}
}

func TestHTTPMetricsConcurrentCountersAndInFlight(t *testing.T) {
	metrics, err := NewHTTPMetrics(HTTPMetricsOptions{})
	if err != nil {
		t.Fatalf("create HTTP metrics: %v", err)
	}

	allStarted := make(chan struct{})
	release := make(chan struct{})
	app := newHTTPMetricsTestApp(metrics)
	const requestCount = 32
	var started atomic.Int64
	app.Get("/wait", func(ctx fiber.Ctx) error {
		if started.Add(1) == requestCount {
			close(allStarted)
		}
		<-release
		return ctx.SendStatus(http.StatusOK)
	})

	var workers sync.WaitGroup
	workers.Add(requestCount)
	for range requestCount {
		go func() {
			defer workers.Done()
			response := requestHTTPMetricsTest(t, app, http.MethodGet, "/wait", nil)
			response.Body.Close()
		}()
	}

	select {
	case <-allStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("not all concurrent requests reached the handler")
	}
	if got := metrics.Snapshot().RequestInFlight; got == nil || *got != requestCount {
		t.Fatalf("request_in_flight before release = %v, want %d", got, requestCount)
	}
	close(release)
	workers.Wait()

	snapshot := metrics.Snapshot()
	if snapshot.RequestTotal != requestCount ||
		snapshot.RequestTotal != snapshot.RequestSuccessTotal+snapshot.RequestClientErrorTotal+snapshot.RequestServerErrorTotal {
		t.Errorf("concurrent counters violate totals: %#v", snapshot)
	}
	if snapshot.RequestInFlight == nil || *snapshot.RequestInFlight != 0 {
		t.Errorf("request_in_flight = %v, want 0", snapshot.RequestInFlight)
	}
}

func TestHTTPMetricsAllowlistUsesPeerIPAndValidatesEntries(t *testing.T) {
	tests := []struct {
		name    string
		entries []string
		ip      net.IP
		want    bool
	}{
		{name: "empty denies", ip: net.ParseIP("127.0.0.1")},
		{name: "exact IPv4", entries: []string{"127.0.0.1"}, ip: net.ParseIP("127.0.0.1"), want: true},
		{name: "IPv4 CIDR", entries: []string{"10.0.0.0/8"}, ip: net.ParseIP("10.2.3.4"), want: true},
		{name: "IPv6 CIDR", entries: []string{"2001:db8::/32"}, ip: net.ParseIP("2001:db8::1"), want: true},
		{name: "outside range", entries: []string{"10.0.0.0/8"}, ip: net.ParseIP("192.0.2.1")},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			metrics, err := NewHTTPMetrics(HTTPMetricsOptions{AllowedIPs: test.entries})
			if err != nil {
				t.Fatalf("create HTTP metrics: %v", err)
			}
			if got := metrics.AllowsPeer(test.ip); got != test.want {
				t.Errorf("AllowsPeer(%s) = %t, want %t", test.ip, got, test.want)
			}
		})
	}

	if _, err := NewHTTPMetrics(HTTPMetricsOptions{AllowedIPs: []string{"not-an-ip"}}); err == nil {
		t.Fatal("invalid allowlist entry unexpectedly succeeded")
	}
	if _, err := NewHTTPMetrics(HTTPMetricsOptions{AllowedIPs: []string{" "}}); err == nil {
		t.Fatal("empty allowlist entry unexpectedly succeeded")
	}
}

func TestHTTPMetricsSnapshotDoesNotResetCounters(t *testing.T) {
	metrics, err := NewHTTPMetrics(HTTPMetricsOptions{})
	if err != nil {
		t.Fatalf("create HTTP metrics: %v", err)
	}

	app := newHTTPMetricsTestApp(metrics)
	app.Get("/ok", func(ctx fiber.Ctx) error {
		return ctx.SendStatus(http.StatusOK)
	})

	for range 2 {
		response := requestHTTPMetricsTest(t, app, http.MethodGet, "/ok", nil)
		response.Body.Close()
	}

	first := metrics.Snapshot()
	second := metrics.Snapshot()
	if first.RequestTotal != 2 || second.RequestTotal != 2 {
		t.Errorf("snapshots reset or changed counters: first=%d second=%d", first.RequestTotal, second.RequestTotal)
	}
}

func TestHTTPMetricsPerRouteDisabledByDefault(t *testing.T) {
	metrics, err := NewHTTPMetrics(HTTPMetricsOptions{})
	if err != nil {
		t.Fatalf("create HTTP metrics: %v", err)
	}

	app := newHTTPMetricsTestApp(metrics)
	app.Get("/ok", func(ctx fiber.Ctx) error {
		return ctx.SendStatus(http.StatusOK)
	})
	response := requestHTTPMetricsTest(t, app, http.MethodGet, "/ok", nil)
	response.Body.Close()

	if routes := metrics.Snapshot().Routes; routes != nil {
		t.Errorf("per-route metrics = %#v, want disabled by default", routes)
	}
}

func newHTTPMetricsTestApp(metrics *HTTPMetrics) *fiber.App {
	app := fiber.New(fiber.Config{
		ErrorHandler: metrics.WrapErrorHandler(fiber.DefaultErrorHandler),
	})
	app.Use(recover.New())
	app.Use(metrics.Middleware)
	return app
}

func requestHTTPMetricsTest(t *testing.T, app *fiber.App, method, path string, body io.Reader) *http.Response {
	t.Helper()
	return requestHTTPMetricsTestWithRequest(t, app, httptest.NewRequest(method, path, body))
}

func requestHTTPMetricsTestWithRequest(t *testing.T, app *fiber.App, request *http.Request) *http.Response {
	t.Helper()
	response, err := app.Test(request, fiber.TestConfig{Timeout: 0})
	if err != nil {
		t.Fatalf("serve %s %s: %v", request.Method, request.URL.Path, err)
	}
	return response
}

func valueOrZero(value *uint64) uint64 {
	if value == nil {
		return 0
	}
	return *value
}

func TestHTTPMetricsCountsHandlerErrorWithFinalStatus(t *testing.T) {
	metrics, err := NewHTTPMetrics(HTTPMetricsOptions{})
	if err != nil {
		t.Fatalf("create HTTP metrics: %v", err)
	}

	app := newHTTPMetricsTestApp(metrics)
	app.Get("/failure", func(fiber.Ctx) error {
		return errors.New("handler failure")
	})

	response := requestHTTPMetricsTest(t, app, http.MethodGet, "/failure", nil)
	response.Body.Close()

	snapshot := metrics.Snapshot()
	if snapshot.RequestTotal != 1 || snapshot.RequestServerErrorTotal != 1 {
		t.Errorf("error response was not counted with its final status: %#v", snapshot)
	}
	if snapshot.RequestInFlight == nil || *snapshot.RequestInFlight != 0 {
		t.Errorf("request_in_flight = %v after error, want 0", snapshot.RequestInFlight)
	}
}

func TestHTTPMetricsCountsFallbackStatusWhenCustomErrorHandlerFails(t *testing.T) {
	metrics, err := NewHTTPMetrics(HTTPMetricsOptions{})
	if err != nil {
		t.Fatalf("create HTTP metrics: %v", err)
	}
	app := fiber.New(fiber.Config{
		ErrorHandler: metrics.WrapErrorHandler(func(fiber.Ctx, error) error {
			return errors.New("custom error handler failed")
		}),
	})
	app.Use(recover.New())
	app.Use(metrics.Middleware)
	app.Get("/failure", func(fiber.Ctx) error {
		return errors.New("handler failed")
	})

	response := requestHTTPMetricsTest(t, app, http.MethodGet, "/failure", nil)
	response.Body.Close()
	snapshot := metrics.Snapshot()
	if snapshot.RequestTotal != 1 || snapshot.RequestServerErrorTotal != 1 || snapshot.StatusCodeTotal["500"] != 1 {
		t.Errorf("fallback 500 response was not counted: %#v", snapshot)
	}
	if snapshot.RequestInFlight == nil || *snapshot.RequestInFlight != 0 {
		t.Errorf("request_in_flight after custom error handler failure = %v, want 0", snapshot.RequestInFlight)
	}
}

func TestHTTPMetricsCountsApplicationResponseWhenClientDisconnects(t *testing.T) {
	metrics, err := NewHTTPMetrics(HTTPMetricsOptions{})
	if err != nil {
		t.Fatalf("create HTTP metrics: %v", err)
	}
	app := newHTTPMetricsTestApp(metrics)
	handlerStarted := make(chan struct{})
	releaseHandler := make(chan struct{})
	app.Get("/disconnect", func(ctx fiber.Ctx) error {
		close(handlerStarted)
		<-releaseHandler
		return ctx.SendString("prepared")
	})

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen for disconnect test: %v", err)
	}
	serverDone := make(chan error, 1)
	go func() {
		serverDone <- app.Listener(listener)
	}()
	shutdown := false
	defer func() {
		if !shutdown {
			if err := app.ShutdownWithTimeout(2 * time.Second); err != nil {
				t.Errorf("shut down disconnect test server: %v", err)
			}
		}
	}()

	connection, err := net.DialTimeout("tcp", listener.Addr().String(), 2*time.Second)
	if err != nil {
		t.Fatalf("connect to disconnect test server: %v", err)
	}
	if _, err := io.WriteString(connection, "GET /disconnect HTTP/1.1\r\nHost: localhost\r\nConnection: close\r\n\r\n"); err != nil {
		_ = connection.Close()
		t.Fatalf("write disconnect test request: %v", err)
	}
	select {
	case <-handlerStarted:
	case <-time.After(5 * time.Second):
		_ = connection.Close()
		t.Fatal("disconnect test request did not reach handler")
	}
	if err := connection.Close(); err != nil {
		t.Fatalf("close client connection: %v", err)
	}
	close(releaseHandler)

	deadline := time.Now().Add(5 * time.Second)
	for metrics.Snapshot().RequestTotal == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	snapshot := metrics.Snapshot()
	if snapshot.RequestTotal != 1 || snapshot.RequestSuccessTotal != 1 {
		t.Errorf("application-completed response after disconnect = total %d success %d, want (1, 1)",
			snapshot.RequestTotal, snapshot.RequestSuccessTotal)
	}
	if snapshot.ResponseBytesTotal == nil || *snapshot.ResponseBytesTotal != uint64(len("prepared")) {
		t.Errorf("application response bytes after disconnect = %v, want %d",
			snapshot.ResponseBytesTotal, len("prepared"))
	}

	if err := app.ShutdownWithTimeout(2 * time.Second); err != nil {
		t.Fatalf("shut down disconnect test server: %v", err)
	}
	shutdown = true
	select {
	case err := <-serverDone:
		if err != nil {
			t.Errorf("HTTP metrics test listener stopped with an error: %v", err)
		}
	default:
	}
}
