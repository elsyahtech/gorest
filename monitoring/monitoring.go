package monitoring

import (
	"fmt"
	"net"

	"github.com/elsyahtech/gorest/server"
	"github.com/gofiber/fiber/v3"
)

// Run applies monitoring configuration defaults.
func Run(config ...Config) Config {
	return configDefault(config...)
}

// New initializes a monitoring service and validates its configuration.
func New(config Config) (*Monitoring, error) {
	config = Run(config)
	httpMetrics, err := NewHTTPMetrics(HTTPMetricsOptions{
		AllowedIPs:     config.HTTPMetricsAllowedIPs,
		EnablePerRoute: config.EnablePerRouteHTTPMetrics,
	})
	if err != nil {
		return nil, fmt.Errorf("initialize monitoring service: %w", err)
	}

	return &Monitoring{
		config:      config,
		httpMetrics: httpMetrics,
	}, nil
}

// EndpointsEnabled reports whether the built-in operational endpoints are enabled.
func (m *Monitoring) EndpointsEnabled() bool {
	return m != nil && !m.config.DisableMonitoringEndpoints
}

// AllowsPeer reports whether the peer IP is allowed to access operational endpoints.
func (m *Monitoring) AllowsPeer(peerIP net.IP) bool {
	return m != nil && m.httpMetrics != nil && m.httpMetrics.AllowsPeer(peerIP)
}

// Middleware observes application HTTP requests when monitoring endpoints are enabled.
func (m *Monitoring) Middleware(ctx fiber.Ctx) error {
	if m == nil || m.httpMetrics == nil {
		return ctx.Next()
	}

	return m.httpMetrics.Middleware(ctx)
}

// WrapErrorHandler wraps the server error handler to record finalized error responses.
func (m *Monitoring) WrapErrorHandler(next fiber.ErrorHandler) fiber.ErrorHandler {
	if m == nil || m.httpMetrics == nil {
		return next
	}

	return m.httpMetrics.WrapErrorHandler(next)
}

// RegisterHTTPMetrics registers the HTTP metrics endpoint on the supplied server group.
func (m *Monitoring) RegisterHTTPMetrics(routes *server.Server) {
	if m == nil {
		return
	}

	RegisterHTTPMetrics(routes, m.httpMetrics)
}
