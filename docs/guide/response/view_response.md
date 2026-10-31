# Response Guide

GOREST route handlers use the underlying Fiber context for HTTP responses.
Return Fiber's response error from the handler so failures are not discarded.
For example:

```go
func GetUsers(ctx *server.Context) error {
	return ctx.Status(http.StatusOK).JSON(gorest.Map{
		"data": users,
	})
}
```

The `server.Context` wraps Fiber's context, so handlers can set an HTTP status
and serialize JSON directly. See the [server and router guide](../router.md)
for route registration and middleware.

## Optional JSON response envelope

The `view` package provides a consistent JSON shape with `data`, `success`,
`httpCode`, and optional `message` or `error` fields. `NewResponse` accepts
optional additional top-level fields; set `Data` to the response payload and
use `WithMessage` to mark success or failure.

```go
func GetUsers(ctx *server.Context) error {
	response := (&view.JSON{}).NewResponse(nil)
	response.Data = users
	response.WithMessage("Users loaded", nil)
	return (&view.JSON{}).SendJSON(ctx, response, nil)
}
```

On success, `WithMessage` sets HTTP 200 and `success: true`. On failure, it
sets `success: false` and defaults to HTTP 400; an error implementing
`HTTPStatusCode() int` supplies a different status, or an explicit status
argument can override it. The HTTP status is sent with the response and is
also included in the JSON body's `httpCode` field. `SendJSON` sends the
prepared response; it does not derive response fields from its `err` argument.

Do not expose raw internal errors to clients. Map errors to deliberate,
sanitized messages and appropriate status codes before constructing a public
response. The response helper does not replace application-level validation,
authorization, or error policy.

## HTML and templates

GOREST does not install a template engine by default. Handlers can use Fiber
response methods directly; configure a Fiber-compatible view engine in the
server's Fiber configuration if the application needs server-side rendering.