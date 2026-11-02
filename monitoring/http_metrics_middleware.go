package monitoring

import (
	"net/http"
	"sync"
	"time"

	"github.com/gofiber/fiber/v3"
)

type httpMetricsObservation struct {
	startedAt         time.Time
	requestBytes      uint64
	requestBytesKnown bool
	pendingError      bool
	finished          sync.Once
}

// Middleware observes application request handling without reading or
// modifying request or response bodies.
func (m *HTTPMetrics) Middleware(ctx fiber.Ctx) (err error) {
	if m == nil {
		//nolint:wrapcheck // Preserve Fiber's configured error-handler behavior.
		return ctx.Next()
	}

	observation := &httpMetricsObservation{
		startedAt:         time.Now(),
		requestBytesKnown: !ctx.Request().IsBodyStream(),
	}
	if observation.requestBytesKnown {
		observation.requestBytes = uint64(len(ctx.Request().Body()))
	}

	m.inFlight.Add(1)
	ctx.Locals(httpMetricsObservationKey, observation)

	defer func() {
		if recovered := recover(); recovered != nil {
			observation.pendingError = true

			panic(recovered)
		}

		if !observation.pendingError {
			m.finishInFlight(observation)
		}
	}()

	if err = ctx.Next(); err != nil {
		observation.pendingError = true
		//nolint:wrapcheck // Preserve Fiber's configured error-handler behavior.
		return err
	}

	m.completeObservation(ctx, observation)

	return nil
}

// WrapErrorHandler records responses after the configured Fiber error handler
// has produced its final application-level status and body.
func (m *HTTPMetrics) WrapErrorHandler(next fiber.ErrorHandler) fiber.ErrorHandler {
	return func(ctx fiber.Ctx, err error) (handlerErr error) {
		observation, ok := ctx.Locals(httpMetricsObservationKey).(*httpMetricsObservation)
		if !ok || observation == nil || !observation.pendingError {
			return next(ctx, err)
		}

		defer func() {
			if recovered := recover(); recovered != nil {
				m.finishInFlight(observation)

				panic(recovered)
			}

			if handlerErr != nil {
				// Fiber sends a 500 when a custom error handler itself fails.
				// Do that here so the finalized status is observable before returning.
				handlerErr = ctx.SendStatus(http.StatusInternalServerError)
			}

			if handlerErr == nil {
				m.completeObservation(ctx, observation)
			}

			m.finishInFlight(observation)
		}()

		return next(ctx, err)
	}
}

func (m *HTTPMetrics) completeObservation(ctx fiber.Ctx, observation *httpMetricsObservation) {
	if observation == nil {
		return
	}

	statusCode := ctx.Response().StatusCode()
	if !isFinalHTTPStatus(statusCode) {
		return
	}

	duration := time.Since(observation.startedAt)
	responseBytesKnown := true
	responseBytes := uint64(0)

	if !responseHasNoBody(ctx, statusCode) {
		if ctx.Response().IsBodyStream() {
			responseBytesKnown = false
		} else {
			responseBytes = uint64(len(ctx.Response().Body()))
		}
	}

	requestBytes := bodyByteMeasurement{
		value: observation.requestBytes,
		known: observation.requestBytesKnown,
	}
	responseBytesMeasurement := bodyByteMeasurement{
		value: responseBytes,
		known: responseBytesKnown,
	}

	m.global.record(statusCode, duration, requestBytes, responseBytesMeasurement)

	if !m.enablePerRoute {
		return
	}

	method, route := routeTemplate(ctx)
	counters := m.routeCounters(method, route)
	counters.record(statusCode, duration, requestBytes, responseBytesMeasurement)
}

func responseHasNoBody(ctx fiber.Ctx, statusCode int) bool {
	switch statusCode {
	case http.StatusNoContent, http.StatusResetContent, http.StatusNotModified:
		return true
	}

	return ctx.Method() == http.MethodHead || ctx.Response().SkipBody
}

func routeTemplate(ctx fiber.Ctx) (method, routePath string) {
	method = ctx.Method()
	if method == "" {
		method = "UNKNOWN"
	}

	route := ctx.Route()
	if route == nil || route.Path == "" || route.Path == "*" || route.Path == "/*" ||
		(route.Path == "/" && ctx.Path() != "/" &&
			(ctx.Response().StatusCode() == http.StatusNotFound || ctx.Response().StatusCode() == http.StatusMethodNotAllowed)) {
		return method, "unmatched"
	}

	return method, route.Path
}

func (m *HTTPMetrics) routeCounters(method, route string) *httpMetricCounters {
	key := method + "\x00" + route

	m.routesMu.Lock()
	defer m.routesMu.Unlock()

	entry := m.routes[key]
	if entry == nil {
		entry = &routeMetricCounters{
			method:   method,
			route:    route,
			counters: newHTTPMetricCounters(),
		}
		m.routes[key] = entry
	}

	return entry.counters
}

func (m *HTTPMetrics) finishInFlight(observation *httpMetricsObservation) {
	if observation == nil {
		return
	}

	observation.finished.Do(func() {
		m.inFlight.Add(-1)
	})
}
