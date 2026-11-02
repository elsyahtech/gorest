package monitoring

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/elsyahtech/gorest/server"
	"github.com/gofiber/fiber/v3"
)

const httpMetricsObservationKey = "gorest.http.metrics.observation"

const (
	httpMetricsStatusAvailable = "available"
	httpMetricsStatusPartial   = "partial"
	httpMetricsCounterPeriod   = "since_application_start"
	httpMetricsLatencyMethod   = "fixed_histogram_bucket_upper_bound"
)

// HTTPMetricsOptions configures the application-scoped HTTP metrics collector.
type HTTPMetricsOptions struct {
	AllowedIPs     []string
	EnablePerRoute bool
}

// HTTPMetrics collects bounded application-level HTTP observations.
//
//nolint:govet // Keep lifecycle state grouped with its collector-owned counters.
type HTTPMetrics struct {
	allowedIPs     []*net.IPNet
	global         *httpMetricCounters
	routes         map[string]*routeMetricCounters
	inFlight       atomic.Int64
	routesMu       sync.Mutex
	enablePerRoute bool
}

// HTTPMetricsSnapshot reports global counters collected by one application
// instance. Duration values are expressed in milliseconds.
//
//nolint:govet,tagliatelle // Keep snake_case metric names and stable grouped output.
type HTTPMetricsSnapshot struct {
	Scope                   string                     `json:"scope"`
	CounterPeriod           string                     `json:"counter_period"`
	DurationUnit            string                     `json:"duration_unit"`
	LatencyMethod           string                     `json:"latency_method"`
	RequestTotal            uint64                     `json:"request_total"`
	RequestSuccessTotal     uint64                     `json:"request_success_total"`
	RequestClientErrorTotal uint64                     `json:"request_client_error_total"`
	RequestServerErrorTotal uint64                     `json:"request_server_error_total"`
	RequestErrorTotal       uint64                     `json:"request_error_total"`
	RequestDurationAvg      *float64                   `json:"request_duration_avg"`
	RequestDurationP50      *float64                   `json:"request_duration_p50"`
	RequestDurationP95      *float64                   `json:"request_duration_p95"`
	RequestDurationP99      *float64                   `json:"request_duration_p99"`
	RequestDurationMax      *float64                   `json:"request_duration_max"`
	RequestInFlight         *int64                     `json:"request_in_flight,omitempty"`
	RequestBytesTotal       *uint64                    `json:"request_bytes_total"`
	ResponseBytesTotal      *uint64                    `json:"response_bytes_total"`
	StatusCodeTotal         map[string]uint64          `json:"status_code_total"`
	UnavailableMetrics      map[string]string          `json:"unavailable_metrics"`
	Routes                  []HTTPRouteMetricsSnapshot `json:"routes,omitempty"`
}

// HTTPRouteMetricsSnapshot reports metrics grouped by method and registered
// route template.
type HTTPRouteMetricsSnapshot struct {
	Method  string              `json:"method"`
	Route   string              `json:"route"`
	Metrics HTTPMetricsSnapshot `json:"metrics"`
}

// NewHTTPMetrics creates one collector for the lifetime of an application
// instance and validates its peer-IP allowlist.
func NewHTTPMetrics(options HTTPMetricsOptions) (*HTTPMetrics, error) {
	allowedIPs, err := parseIPAllowlist(options.AllowedIPs)
	if err != nil {
		return nil, fmt.Errorf("configure HTTP metrics allowed IPs: %w", err)
	}

	return &HTTPMetrics{
		enablePerRoute: options.EnablePerRoute,
		allowedIPs:     allowedIPs,
		global:         newHTTPMetricCounters(),
		routes:         make(map[string]*routeMetricCounters),
	}, nil
}

func parseIPAllowlist(entries []string) ([]*net.IPNet, error) {
	allowedIPs := make([]*net.IPNet, 0, len(entries))
	for _, entry := range entries {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			return nil, errors.New("allowed IP entry must not be empty")
		}

		if ip := net.ParseIP(entry); ip != nil {
			if ipv4 := ip.To4(); ipv4 != nil {
				allowedIPs = append(allowedIPs, &net.IPNet{IP: ipv4, Mask: net.CIDRMask(32, 32)})
			} else {
				ipv6 := ip.To16()
				allowedIPs = append(allowedIPs, &net.IPNet{IP: ipv6, Mask: net.CIDRMask(128, 128)})
			}

			continue
		}

		_, ipNet, err := net.ParseCIDR(entry)
		if err != nil {
			return nil, fmt.Errorf("invalid IP address or CIDR %q: %w", entry, err)
		}

		allowedIPs = append(allowedIPs, ipNet)
	}

	return allowedIPs, nil
}

// AllowsPeer reports whether the connection peer is present in the allowlist.
func (m *HTTPMetrics) AllowsPeer(peerIP net.IP) bool {
	if m == nil || peerIP == nil || len(m.allowedIPs) == 0 {
		return false
	}

	for _, allowed := range m.allowedIPs {
		if allowed.Contains(peerIP) {
			return true
		}
	}

	return false
}

// RegisterHTTPMetrics adds the IP-restricted HTTP metrics endpoint to a route group.
func RegisterHTTPMetrics(routes *server.Server, metrics *HTTPMetrics) {
	if routes == nil {
		return
	}

	routes.Get("/http", func(ctx *server.Context) error {
		if metrics == nil || !metrics.AllowsPeer(ctx.RequestCtx().RemoteIP()) {
			return ctx.Status(http.StatusForbidden).JSON(fiber.Map{"status": "forbidden"})
		}

		snapshot := metrics.Snapshot()

		status := httpMetricsStatusAvailable
		if len(snapshot.UnavailableMetrics) > 0 {
			status = httpMetricsStatusPartial
		}

		for _, route := range snapshot.Routes {
			if len(route.Metrics.UnavailableMetrics) > 0 {
				status = httpMetricsStatusPartial
				break
			}
		}

		return ctx.JSON(fiber.Map{
			"status":  status,
			"metrics": snapshot,
		})
	})
}
