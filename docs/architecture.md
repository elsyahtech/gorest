# GOREST architecture

GOREST is an application framework that coordinates HTTP serving and optional
infrastructure while leaving handlers and business rules in ordinary Go code.
It is intended for Go developers building web applications, REST APIs, and
backend services who want common lifecycle and data-access tools without
giving up control of the underlying drivers or Go ecosystem.

## Components

```text
Application code
  ├── Routes and handlers ── server (Fiber)
  ├── ORM and native queries ── database drivers
  ├── Redis operations ── Redis client
  ├── JWT filter
  ├── Logger
  └── Helpers and response types
             │
             └── gorest.App coordinates configuration and lifecycle
```

| Package | Responsibility |
|---|---|
| `gorest` | Application configuration, subsystem registration, lifecycle, route helpers, operational endpoints, and application-facing sessions |
| `server` | Fiber server configuration, routing, middleware, and request context |
| `database` | Seven database drivers, native operations, connection configuration, and migrations/seeders |
| `orm` | Shared query-building and mapping behavior used by application database sessions |
| `redis` | Redis connection and client configuration |
| `filter` | Optional JWT creation and request authentication middleware |
| `log` | Structured logging, file rotation, and heuristic redaction |
| `helper` | Request-body parsing, conversions, and hash helpers |
| `view` | Optional JSON response envelope |

Use the [API reference](api-reference.md) for public package symbols and the
[feature guide](production-validation.md) for supported integrations.

## Application lifecycle

1. `gorest.New(...)` applies application defaults and initializes the logger,
   Fiber server, and an optional filter configured with a signing key.
2. `app.Use(...)` registers subsystem configuration. Database and Redis
   connections are initialized when their enabled configurations are
   registered; logger and server settings can be customized there as well.
3. Application code registers routes and handlers through `app` or the server
   APIs. Handlers use application-facing database, Redis, filter, and logger
   accessors as needed.
4. `app.Start()` logs production warnings, registers operational routes, then
   runs configured migrations and seeders before starting the HTTP listener.
   Startup tasks are skipped when their configuration is disabled. A startup
   error prevents the HTTP server from being treated as ready.
5. On shutdown, GOREST stops the HTTP server before closing registered
   dependencies. Applications can call `app.Close()` when they manage
   shutdown themselves.

For exact configuration defaults and lifecycle behavior, see the
[configuration guide](configuration.md), [router guide](guide/router.md), and
[migration and seeder guide](guide.md#migration-seeder).

## Request and data flow

The Fiber server dispatches requests to application handlers. Handlers decide
what to validate, authorize, and return. They can use GOREST's optional
`helper.BodyRequest` to parse a request body, the ORM for supported mapped
operations, or native database APIs for database-specific queries. SQL values
should be bound as parameters; native query APIs do not make SQL assembled
from untrusted text safe.

The optional JWT filter verifies tokens and exposes token details to
middleware-protected handlers. It does not authenticate user credentials,
check account state, or authorize access to application resources. Those
policies belong to the application or its identity provider.

See the [request and response guide](guide/response/view_response.md),
[querying guide](guide/querying.md), and [filter guide](guide/filter.md).

## Scope

GOREST provides reusable application infrastructure, while application code
retains control of routes, handlers, and business rules. Its built-in scope
includes:

- Application configuration, subsystem initialization, startup tasks, and
  graceful server and dependency shutdown.
- HTTP routing and configurable middleware for CORS, panic
  recovery, request IDs, compression, security headers, and rate limiting.
- Structured logging with rotating file output and heuristic redaction.
- Database connections for PostgreSQL, MySQL, SQL Server, Oracle, SQLite,
  MongoDB, and ScyllaDB; native driver access; and ORM-based data access where
  supported.
- Database migrations and seeders, plus optional Redis connectivity.
- Optional filter authentication and token-verification middleware, request parsing and
  conversion helpers, and an optional JSON response envelope.
- Built-in liveness, readiness, runtime metrics, and database storage-usage
  endpoints, enabled by default and configurable with
  `DisableMonitoringEndpoints`.

These features are building blocks, not a complete application or deployment.
GOREST does not provide user credential management, account/session lifecycle
or revocation, application-specific authorization, a monitoring dashboard or
Prometheus/OpenTelemetry exporter, a file upload/storage service, or a
background job queue. Use application code or suitable Go packages and
services for those requirements. The application and deployment remain
responsible for secrets management, access policies for operational endpoints,
TLS and network configuration, backups, failover, capacity planning, and
validating security and reliability in their actual environment.
