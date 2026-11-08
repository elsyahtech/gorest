package gorest

import (
	"net/http"
	"strings"

	"github.com/elsyahtech/gorest/hardening"
)

type hardeningApplicationConfig struct {
	AppName                 string
	Version                 string
	Environment             string
	Timezone                string
	Debug                   bool
	AllowInsecureProduction bool
	CheckForUpdates         bool
}

func (app *App) registerHardeningRoutes() {
	if app == nil || app.server == nil || app.hardening == nil || !app.hardening.Enabled() {
		return
	}

	app.Get("/gorest/hardening", func(ctx *Context) error {
		if !app.hardening.AllowsPeer(ctx.RequestCtx().RemoteIP()) {
			return ctx.Status(http.StatusForbidden).JSON(Map{"status": "forbidden"})
		}

		format := strings.ToLower(strings.TrimSpace(ctx.Query("format")))
		acceptsHTML := strings.Contains(strings.ToLower(ctx.Get("Accept")), "text/html")
		if format != "" && format != "json" && format != "html" {
			return ctx.Status(http.StatusBadRequest).JSON(Map{
				"status":  "invalid_format",
				"message": "format must be json or html",
			})
		}

		checkForUpdates := app.config.CheckForUpdates != nil && *app.config.CheckForUpdates
		report := hardening.Scan(app.config.Environment, map[string]any{
			"app": hardeningApplicationConfig{
				AppName:                 app.config.AppName,
				Version:                 app.config.Version,
				Environment:             app.config.Environment,
				Timezone:                app.config.Timezone,
				Debug:                   app.config.Debug,
				AllowInsecureProduction: app.config.AllowInsecureProduction,
				CheckForUpdates:         checkForUpdates,
			},
			"server.Config":     app.config.server,
			"database.Config":   app.config.database,
			"redis.Config":      app.config.redis,
			"filter.Config":     app.config.filter,
			"log.Config":        app.config.log,
			"monitoring.Config": app.config.monitoring,
			"hardening.Config":  app.hardening.Config,
		})

		if format == "html" || (format == "" && acceptsHTML) {
			body, err := hardening.RenderHTML(report)
			if err != nil {
				app.Log(map[string]any{LogFieldKeyError: err}, true).Error("failed to render hardening report")
				return ctx.Status(http.StatusInternalServerError).JSON(Map{"status": "report_error"})
			}

			ctx.Set("Content-Type", "text/html; charset=utf-8")
			ctx.Set("X-Content-Type-Options", "nosniff")
			ctx.Set("Cache-Control", "no-store")
			return ctx.Status(http.StatusOK).Send(body)
		}

		body, err := hardening.RenderJSON(report)
		if err != nil {
			app.Log(map[string]any{LogFieldKeyError: err}, true).Error("failed to encode hardening report")
			return ctx.Status(http.StatusInternalServerError).JSON(Map{"status": "report_error"})
		}

		ctx.Set("Content-Type", "application/json; charset=utf-8")
		ctx.Set("X-Content-Type-Options", "nosniff")
		ctx.Set("Cache-Control", "no-store")
		return ctx.Status(http.StatusOK).Send(body)
	})
}
