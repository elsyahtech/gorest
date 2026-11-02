package monitoring

type Config struct {
	// HTTPMetricsAllowedIPs lists peer IP addresses or CIDR ranges allowed to
	// access built-in /gorest endpoints. An empty list denies access. The
	// connection peer is checked directly; forwarded headers are not used.
	//nolint:tagliatelle // Keep snake_case JSON config keys consistent.
	HTTPMetricsAllowedIPs []string `json:"http_metrics_allowed_ips"`

	// DisableMonitoringEndpoints controls whether monitoring endpoints are registered.
	// When true, built-in endpoints such as /gorest/live, /gorest/ready,
	// /gorest/metrics, /gorest/storage, /gorest/database, and /gorest/http
	// are not exposed. Monitoring endpoints are enabled by default.
	DisableMonitoringEndpoints bool `json:"disable_monitoring_endpoints"`

	// EnablePerRouteHTTPMetrics adds counters grouped by HTTP method and
	// registered route template (for example, GET /users/:id) to GET
	// /gorest/http. When false, global HTTP metrics remain available but the
	// routes breakdown is omitted. Enabling it adds per-request processing
	// and memory proportional to the number of registered routes. Defaults to false.
	//nolint:tagliatelle // Keep snake_case JSON config keys consistent.
	EnablePerRouteHTTPMetrics bool `json:"enable_per_route_http_metrics"`
}

var ConfigDefault = Config{
	HTTPMetricsAllowedIPs:      nil,
	DisableMonitoringEndpoints: false,
	EnablePerRouteHTTPMetrics:  false,
}

func configDefault(config ...Config) Config {
	if len(config) < 1 {
		return ConfigDefault
	}

	return config[0]
}
