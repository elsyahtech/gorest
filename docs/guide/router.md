## Server & Router

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