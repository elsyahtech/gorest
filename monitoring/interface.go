package monitoring

// Monitoring owns the operational monitoring configuration and HTTP metrics collector.
type Monitoring struct {
	config      Config
	httpMetrics *HTTPMetrics
}
