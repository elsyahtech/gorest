package monitoring

import (
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

// Snapshot returns a point-in-time copy of the cumulative collector.
func (m *HTTPMetrics) Snapshot() HTTPMetricsSnapshot {
	if m == nil {
		unavailable := make(map[string]string)
		unavailable["http_metrics"] = "collector_unavailable"

		return HTTPMetricsSnapshot{
			Scope:              "application_http_server",
			CounterPeriod:      httpMetricsCounterPeriod,
			DurationUnit:       "ms",
			LatencyMethod:      httpMetricsLatencyMethod,
			StatusCodeTotal:    make(map[string]uint64),
			UnavailableMetrics: unavailable,
		}
	}

	snapshot := m.global.snapshot()
	snapshot.Scope = "application_http_server"
	snapshot.CounterPeriod = httpMetricsCounterPeriod
	snapshot.DurationUnit = "ms"
	snapshot.LatencyMethod = httpMetricsLatencyMethod
	inFlight := m.inFlight.Load()
	snapshot.RequestInFlight = &inFlight

	if !m.enablePerRoute {
		return snapshot
	}

	m.routesMu.Lock()

	routes := make([]routeMetricCounters, 0, len(m.routes))
	for _, route := range m.routes {
		routes = append(routes, *route)
	}
	m.routesMu.Unlock()

	slices.SortFunc(routes, func(left, right routeMetricCounters) int {
		if left.method == right.method {
			return strings.Compare(left.route, right.route)
		}

		return strings.Compare(left.method, right.method)
	})

	snapshot.Routes = make([]HTTPRouteMetricsSnapshot, 0, len(routes))
	for _, route := range routes {
		routeSnapshot := route.counters.snapshot()
		routeSnapshot.Scope = "application_http_server_route"
		routeSnapshot.CounterPeriod = httpMetricsCounterPeriod
		routeSnapshot.DurationUnit = "ms"
		routeSnapshot.LatencyMethod = httpMetricsLatencyMethod
		snapshot.Routes = append(snapshot.Routes, HTTPRouteMetricsSnapshot{
			Method:  route.method,
			Route:   route.route,
			Metrics: routeSnapshot,
		})
	}

	return snapshot
}

func (counters *httpMetricCounters) snapshot() HTTPMetricsSnapshot {
	snapshot := HTTPMetricsSnapshot{
		StatusCodeTotal:    make(map[string]uint64),
		UnavailableMetrics: make(map[string]string),
	}

	for status := http.StatusOK; status <= 599; status++ {
		total := counters.statusCodeTotal[status].Load()
		if total == 0 {
			continue
		}

		snapshot.StatusCodeTotal[strconv.Itoa(status)] = total
		snapshot.RequestTotal += total

		switch {
		case status <= 399:
			snapshot.RequestSuccessTotal += total
		case status <= 499:
			snapshot.RequestClientErrorTotal += total
		default:
			snapshot.RequestServerErrorTotal += total
		}
	}

	snapshot.RequestErrorTotal = snapshot.RequestClientErrorTotal + snapshot.RequestServerErrorTotal

	if snapshot.RequestTotal == 0 {
		for _, name := range []string{
			"request_duration_avg", "request_duration_p50", "request_duration_p95",
			"request_duration_p99", "request_duration_max",
		} {
			snapshot.UnavailableMetrics[name] = "no_completed_requests"
		}
	} else {
		if counters.durationOverflow.Load() {
			snapshot.UnavailableMetrics["request_duration_avg"] = "accumulator_overflow"
		} else {
			average := float64(counters.durationTotal.Load()) / float64(snapshot.RequestTotal) / float64(time.Millisecond)
			snapshot.RequestDurationAvg = finiteFloat(average)
		}

		maximum := float64(counters.durationMax.Load()) / float64(time.Millisecond)
		snapshot.RequestDurationMax = finiteFloat(maximum)
		snapshot.RequestDurationP50 = counters.percentile(0.50, snapshot.RequestTotal, snapshot.UnavailableMetrics, "request_duration_p50")
		snapshot.RequestDurationP95 = counters.percentile(0.95, snapshot.RequestTotal, snapshot.UnavailableMetrics, "request_duration_p95")
		snapshot.RequestDurationP99 = counters.percentile(0.99, snapshot.RequestTotal, snapshot.UnavailableMetrics, "request_duration_p99")
	}

	if counters.requestBytesAvailable.Load() {
		total := counters.requestBytesTotal.Load()
		snapshot.RequestBytesTotal = &total
	} else {
		snapshot.UnavailableMetrics["request_bytes_total"] = "incomplete_measurement"
	}

	if counters.responseBytesAvailable.Load() {
		total := counters.responseBytesTotal.Load()
		snapshot.ResponseBytesTotal = &total
	} else {
		snapshot.UnavailableMetrics["response_bytes_total"] = "incomplete_measurement"
	}

	return snapshot
}

func (counters *httpMetricCounters) percentile(
	quantile float64,
	requestTotal uint64,
	unavailable map[string]string,
	name string,
) *float64 {
	rank := uint64(math.Ceil(quantile * float64(requestTotal)))
	if rank == 0 {
		rank = 1
	}

	var cumulative uint64
	for index := range counters.durationBuckets {
		cumulative += counters.durationBuckets[index].Load()
		if cumulative < rank {
			continue
		}

		if index == len(latencyBucketBounds) {
			unavailable[name] = "histogram_overflow"
			return nil
		}

		value := float64(latencyBucketBounds[index]) / float64(time.Millisecond)

		return &value
	}

	unavailable[name] = "histogram_unavailable"

	return nil
}

func finiteFloat(value float64) *float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return nil
	}

	return &value
}
