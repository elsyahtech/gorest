package monitoring

import (
	"net/http"
	"sort"
	"sync/atomic"
	"time"
)

var latencyBucketBounds = [...]time.Duration{
	time.Millisecond,
	2 * time.Millisecond,
	5 * time.Millisecond,
	10 * time.Millisecond,
	20 * time.Millisecond,
	50 * time.Millisecond,
	100 * time.Millisecond,
	250 * time.Millisecond,
	500 * time.Millisecond,
	time.Second,
	2 * time.Second,
	5 * time.Second,
	10 * time.Second,
	30 * time.Second,
	time.Minute,
	5 * time.Minute,
}

type bodyByteMeasurement struct {
	value uint64
	known bool
}

type httpMetricCounters struct {
	statusCodeTotal        [600]atomic.Uint64
	durationOverflow       atomic.Bool
	requestBytesAvailable  atomic.Bool
	responseBytesAvailable atomic.Bool
	durationTotal          atomic.Uint64
	durationMax            atomic.Uint64
	durationBuckets        [len(latencyBucketBounds) + 1]atomic.Uint64
	requestBytesTotal      atomic.Uint64
	responseBytesTotal     atomic.Uint64
}

type routeMetricCounters struct {
	counters *httpMetricCounters
	method   string
	route    string
}

func newHTTPMetricCounters() *httpMetricCounters {
	counters := &httpMetricCounters{}
	counters.requestBytesAvailable.Store(true)
	counters.responseBytesAvailable.Store(true)

	return counters
}

func (counters *httpMetricCounters) record(
	statusCode int,
	duration time.Duration,
	requestBytes bodyByteMeasurement,
	responseBytes bodyByteMeasurement,
) {
	addSaturating(&counters.durationTotal, uint64(duration), &counters.durationOverflow)
	updateMax(&counters.durationMax, uint64(duration))

	bucketIndex := sort.Search(len(latencyBucketBounds), func(index int) bool {
		return duration <= latencyBucketBounds[index]
	})
	counters.durationBuckets[bucketIndex].Add(1)

	recordByteMeasurement(&counters.requestBytesTotal, &counters.requestBytesAvailable, requestBytes)
	recordByteMeasurement(&counters.responseBytesTotal, &counters.responseBytesAvailable, responseBytes)

	counters.statusCodeTotal[statusCode].Add(1)
}

func recordByteMeasurement(total *atomic.Uint64, available *atomic.Bool, measurement bodyByteMeasurement) {
	if !measurement.known {
		available.Store(false)
		return
	}

	if !available.Load() {
		return
	}

	if !addSaturating(total, measurement.value, nil) {
		available.Store(false)
	}
}

func addSaturating(total *atomic.Uint64, increment uint64, overflow *atomic.Bool) bool {
	for {
		current := total.Load()
		if ^uint64(0)-current < increment {
			if overflow != nil {
				overflow.Store(true)
			}

			return false
		}

		if total.CompareAndSwap(current, current+increment) {
			return true
		}
	}
}

func updateMax(maximum *atomic.Uint64, value uint64) {
	for current := maximum.Load(); value > current; current = maximum.Load() {
		if maximum.CompareAndSwap(current, value) {
			return
		}
	}
}

func isFinalHTTPStatus(statusCode int) bool {
	return statusCode >= http.StatusOK && statusCode <= 599
}
