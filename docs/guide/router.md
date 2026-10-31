## Server & Router

Route registration methods accept one handler followed by optional middleware
and handlers. They run in the order supplied; middleware should call
`ctx.Next()` to continue to the next handler. Configure the JWT filter before
registering protected routes, and register routes before `app.Start()`.
If route registration reports that the server or router is not running, ensure
the server configuration is registered with `app.Use(server.Run(...))` before
registering those routes.
`app.RegisterServices(...)` runs each route registrar synchronously, so GOREST
can detect missing JWT configuration before opening the HTTP listener. Calling
`app.Filter()` when the filter is not running aborts startup; see the
[filter configuration guide](./filter.md#required-startup-validation).

```go
key := os.Getenv("GOREST_JWT_SIGNATURE_KEY")
filterConfig := filter.ConfigDefault
filterConfig.SignatureKey = &key
app.Use(filterConfig) // aborts startup if the key or other filter config is invalid

jwtFilter := app.Filter()
group := app.Group("/orm-queries")
group.Get("/users/select-all", jwtFilter.Authentication(), hndlr.GetUsersWithAllColumns)
```

Use the group middleware argument to apply authentication to every route in a
group. A route can also receive middleware and its handler directly through
`app.Get(...)`, `group.Get(...)`, or the corresponding method for another HTTP
verb. The filter validates the token and stores its details in request locals;
application handlers remain responsible for account and resource-level
authorization. Invalid or missing tokens stop the chain before the protected
handler runs. A valid refresh token can renew the access cookie and continue
the current request, as described in the [filter guide](./filter.md).

```go
// Clean, Express-inspired routing syntax
func UserRoutes(app *server.Server) {
	app.Get("/users", func(ctx *server.Context) error {
		return ctx.JSON(gorest.Map{
			"message": "Successfully get all users!",
			"data": []string{"Elman", "Alif", "Syafa", "Azka"},
		})
	})

	app.Get("/users/:id", func(ctx *server.Context) error {
		id := ctx.Params("id")

		return ctx.JSON(gorest.Map{
			"transaction_id": id,
			"name": "Detail User " + id,
		})
	})
}

func ProductRoutes(app *server.Server) {
	productGroup := app.Group("/product")

	productGroup.Get("/", func(ctx *server.Context) error {
		return ctx.JSON(gorest.Map{
			"message": "Successfully get pruduct data!",
			"data":    []string{"Laptop", "Mouse", "Keyboard"},
		})
	})

	productGroup.Get("/:id", func(ctx *server.Context) error {
		id := ctx.Params("id")

		return ctx.JSON(gorest.Map{
			"product_id": id,
			"name":    "Detail product " + id,
		})
	})

	productGroup.Post("/", func(c *server.Context) error {
		return c.Status(http.StatusCreated).JSON(fiber.Map{
			"message": "Add product success!",
		})
	})
}
```