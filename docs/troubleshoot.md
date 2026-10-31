# Troubleshooting guide

Use the entries below to narrow down common setup and runtime problems. Each
entry separates the symptom, possible causes, diagnosis, and resolution. A
possible cause is a lead to check, not a confirmed diagnosis; use the
application and service logs to identify the actual failure.

## Installation and startup

### Go reports an unsupported version or cannot resolve GOREST

**Problem:** `go get`, `go mod tidy`, build, or test cannot use the GOREST
module.

**Possible Cause:** The installed Go toolchain may be older than the version
required by `go.mod`, or the module proxy/network may not be able to retrieve
the requested module version.

**How to Diagnose:** Run `go version`, inspect the `go` directive in the
selected GOREST version's `go.mod`, and read the complete module error.

**Solution:** Install a compatible Go version, select an available module
version, and retry module resolution. For version selection, see
[versions and change history](versioning.md).

**Related Documentation:** [Installation quick start](../README.md#quick-start),
[Go module versioning](versioning.md).

### Application exits or does not listen on the expected address

**Problem:** The application exits during startup, or requests cannot reach
the expected HTTP address.

**Possible Cause:** Server configuration, route setup, startup tasks, or a
registered database/Redis dependency may be invalid or unavailable. The
configured server address or port may also differ from the address being
tested.

**How to Diagnose:** Read startup and server logs, inspect the effective
`server.Config`, confirm the host and port, and check whether startup tasks or
dependency startup reports an error.

**Solution:** Correct the relevant configuration or service availability.
For local development, confirm the configured listener address and port, then
request a route registered by the application.

**Related Documentation:** [Architecture and lifecycle](architecture.md),
[server and router guide](guide/router.md),
[configuration guide](configuration.md).

## Requests, routes, and responses

### Request body parsing returns `400 Bad Request`

**Problem:** `helper.BodyRequest` reports a parse error or the handler returns
HTTP 400 for a request body.

**Possible Cause:** The request may have an incorrect or missing content type,
malformed JSON, or values whose types do not match the destination struct.

**How to Diagnose:** Inspect the request's `Content-Type`, the sanitized body
shape, and the wrapped parsing error. Reproduce with the smallest valid
payload for the handler.

**Solution:** Send the expected content type and valid data types, then apply
business validation separately after parsing.

**Related Documentation:** [Helper package guide](guide/helpers.md),
[request and response guide](guide/response/view_response.md).

### Route returns `404 Not Found`

**Problem:** A request receives a 404 even though a handler is expected.

**Possible Cause:** The route may not have been registered, the HTTP method or
path may differ, or a route group may add a prefix.

**How to Diagnose:** Compare the request method and path with the registered
route, including group prefixes and path parameters. Confirm that the route
registration function is called for the running application.

**Solution:** Register the handler on the intended application/server and use
the matching method and full path.

**Related Documentation:** [Server and router guide](guide/router.md),
[application architecture](architecture.md).

## Database and query errors

### Database startup fails

**Problem:** Startup reports a database connection or configuration error.

**Possible Cause:** Driver name, service/database name, address, credentials,
TLS configuration, client-library setup, network access, or service readiness
may not match the target database.

**How to Diagnose:** Read the startup error and driver-aware message without
publishing secrets. Confirm the selected driver, host, port, name, credentials
source, service health, and TLS/client requirements. For Oracle, also check
Instant Client discovery and wallet configuration.

**Solution:** Correct the setting that differs from the target environment.
Use a dedicated test database and validate credentials and TLS independently
when possible.

**Related Documentation:** [Database configuration](configuration.md),
[database and framework guides](testing-result.md),
[Oracle configuration](testing-result/testing-result-oracle.md).

### Database startup tasks fail

**Problem:** `app.Start()` fails while running migrations or seeders, or
`app.RunDatabaseStartupTasks()` returns that the database service is not
running.

**Possible Cause:** Startup tasks are enabled but no enabled database service
has been registered with the application.

**How to Diagnose:** Check the database configuration's `Disable`,
`Migration`, and `Seeder` fields, and confirm the configured database startup
error is resolved.

**Solution:** Register an enabled database configuration with
`app.Use(database.Run(config))` before `app.Start()` or
`app.RunDatabaseStartupTasks()`. `database.Run(...)` normalizes configuration;
`app.Use(...)` starts the database service.

**Related Documentation:** [Database configuration](configuration.md),
[application guide](guide.md).

### ORM returns `400`, `404`, `409`, `503`, `504`, or `500`

**Problem:** An ORM operation returns a non-success status.

**Possible Cause:** `400` commonly indicates invalid input or an unsupported
query shape; `404` can mean a requested single row was not found; `409` can
indicate a primary-key or unique-value conflict; `503` indicates an
unavailable database service; `504` indicates a timeout; and `500` can result
from schema, permission, conversion, or other driver errors.

**How to Diagnose:** Inspect the operation's `HTTPCode`, `Message`, and
wrapped `Error`. Compare the query with the model's `gorest` tags, schema,
constraints, selected columns, and the active driver's feature matrix.

**Solution:** Correct the input or query shape, ensure the target row/schema
and permissions are present, or address the underlying service/timeout
problem. Do not expose raw driver errors as public API responses.

**Related Documentation:** [ORM error contract and troubleshooting](guide/orm.md#8-troubleshooting),
[ORM feature matrix](guide/orm.md#7-driver-feature-matrix),
[native query guidance](guide/querying.md).

### Native SQL reports binding, scan, or query errors

**Problem:** A native SQL operation fails to bind values, scan rows, or return
the expected result.

**Possible Cause:** Placeholder syntax may not match the driver; arguments
may be missing or ordered incorrectly; selected columns may not match scan
destinations; or request data may have been concatenated into SQL text.

**How to Diagnose:** Reduce the statement to a minimal query, compare its
placeholders with supplied arguments, and verify column order and types.
Inspect the returned error and close rows after use.

**Solution:** Bind user-controlled values as arguments, keep identifiers and
SQL syntax fixed or allowlisted, match scan destinations to the selected
columns, and check `rows.Err()` after iteration.

**Related Documentation:** [Native query guide](guide/querying.md#native-sql),
[ORM and native API comparison](guide/querying.md).

### Migration or seeder reports an unfinished run or active lock

**Problem:** Startup refuses to continue because migration/seeder state is
unfinished or another runner holds the database startup lock.

**Possible Cause:** Another migration job may still be running, or a previous
run may have stopped after recording state but before completing its work.

**How to Diagnose:** Stop concurrent migration jobs. Inspect the affected
schema/data, migration history, `gorest_migration_state`, and the startup lock
using the database's native tools. Compare recorded state with the actual
effects of the migration/seeder.

**Solution:** Reconcile the state with what actually completed before
manually removing a stale lock. Do not clear the state or lock blindly or
retry against production data without inspection.

**Related Documentation:** [Migration and seeder recovery](guide.md#migration-seeder),
[migration examples](../database/migrations/README.md),
[seeder examples](../database/seeders/README.md).

### MongoDB or ScyllaDB operation is assumed to be transactional

**Problem:** A sequence of operations appears to commit or roll back even
though an error occurs.

**Possible Cause:** GOREST's generic SQL transaction methods do not provide
transaction atomicity for MongoDB or ScyllaDB.

**How to Diagnose:** Check the selected driver and confirm whether the code
uses the native transaction/conditional operation supported by that database.

**Solution:** Use MongoDB native session transactions where the deployment
supports them, or ScyllaDB conditional statements/lightweight transactions
within their documented constraints. Otherwise redesign the operation so it
does not depend on multi-operation atomicity.

**Related Documentation:** [Query and transaction behavior](guide/querying.md#nosql-transaction-behavior),
[MongoDB guide](testing-result/testing-result-mongo.md),
[ScyllaDB guide](testing-result/testing-result-scylla.md).

## Redis and authentication

### Redis startup or commands fail

**Problem:** Application startup cannot run Redis, or Redis commands
time out or fail.

**Possible Cause:** Redis may be unreachable, credentials or TLS settings may
be incorrect, the configured timeout may be too short, or Redis may be
registered when the application intends it to be disabled.

**How to Diagnose:** Check whether Redis is registered and enabled, verify
host/port and service health, inspect sanitized connection errors, and confirm
the TLS CA and client certificate settings when used.

**Solution:** Align the Redis configuration with the service and choose
timeouts suitable for the application. Decide explicitly whether Redis is a
required dependency and how the application behaves when it is unavailable.

**Related Documentation:** [Redis guide](testing-result/testing-result-redis.md),
[configuration guide](configuration.md),
[Redis integration setup](../integration_test/README.md).

### JWT filter is unavailable or rejects a request

**Problem:** JWT filter setup aborts application startup, or authentication
middleware rejects a request.

**Possible Cause:** JWT is optional and remains disabled unless the application
registers a filter configuration. Explicit filter configuration with a missing
key, unsupported signing method, or invalid token duration aborts startup.
Authentication can also reject an expired/invalid token, an unexpected issuer
or signing method, or a token in the wrong header/cookie.

**How to Diagnose:** Confirm filter configuration is registered before route
setup and that `app.Filter()` is called only after the filter is running. For request
failures, check whether the request uses the `Authorization` header, `access`
cookie, or `refresh` cookie without logging token values.

**Solution:** Load a strong signing key from protected configuration, use the
configured token transport, and check expiration and issuer settings. Add
application-level account-state and resource authorization checks; successful
token validation alone does not grant authorization.

**Related Documentation:** [JWT filter guide](guide/filter.md),
[security policy](../SECURITY.md).

## Testing and deployment

### Integration tests skip, fail to compile, or cannot start a service

**Problem:** A test is skipped, a package reports no tests, or an
integration-backed service does not start.

**Possible Cause:** Some tests require build tags, Docker, an Oracle Instant
Client, or configured external-service variables. Running a package without
the required build tags or service does not exercise that integration case.

**How to Diagnose:** Check the test command, build tags, Docker availability,
required client libraries, and the documented `GOREST_IT_*` variables. Read
the test output to distinguish a prerequisite issue from an assertion failure.

**Solution:** Follow the exact setup for the selected suite and use a
disposable service/database. Do not treat a skipped or unconfigured test as a
passing integration result.

**Related Documentation:** [Integration test guide](../integration_test/README.md),
[Go module versions](versioning.md).

### Startup rejects an unencrypted production-like connection

**Problem:** A database or Redis connection without TLS is refused during
production-like startup.

**Possible Cause:** GOREST requires an explicit approval for a non-TLS
connection in production-like environments; headless startup cannot answer an
interactive prompt.

**How to Diagnose:** Check the environment name, configured TLS mode, and
startup error. Confirm whether the selected driver requires a CA, wallet, or
other TLS material.

**Solution:** Prefer enabling TLS and validating the server certificate. Use
`AllowInsecureProduction` only as a documented exception for an intentional
trusted-network deployment.

**Related Documentation:** [Database and Redis TLS configuration](configuration.md),
[features and deployment guide](production-validation.md).

## Reading operation errors

For ORM operations, `HTTPCode` is the suggested status, `Message` contains
troubleshooting context, and `Error` contains the underlying error. Log
internal errors on the server and map them to a stable, sanitized public
response. Native database methods provide diagnostic values too, but the
application is responsible for operation-specific classification and public
response handling.

If these steps do not resolve the problem, see the [support guide](support.md)
for the information to include in a report. Never include credentials,
private keys, personal data, or production data.
