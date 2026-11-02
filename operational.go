package gorest

import (
	"context"
	"math"
	"net/http"
	"runtime"
	runtimemetrics "runtime/metrics"
	"strconv"
	"strings"
	"time"

	"github.com/elsyahtech/gorest/monitoring"
)

func formatUptime(uptime time.Duration) string {
	units := []struct {
		duration time.Duration
		name     string
	}{
		{24 * time.Hour, "day"},
		{time.Hour, "hour"},
		{time.Minute, "minute"},
		{time.Second, "second"},
	}

	var parts []string
	for _, unit := range units {
		count := uptime / unit.duration
		if count == 0 {
			continue
		}

		name := unit.name
		if count != 1 {
			name += "s"
		}
		parts = append(parts, strconv.FormatInt(int64(count), 10)+" "+name)
		uptime %= unit.duration
	}

	if len(parts) == 0 {
		return "less than a second"
	}
	return strings.Join(parts, ", ")
}

func (app *App) registerOperationalRoutes() {
	if app == nil || app.server == nil || app.monitoring == nil || !app.monitoring.EndpointsEnabled() {
		return
	}

	startedAt := time.Now()
	operational := app.Group("/gorest", func(ctx *Context) error {
		if !app.monitoring.AllowsPeer(ctx.RequestCtx().RemoteIP()) {
			return ctx.Status(http.StatusForbidden).JSON(Map{"status": "forbidden"})
		}

		return ctx.Next()
	})
	monitoring.Register(operational, app.GetDBConfig(), app.database)
	app.monitoring.RegisterHTTPMetrics(operational)
	liveHandler := func(c *Context) error {
		return c.Status(200).JSON(Map{"status": "alive"})
	}
	operational.Get("/live", liveHandler)

	readyHandler := func(c *Context) error {
		ctx, cancel := context.WithTimeout(c.Context(), 3*time.Second)
		defer cancel()

		ready := true
		response := Map{
			"uptime": formatUptime(time.Since(startedAt)),
		}

		switch {
		case app.log == nil || app.config.log == nil:
			response["logger"] = "not running"
			ready = false
		case app.log.Log(nil).Sync() != nil:
			response["logger"] = "unhealthy"
			ready = false
		default:
			response["logger"] = "healthy"
		}

		switch {
		case app.server == nil || app.config.server == nil || app.server.SRV == nil:
			response["server"] = "not running"
			ready = false
		default:
			response["server"] = "healthy"
		}

		switch {
		case app.database == nil || app.config.database == nil || app.config.database.Disable:
			response["database"] = "not running"
			ready = false
		default:
			if _, err := app.Database().Ping(ctx); err != nil {
				response["database"] = "unhealthy"
				ready = false
			} else {
				response["database"] = "healthy"
			}
		}

		switch {
		case app.config.redis == nil || app.config.redis.Disable:
			response["redis"] = "not running"
		case app.redis == nil:
			response["redis"] = "not running"
			ready = false
		case app.Redis().Ping(ctx) != nil:
			response["redis"] = "unhealthy"
			ready = false
		default:
			response["redis"] = "healthy"
		}

		if app.filter == nil {
			response["filter"] = "not running"
		} else if app.filter.Config == nil || app.filter.JWT == nil || app.filter.JWT.Method == nil {
			response["filter"] = "unhealthy"
			ready = false
		} else {
			response["filter"] = "healthy"
		}

		if !ready {
			response["status"] = "not_ready"
			return c.Status(503).JSON(response)
		}

		response["status"] = "ready"
		return c.JSON(response)
	}
	operational.Get("/ready", readyHandler)

	metricsHandler := func(c *Context) error {
		return c.JSON(collectRuntimeMetrics(app.config.AppName))
	}
	operational.Get("/metrics", metricsHandler)
}

type runtimeMetricsResponse struct {
	Service   string           `json:"service"`
	Runtime   runtimeGauges    `json:"runtime"`
	Memory    runtimeMemory    `json:"memory"`
	GC        runtimeGC        `json:"gc"`
	Scheduler runtimeScheduler `json:"scheduler"`
}

type runtimeGauges struct {
	Goroutines int `json:"goroutines"`
	GOMAXPROCS int `json:"gomaxprocs"`
}

type runtimeMemory struct {
	HeapAllocBytes    uint64 `json:"heap_alloc_bytes"`
	HeapSysBytes      uint64 `json:"heap_sys_bytes"`
	HeapInuseBytes    uint64 `json:"heap_inuse_bytes"`
	HeapIdleBytes     uint64 `json:"heap_idle_bytes"`
	HeapReleasedBytes uint64 `json:"heap_released_bytes"`
	HeapLiveBytes     uint64 `json:"heap_live_bytes"`
	HeapObjects       uint64 `json:"heap_objects"`
	StackInuseBytes   uint64 `json:"stack_inuse_bytes"`
	StackSysBytes     uint64 `json:"stack_sys_bytes"`
	TotalAllocBytes   uint64 `json:"total_alloc_bytes"`
	TotalSysBytes     uint64 `json:"total_sys_bytes"`
	NextGCBytes       uint64 `json:"next_gc_bytes"`
	MSpanInuseBytes   uint64 `json:"mspan_inuse_bytes"`
	MCacheInuseBytes  uint64 `json:"mcache_inuse_bytes"`
}

type runtimeGC struct {
	Cycles         uint32  `json:"gc_cycles"`
	ForcedCycles   uint32  `json:"gc_forced_cycles"`
	CPUFraction    float64 `json:"gc_cpu_fraction"`
	PauseTotalNs   uint64  `json:"gc_pause_total_ns"`
	LastTimeUnixNs uint64  `json:"gc_last_time_unix_ns"`
	HeapGoalBytes  uint64  `json:"gc_heap_goal_bytes"`
	HeapLiveBytes  uint64  `json:"gc_heap_live_bytes"`
}

type runtimeScheduler struct {
	LatencyDistribution schedulerLatencyDistribution `json:"scheduler_latency_distribution"`
	MutexWaitTotal      float64                      `json:"mutex_wait_total_seconds"`
}

type schedulerLatencyDistribution struct {
	BucketBoundsSeconds []string `json:"bucket_bounds_seconds"`
	Counts              []uint64 `json:"counts"`
}

func collectRuntimeMetrics(service string) runtimeMetricsResponse {
	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)

	samples := []runtimemetrics.Sample{
		{Name: "/gc/heap/goal:bytes"},
		{Name: "/gc/heap/live:bytes"},
		{Name: "/sched/latencies:seconds"},
		{Name: "/sync/mutex/wait/total:seconds"},
	}
	runtimemetrics.Read(samples)

	var schedulerLatency schedulerLatencyDistribution
	if histogram := samples[2].Value.Float64Histogram(); histogram != nil {
		schedulerLatency.BucketBoundsSeconds = make([]string, len(histogram.Buckets))
		for index, bound := range histogram.Buckets {
			switch {
			case math.IsInf(bound, -1):
				schedulerLatency.BucketBoundsSeconds[index] = "-Inf"
			case math.IsInf(bound, 1):
				schedulerLatency.BucketBoundsSeconds[index] = "+Inf"
			default:
				schedulerLatency.BucketBoundsSeconds[index] = strconv.FormatFloat(bound, 'g', -1, 64)
			}
		}
		schedulerLatency.Counts = append([]uint64{}, histogram.Counts...)
	}

	return runtimeMetricsResponse{
		Service: service,
		Runtime: runtimeGauges{
			Goroutines: runtime.NumGoroutine(),
			GOMAXPROCS: runtime.GOMAXPROCS(0),
		},
		Memory: runtimeMemory{
			HeapAllocBytes:    memory.HeapAlloc,
			HeapSysBytes:      memory.HeapSys,
			HeapInuseBytes:    memory.HeapInuse,
			HeapIdleBytes:     memory.HeapIdle,
			HeapReleasedBytes: memory.HeapReleased,
			HeapLiveBytes:     samples[1].Value.Uint64(),
			HeapObjects:       memory.HeapObjects,
			StackInuseBytes:   memory.StackInuse,
			StackSysBytes:     memory.StackSys,
			TotalAllocBytes:   memory.TotalAlloc,
			TotalSysBytes:     memory.Sys,
			NextGCBytes:       memory.NextGC,
			MSpanInuseBytes:   memory.MSpanInuse,
			MCacheInuseBytes:  memory.MCacheInuse,
		},
		GC: runtimeGC{
			Cycles:         memory.NumGC,
			ForcedCycles:   memory.NumForcedGC,
			CPUFraction:    memory.GCCPUFraction,
			PauseTotalNs:   memory.PauseTotalNs,
			LastTimeUnixNs: memory.LastGC,
			HeapGoalBytes:  samples[0].Value.Uint64(),
			HeapLiveBytes:  samples[1].Value.Uint64(),
		},
		Scheduler: runtimeScheduler{
			LatencyDistribution: schedulerLatency,
			MutexWaitTotal:      samples[3].Value.Float64(),
		},
	}
}
