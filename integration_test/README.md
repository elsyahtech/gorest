# Integration tests

Select the suite that matches the required verification scope:

| Suite | Command | Requirements |
| --- | --- | --- |
| Driver APIs and ORM | `go test -tags=integration ./integration_test/<driver> -count=1 -v` | Configured service and `GOREST_IT_*` variables; SQLite is temporary |
| HTTP end-to-end | `make integration-test-<driver>-e2e` | Docker for remote drivers; Oracle also needs host Instant Client |
| App, filter, logger | `make integration-test-app-filter-log` | No external service |
| Redis, managed disposable | `make integration-test-redis-e2e` | Docker; starts and removes a disposable `redis:7-alpine` container |
| Redis, external endpoint | `go test -tags=integration ./integration_test/redis -count=1 -v` | Configured Redis service; use for TLS endpoint validation |

These suites verify framework and driver behavior. They do not validate an
application's schema, business logic, authorization policy, or deployment.
For a manual CRUD walkthrough, see the
[`main.go` ORM tutorial](../docs/guide/orm-tutorial.md#2-run-one-manual-crud-flow).
Recorded endpoint-specific verification results are maintained in the
[database testing results](../docs/testing-result.md).

## Managed HTTP end-to-end runners

Each runner creates an isolated database/schema, exercises a GOREST example
handler over HTTP, verifies responses and persisted state, then cleans up.
Coverage includes CRUD, bound-value search, validation, duplicate-key and
not-found responses, plus 12 concurrent Create/GET/Delete requests. MongoDB
and ScyllaDB use driver-specific assertions.

The managed SQL HTTP runners also execute shared ORM coverage scenarios
against the same driver: relation preloads and joins, nullable values,
pagination boundaries, insert atomicity, live query cancellation, Upsert, and
invalid inputs. MongoDB and ScyllaDB run their driver-specific ORM coverage
scenarios as part of their managed HTTP runners.

Each driver also tests migration/seeder execution and repeat runs, lock
contention, and recovery after a migration process is killed. Container-backed
drivers additionally test a forced database restart. PostgreSQL, MySQL, and
SQL Server test interruption during an active migration statement; other
drivers test recovery at a file boundary. SQLite tests killed-process recovery
without a separate server. These tests do not simulate host power loss,
storage corruption, or production deployment ordering.
The ScyllaDB managed HTTP runner also verifies outage and recovery using the
same open GOREST database session.
The managed ScyllaDB runner also verifies mTLS: the server rejects a
connection without a client certificate and accepts a trusted client
certificate.
The managed MongoDB runner performs the same client-certificate rejection and
acceptance checks over TLS.

The five SQL runners also verify eight concurrent SQL transactions. MongoDB
and ScyllaDB do not claim SQL-like transaction atomicity.

| Driver | Command | Default setup |
| --- | --- | --- |
| PostgreSQL | `make integration-test-postgres-e2e` | Disposable PostgreSQL container |
| Oracle | `make integration-test-oracle-e2e` | Oracle Free container; host Instant Client required. Set `GOREST_IT_ORACLE_HOST`, `GOREST_IT_ORACLE_PORT`, `GOREST_IT_ORACLE_NAME`, `GOREST_IT_ORACLE_USERNAME`, `GOREST_IT_ORACLE_PASSWORD`, and `GOREST_IT_ORACLE_TLS` to use an existing test database instead |
| SQL Server | `GOREST_IT_SQLSERVER_ACCEPT_EULA=Y make integration-test-sqlserver-e2e` | SQL Server 2022 container; license opt-in required |
| MySQL | `make integration-test-mysql-e2e` | `mysql:8.4` container |
| SQLite | `make integration-test-sqlite-e2e` | Temporary database; no container |
| MongoDB | `make integration-test-mongo-e2e` | `mongo:7.0` container; JSON migrations/seeders |
| ScyllaDB | `make integration-test-scylla-e2e` | `scylladb/scylla:2025.1`; single-node developer mode |

Run driver suites individually when isolating a driver-specific issue. Required
container images are downloaded on first use.
Override container images with `GOREST_IT_<DRIVER>_IMAGE`.
SQL Server is validated on x86-64 Linux, not ARM64 or emulation. Oracle
initialization may take several minutes.

For an external MySQL TLS endpoint, the driver suite checks successful
system-root and hostname validation and rejection of an untrusted CA and
hostname. It also logs the server-reported `@@version_comment` alongside
`VERSION()`; these are server claims, not cryptographic vendor attestation.
The optional MySQL mTLS test requires a separate disposable account and
endpoint configured to require a client certificate (for example, account
`REQUIRE X509`). Set `GOREST_IT_MYSQL_MTLS_HOST`, `_PORT`, `_NAME`,
`_USERNAME`, `_PASSWORD`, `_TLS`, `_SSL_CA`, `_CLIENT_CERT`, and `_CLIENT_KEY`.
The test verifies that the server rejects the account without a client
certificate, then connects with the configured client certificate and checks
that TLS was negotiated. Use a separate test account; changing the regular
integration account to require a certificate could affect other tests or
applications.

For PostgreSQL mTLS, configure a separate disposable endpoint and account
whose server-side TLS/authentication policy requires a trusted client
certificate. Set `GOREST_IT_POSTGRES_MTLS_HOST`, `_PORT`, `_NAME`, `_USERNAME`,
`_PASSWORD`, `_SSL_CA`, `_SSL_CERT`, and `_SSL_KEY`. The integration test uses
`verify-full`, confirms the server reports the client certificate for the
session, and verifies that the same account cannot connect when the client
certificate is omitted. Client certificate and key files must be readable by
the test process; do not use a production account.

### Preparing a disposable PostgreSQL mTLS endpoint

For a Docker-based endpoint, keep the current server certificate and key if
they already provide valid TLS for the endpoint hostname. Add the following
server configuration using the deployment's existing configuration mechanism:

- Enable PostgreSQL TLS and configure its server certificate and private key.
- Set `ssl_ca_file` to a PEM CA certificate that signs the test client
  certificate. This is the server-side trust used to verify client identity.
- Add a `hostssl` rule for the disposable database, test account, and test
  runner source address. Require client-certificate verification with
  `clientcert=verify-ca` and the selected password authentication method.
- Ensure no earlier or broader `pg_hba.conf` rule allows this test account to
  connect without a client certificate. Keep the rule limited to the test
  database, account, and runner network.

For example, place narrowly scoped rules before any broader matching rules:

```conf
hostnossl  <test_database> <test_user> <test_runner_cidr> reject
hostssl    <test_database> <test_user> <test_runner_cidr> scram-sha-256 clientcert=verify-ca
```

Replace the placeholders with the disposable database, role, and test
runner's source CIDR. Confirm the rule order does not leave an earlier
matching rule that permits password-only access.

The test runner's `GOREST_IT_POSTGRES_MTLS_SSL_CA` has a different purpose: it
must contain the CA that validates the PostgreSQL **server** certificate.
`_SSL_CERT` and `_SSL_KEY` identify the test client's certificate and private
key. The server-side `ssl_ca_file` must trust the CA that issued that client
certificate. These CA files may differ; do not substitute a mixed-purpose CA
bundle without reviewing its trust scope.

Use a client certificate valid for client authentication and a least-privilege
test role. Ensure the mounted PostgreSQL server key is readable by the
PostgreSQL process and is not accessible to other users. Reload or restart the
disposable service after changing TLS or `pg_hba.conf`, then run:

```sh
go test -tags=integration -count=1 ./integration_test/postgres \
  -run '^TestPostgresMutualTLSRequiresClientCertificate$' -v
```

The scenario must connect with the trusted client certificate and fail without
it. Do not configure this policy on a shared or production account.

### ScyllaDB mTLS tests

The managed ScyllaDB E2E runner creates a disposable CA and verifies that a
TLS-enabled server requiring client certificates rejects a connection without
one, while GOREST connects successfully with a trusted client certificate.
Run it with:

```sh
make integration-test-scylla-e2e
```

To test an external ScyllaDB endpoint, configure a disposable endpoint to
require client certificates and set `GOREST_IT_SCYLLA_MTLS_HOST`, `_PORT`,
`_NAME`, `_USERNAME`, `_PASSWORD`, `_SSL_CA`, `_SSL_CERT`, and `_SSL_KEY`.
`_SSL_CA` must trust the server certificate; the server must separately trust
the CA that issued `_SSL_CERT`. The endpoint test verifies rejection without
the client certificate and success with it:

```sh
go test -tags=integration -count=1 ./integration_test/scylla \
  -run '^TestScyllaMTLSExternal$' -v
```

Keep the account, certificates, and keys disposable and private. Do not paste
credentials or private keys into chat or commit them to the repository.

### MongoDB mTLS tests

The managed MongoDB E2E runner creates a disposable CA and verifies that the
TLS-required server rejects clients without a certificate and accepts a trusted
client certificate. Run it with:

```sh
make integration-test-mongo-e2e
```

For an external endpoint, configure a disposable MongoDB test endpoint to
require trusted client certificates. Set `GOREST_IT_MONGO_MTLS_HOST`, `_PORT`,
`_NAME`, `_USERNAME`, `_PASSWORD`, `_TLS=true`, `_SSL_CA`, `_SSL_CERT`, and
`_SSL_KEY`; set `_AUTH_SOURCE` if the test account is not authenticated against
`admin`. The CA path validates the server certificate; MongoDB must separately
trust the CA that issued the client certificate. The external test checks
rejection without a client certificate, then verifies a successful
certificate-authenticated TLS connection:

```sh
go test -tags=integration -count=1 ./integration_test/mongo \
  -run '^TestMongoMTLSExternal$' -v
```

For MongoDB, `_SSL_CERT` and `_SSL_KEY` are separate PEM files; `_SSL_CA` is
the CA bundle used to verify the server certificate. The client certificate
CA must be trusted by the MongoDB server independently of that server CA.
Use a disposable, least-privilege test account and certificate. Keep private
keys private; do not paste them into chat or commit them to the repository.

When an Oracle host is provided, the HTTP E2E test uses that existing endpoint
instead of creating a container. It creates uniquely named temporary test
tables and GOREST migration metadata in the configured schema, cleans up the
test objects, and does not stop or restart the external database. Use a
disposable test schema; do not point this test at production.

Use `GOFLAGS=-race` to run Go's race detector, for example:

```sh
GOFLAGS=-race make integration-test-postgres-e2e
```

This detects Go memory races, not database isolation or load behavior.

## Coverage runs

Run unit tests with coverage across production packages:

```sh
make unit-coverage
```

For the managed all-services integration matrix and one combined production
coverage profile, start Docker and run:

```sh
GOREST_IT_SQLSERVER_ACCEPT_EULA=Y make integration-test-coverage
```

Review and accept Microsoft's SQL Server container license before setting the
EULA variable. This matrix also needs Oracle host Instant Client. It starts
managed containers for the HTTP database scenarios and Redis; database
service-package tests requiring configured `GOREST_IT_*_HOST` endpoints skip
when no endpoint is configured. The command serializes package execution to
avoid resource contention and writes both a coverage profile (`.out`) and
matching HTML report (`.html`) under `integration_test/reports/`. The HTML is
generated only after the full integration matrix passes. Inspect functions
with `go tool cover -func=<profile>` or open the generated HTML report in a
browser.

Coverage shows which statements ran, not whether every behavior is correct.
Use it to identify branches needing meaningful assertions; do not force 100%
by invoking internal helpers without checking behavior.

### Run reports

Runners save a JSON Lines event stream and JSON summary in
`integration_test/reports/` (override with `GOREST_IT_REPORT_DIR`). Reports
include command, start time, result, exit code, and available server version;
they exclude credentials and Oracle client paths. Keep both files as evidence
of the specific environment and scenarios tested.

Migration scripts may not be atomic across drivers. Inspect the database
before retrying a failed script. See the
[migration and seeder guide](../docs/guide.md#migration-seeder)
for recovery guidance.

## Integration suite map

The configured driver packages use the `integration` build tag. They connect
to real services and create uniquely named temporary tables or collections.
The SQL driver packages share common CRUD/bound-value, restricted-`Where()`
grammar rejection, transaction rollback,
pagination/grouping, and extended-coverage helpers where those scenarios are
implemented for that driver. MongoDB and ScyllaDB use separate NoSQL scenarios.

### External TLS/mTLS tests

The driver suites document the TLS and mTLS scenarios they can exercise and
how to configure external test endpoints. For endpoint-specific results and
the exact server profiles tested, see the
[database testing results](../docs/testing-result.md).

| Package | Main scenarios | Coverage details |
| --- | --- | --- |
| [`sqlite/`](./sqlite/) | CRUD and bound values; transaction rollback; **8 parallel SQL transactions**; pagination and grouping; extended ORM coverage | Join/Preload, nullable mapping, pagination boundaries, multi-row insert failure, live-query cancellation, Upsert, invalid ORM inputs |
| [`mysql/`](./mysql/) | Server version and charset; CRUD and bound values; transaction rollback; **8 parallel SQL transactions**; pagination and grouping; extended ORM coverage | Same extended SQL scenarios, including MySQL-specific Upsert behavior |
| [`postgres/`](./postgres/) | Server version; CRUD and bound values; transaction rollback; **8 parallel SQL transactions**; pagination and grouping; optional mTLS endpoint verification | Join/Preload, nullable mapping, pagination boundaries, multi-row statement atomicity, live-query cancellation, Upsert matrix, PostgreSQL row-level security |
| [`oracle/`](./oracle/) | Server version; CRUD and bound values; transaction rollback; **8 parallel SQL transactions**; pagination and grouping; extended ORM coverage | Join/Preload, nullable mapping, pagination boundaries, multi-row insert failure, live-query cancellation, Upsert, invalid ORM inputs |
| [`sqlserver/`](./sqlserver/) | Server version; CRUD and bound values; transaction rollback; **8 parallel SQL transactions**; pagination and grouping; extended ORM coverage | Join/Preload, nullable mapping, pagination boundaries, multi-row insert failure, live-query cancellation, Upsert, invalid ORM inputs |
| [`mongo/`](./mongo/) | Server version; CRUD and bound values; extended ORM coverage | Alternate-unique-key Upsert, unfiltered-write guards, nullable mapping, canceled-context propagation |
| [`scylla/`](./scylla/) | Server version; CRUD; extended ORM coverage | Composite-key Upsert validation, unfiltered-write guards, nullable mapping, canceled-context propagation |
| [`httpapi/`](./httpapi/) | Managed HTTP CRUD, bound-value search, and 12-way concurrent Create/GET/Delete checks per driver; PostgreSQL additionally verifies TLS encryption, trusted-CA/hostname validation, and untrusted-CA rejection | Requires both `integration` and `integrationcontainers` tags; use the Make targets above |
| [`app/`](./app/) | App startup and signal-driven graceful shutdown with an in-flight request | Helper-process test of the public `app.Start()` path |
| [`log/`](./log/) | Configured file output, rotation, and redaction | Uses isolated temporary files |
| [`redis/`](./redis/) | Operations, concurrent pool use, operation/context timeouts, startup failure, reconnect after server-dropped client connections, and TLS rejection | `make integration-test-redis-e2e` starts and removes disposable Redis; external TLS checks use the configured endpoint |

Run just the parallel SQL transaction scenario after configuring the selected
driver's `GOREST_IT_*` environment variables:

```sh
GOREST_IT_REQUIRED=1 go test -race -tags=integration \
  ./integration_test/postgres -run '^TestORMParallelTransactions$' -count=1 -v
```

Replace `postgres` with `mysql`, `oracle`, `sqlserver`, or `sqlite`.

**Transaction coverage**
- The test runs eight independent SQL transactions concurrently. Each inserts
  and commits one row; the test verifies all eight.
- MongoDB and ScyllaDB are excluded: their transaction APIs do not guarantee
  SQL-like atomicity. Their HTTP runners still test concurrent CRUD.
- `-race` checks Go memory access, not database throughput or server-side
  isolation. Remove it if the selected environment cannot build with it.

**ORM and HTTP coverage limits**
- Extended SQL tests cover representative cases, not every dialect or schema.
  PostgreSQL has separate named tests, including RLS.
- Multi-row atomicity applies to one `INSERT` statement, not transactions across
  statements or chunks. NoSQL canceled-context tests start with an already
  canceled context; they do not interrupt an operation already running on the
  server. NoSQL transaction methods do not establish SQL-like atomicity.
- HTTP tests demonstrate binding for the tested search value. ORM suites also
  bind special characters (`'`, `?`, `--`, `/* ... */`) and check that
  `Where()` rejects SQL literals/comments with HTTP 400 across all seven
  drivers. Rejected conditions stop at ORM validation and are not sent to the
  database.
- These checks do not prove arbitrary dynamic table, column, sort, or clause
  expressions are safe. They do not test application authorization, secret
  management, or every TLS configuration.
- Concurrent HTTP coverage is limited to 12 simultaneous requests per
  operation; it is not a sustained load, throughput, or soak test.

## Running driver integration suites

Run an individual package with:

```sh
go test -tags=integration ./integration_test/<driver> -count=1 -v
```

The broad command `go test -tags=integration ./integration_test/...` also
runs Redis and requires a Redis service. For managed Redis, use
`go test -tags=integration,integrationcontainers ./integration_test/redis`.
Remote driver suites skip when
unconfigured; set `GOREST_IT_REQUIRED=1` in CI to make missing configuration
fail. SQLite uses a temporary database. Other drivers use
`GOREST_IT_<DRIVER>_HOST`, `PORT`,
`NAME`, `USERNAME`, `PASSWORD`, and optional `TLS`, `SSL_CERT`, `SSL_KEY`,
`SSL_CA`, `AUTH_SOURCE` (MongoDB; defaults to `admin`), `LIB_DIR`, or
`TRUST_SERVER_CERTIFICATE` settings. The selected
database account must be able to create and drop test tables or collections;
never use production data. See the
[database configuration guide](../docs/configuration.md) for TLS options.
Oracle needs host Instant Client; ScyllaDB needs an existing keyspace. Its ORM
does not support conditional Upsert. ScyllaDB transport TLS is configured with
`TLS=enable`; use `SSL_CA`, `SSL_CERT`, and `SSL_KEY` (cert/key together for
mTLS) to configure verified certificates.

PostgreSQL's `TestPostgresRowLevelSecurityEnforcesTenantPolicy` checks
tenant-scoped reads/writes and transaction-local settings on a reused pooled
connection. Use an isolated database and a non-superuser role without
`BYPASSRLS`; it needs table create/drop privileges. A test role does not prove
the production role's behavior.

## ORM scenarios by driver

Run shared SQL coverage against MySQL, Oracle, SQL Server, and SQLite:

```sh
GOREST_IT_REQUIRED=1 go test -tags=integration ./integration_test/mysql ./integration_test/oracle ./integration_test/sqlserver -run TestORMCoverage -count=1 -v
go test -tags=integration ./integration_test/sqlite -run TestORMCoverage -count=1 -v
```

Shared SQL scenarios cover joins/preloads, nullable results, pagination,
multi-row insert failure, live-query cancellation, Upsert, and invalid ORM
input. Upsert support varies by driver; this is not an exhaustive dialect or
constraint matrix. Cancellation depends on driver behavior and server
permissions. Multi-row failure tests one statement, not a transaction across
multiple statements or chunks.

PostgreSQL uses separate tests in [`postgres/coverage_test.go`](./postgres/coverage_test.go):

| Scenario | PostgreSQL test |
| --- | --- |
| Explicit JOIN and belongs-to/has-many preload mapping | `TestPostgresJoinAndRelationPreloadMapping` |
| Nullable result mapping | `TestPostgresNullableColumnMapping` |
| Pagination boundary inputs | `TestPostgresPaginationBoundaries` |
| Multi-row INSERT statement atomicity | `TestPostgresBulkInsertFailureIsStatementAtomic` |
| Live-query context cancellation | `TestPostgresContextCancellationStopsLiveQuery` |
| Primary-key, alternate-unique-key, and conditional Upsert | `TestPostgresUpsertBehaviorMatrix` |

Invalid ORM input rejection is covered by `TestORMCoverage` for MySQL, Oracle,
SQL Server, and SQLite; PostgreSQL has no dedicated test for it.

MongoDB tests nullable mapping, alternate-key Upsert, unfiltered-write guards,
and canceled-context propagation. ScyllaDB tests nullable mapping,
composite-key Upsert validation, unfiltered-write guards, and canceled-context
propagation. Joins/preloads and SQL pagination do not apply to these drivers.

## Application, JWT filter, and logger integration tests

These suites need no external database or Redis service. Run the report-enabled
target:

```sh
make integration-test-app-filter-log
```

It prints each test result and saves a JSON summary plus JSON Lines test events
under `integration_test/reports/` (or `GOREST_IT_REPORT_DIR`). To run Go's
test command directly without report files:

```sh
go test -tags=integration ./integration_test/app ./integration_test/filter ./integration_test/log -count=1 -v
```

- **App:** `app.Start()` handles `SIGTERM` and completes an in-flight request.
  Validate the 30-second shutdown grace period with your process supervisor.
- **Filter:** token issuance, signature/claim checks, cookie settings, and HTTP
  authentication/refresh. Does not test application login, account status,
  authorization, revocation, or CSRF policy.
- **Logger:** file output, rotation limits, and redaction of recognized
  sensitive values.

The separate unit test `TestAppCloseGracefullyShutsDownHTTPServer` checks
`app.Close()` directly. The helper-process integration test above exercises
the public `app.Start()` signal-handling path.

## Redis integration tests

Redis tests use `127.0.0.1:6379` by default and require a running service.
Override the connection with:

| Variable | Required | Purpose |
| --- | --- | --- |
| `GOREST_IT_REDIS_HOST` | No | Redis host; defaults to `127.0.0.1` |
| `GOREST_IT_REDIS_PORT` | No | Redis port; defaults to `6379` |
| `GOREST_IT_REDIS_USERNAME` | No | Redis ACL username |
| `GOREST_IT_REDIS_PASSWORD` | No | Redis password |
| `GOREST_IT_REDIS_TIMEOUT` | No | Per-operation timeout; defaults to `300ms`, configurable up to `5s` for remote latency |
| `GOREST_IT_REDIS_DATABASE` | No | Redis logical database index |
| `GOREST_IT_REDIS_TLS` | No | Enable TLS (`true`/`false`; defaults to `false`) |
| `GOREST_IT_REDIS_SSL_CA` | No | CA certificate path used for server verification |
| `GOREST_IT_REDIS_SSL_CERT` | No | Client certificate for mTLS |
| `GOREST_IT_REDIS_SSL_KEY` | No | Client private key for mTLS |

Run only the Redis checks with:

```sh
GOREST_IT_REDIS_HOST=HOST_ADDRESS \
GOREST_IT_REDIS_PORT=6379 \
GOREST_IT_REDIS_USERNAME=USERNAME_REDIS \
GOREST_IT_REDIS_PASSWORD=PASSWORD_REDIS \
GOREST_IT_REDIS_DATABASE=0 \
GOREST_IT_REDIS_TLS=false \
go test -tags=integration ./integration_test/redis -count=1 -v
```

Coverage includes app registration/shutdown, unique-key SET/GET/DEL,
concurrent pool use, timeouts, initialization failure, and reconnection after
the managed Redis server drops existing client connections. This does not stop
the Redis service or prove recovery from a full outage, failover, or topology
change. TLS and certificate checks need a TLS-enabled endpoint.
For remote endpoints, set `GOREST_IT_REDIS_TIMEOUT=5s` if the default
`300ms` operation timeout is too short; the test maximum is `5s` so its
timeout assertions remain bounded.

Use these suites as framework-level checks, not substitutes for application
E2E tests. For deployment evidence, match the application's database versions,
schema, TLS settings, and permissions.
