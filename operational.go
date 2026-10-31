package gorest

import (
	"context"
	"runtime"
	"strconv"
	"strings"
	"time"
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
	if app == nil || app.server == nil || app.config.DisableMonitoringEndpoints {
		return
	}

	startedAt := time.Now()
	operational := app.Group("/gorest")
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
		var memory runtime.MemStats
		runtime.ReadMemStats(&memory)
		return c.JSON(Map{
			"service":          app.config.AppName,
			"goroutines":       runtime.NumGoroutine(),
			"heap_alloc_bytes": memory.HeapAlloc,
			"heap_sys_bytes":   memory.HeapSys,
			"gc_cycles":        memory.NumGC,
		})
	}
	operational.Get("/metrics", metricsHandler)
}
