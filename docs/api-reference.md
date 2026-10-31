# API reference

Go's package documentation is the reference for exported GOREST types,
functions, methods, parameters, and return values. Package documentation is
generated from the source for each published module version.

Browse the packages on [pkg.go.dev](https://pkg.go.dev/github.com/elsyahtech/gorest):

| Package | Main API area |
|---|---|
| [`gorest`](https://pkg.go.dev/github.com/elsyahtech/gorest) | `App`, application configuration, lifecycle, route registration, and application sessions |
| [`database`](https://pkg.go.dev/github.com/elsyahtech/gorest/database) | Database drivers, connection configuration, native APIs, and migrations |
| [`orm`](https://pkg.go.dev/github.com/elsyahtech/gorest/orm) | ORM query construction and execution |
| [`server`](https://pkg.go.dev/github.com/elsyahtech/gorest/server) | HTTP server, routes, handlers, and request context |
| [`redis`](https://pkg.go.dev/github.com/elsyahtech/gorest/redis) | Redis configuration and client |
| [`filter`](https://pkg.go.dev/github.com/elsyahtech/gorest/filter) | JWT configuration, token details, and authentication |
| [`log`](https://pkg.go.dev/github.com/elsyahtech/gorest/log) | Logger configuration and structured logger |
| [`helper`](https://pkg.go.dev/github.com/elsyahtech/gorest/helper) | Request parsing, conversion, and hashing helpers |
| [`view`](https://pkg.go.dev/github.com/elsyahtech/gorest/view) | JSON response envelope |

To inspect the API for the version currently checked out locally, run:

```sh
go doc .
go doc ./database
go doc ./server
```

For examples and explanations, use the relevant [guides](README.md) rather
than relying on a symbol signature alone.
