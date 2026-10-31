package gorest

import (
	"context"
	"runtime"
	"time"
)

func (app *App) registerOperationalRoutes() {
	if app == nil || app.server == nil {
		return
	}

	app.Get("/health/live", func(c *Context) error {
		return c.Status(200).JSON(Map{"status": "alive"})
	})
	app.Get("/health/ready", func(c *Context) error {
		ctx, cancel := context.WithTimeout(c.Context(), 3*time.Second)
		defer cancel()
		if app.database == nil || app.config.database == nil {
			return c.Status(503).JSON(Map{"status": "not_ready", "database": "not_initialized"})
		}
		message, err := app.Database().Ping(ctx)
		if err != nil {
			return c.Status(503).JSON(Map{"status": "not_ready", "database": message})
		}

		if app.config.redis != nil && !app.config.redis.Disable {
			if app.redis == nil {
				return c.Status(503).JSON(Map{"status": "not_ready", "database": "healthy", "redis": "not_initialized"})
			}
			if err := app.Redis().Ping(ctx); err != nil {
				return c.Status(503).JSON(Map{"status": "not_ready", "database": "healthy", "redis": "unhealthy"})
			}
		}

		if app.config.redis == nil || app.config.redis.Disable {
			return c.JSON(Map{"status": "ready", "database": "healthy", "redis": "disabled"})
		}
		return c.JSON(Map{"status": "ready", "database": "healthy", "redis": "healthy"})
	})
	app.Get("/metrics", func(c *Context) error {
		var memory runtime.MemStats
		runtime.ReadMemStats(&memory)
		return c.JSON(Map{
			"service":          app.config.AppName,
			"goroutines":       runtime.NumGoroutine(),
			"heap_alloc_bytes": memory.HeapAlloc,
			"heap_sys_bytes":   memory.HeapSys,
			"gc_cycles":        memory.NumGC,
		})
	})
}
