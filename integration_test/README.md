# Integration tests

This directory contains three kinds of integration checks. They exercise
GOREST against real services, but they do not replace tests for an application's
own schema, handlers, authorization rules, or deployment configuration.

| Test layer | Build tag / command | What it runs | External setup |
| --- | --- | --- | --- |
| Driver integration suites | `integration`; `go test -tags=integration ./integration_test/<driver> -count=1 -v` | ORM and database API scenarios against a configured service; SQLite uses a temporary file | A service and `GOREST_IT_*` settings for remote drivers; Redis tests are a separate package |
| Managed HTTP end-to-end runners | `integration,integrationcontainers`; `make integration-test-<driver>-e2e` | A small GOREST HTTP server, example handlers, real database, HTTP requests, response assertions, and persisted-state checks | Docker for six containerized drivers; SQLite runs without Docker; Oracle also needs host Oracle Instant Client |
| App, logger, and Redis suites | `integration`; package-specific commands below | Application startup/shutdown, logger output, and Redis operations/pool/TLS behavior | App/logger need no external service; Redis requires a Redis endpoint |

## Managed HTTP end-to-end runners

Each runner starts with an isolated test database or collection, creates its
own schema, starts a small HTTP server in-process, sends requests through the
HTTP routes, verifies response codes and persisted data, and cleans up. The
shared SQL scenario covers Create, an injection-shaped search value passed as
a bound parameter, required-field rejection (`400`), duplicate unique-key
rejection (`409`), Update, GET verification, Delete, and not-found (`404`).
After those checks, every runner also sends 12 Create, 12 GET, and 12 Delete
requests concurrently, then verifies that no test rows remain. MongoDB and
ScyllaDB use driver-specific schemas and assertions described in their rows
below. These are GOREST examples, not tests of a consuming application's
routes, business logic, or authorization.

The five SQL HTTP runners also start eight independent SQL transactions at the
same time, insert and commit one row per transaction, then verify all eight
rows. This makes each managed SQL runner report include both concurrent HTTP
requests and parallel transaction evidence. The configured SQL package suites
expose the same transaction scenario as `TestORMParallelTransactions`.

| Driver | Command | Default service | Notes |
| --- | --- | --- | --- |
| PostgreSQL | `make integration-test-postgres-e2e` | PostgreSQL container | Shared SQL HTTP scenario |
| Oracle | `make integration-test-oracle-e2e` | Oracle Database Free container | Host must have Oracle Instant Client; image startup can take several minutes |
| SQL Server | `GOREST_IT_SQLSERVER_ACCEPT_EULA=Y make integration-test-sqlserver-e2e` | SQL Server 2022 container | Explicit license opt-in; validated on x86-64 Linux Docker hosts |
| MySQL | `make integration-test-mysql-e2e` | `mysql:8.4` | Shared SQL HTTP scenario |
| SQLite | `make integration-test-sqlite-e2e` | Temporary SQLite database | No container or external service |
| MongoDB | `make integration-test-mongo-e2e` | `mongo:7.0` | Creates a unique email index; maps GOREST primary key to MongoDB `_id` |
| ScyllaDB | `make integration-test-scylla-e2e` | `scylladb/scylla:2025.1` | Single-node developer-mode test; checks full primary-key conflict, not email uniqueness |

Run one driver at a time. There is not currently a single all-driver matrix
command. Docker images are downloaded on first use. Image overrides are
available for containerized drivers through `GOREST_IT_<DRIVER>_IMAGE`; SQLite
does not use an image. SQL Server also requires the explicit EULA setting
shown above. Oracle's Go process needs Instant Client on
the host even though Oracle Database itself runs in a container. SQL Server's
container runner is not validated on ARM64 or emulated x86 hosts.

To run a managed HTTP scenario with Go's race detector enabled, set `GOFLAGS`:

```sh
GOFLAGS=-race make integration-test-postgres-e2e
```

Use the matching Make target for another driver; SQL Server still requires
`GOREST_IT_SQLSERVER_ACCEPT_EULA=Y`. This instruments GOREST and the test
process for Go memory races. It does not detect database-server isolation or
replace a sustained load test.

The runners write a JSON Lines Go test-event stream and a JSON summary to
`integration_test/reports/` (or `GOREST_IT_REPORT_DIR`). The summary records
the UTC start time, driver, image or temporary mode, command, result, exit
code, and event filename. The event stream contains test output, including the
database server version when available. Credentials and Oracle client paths
are not included by the runner. Reports are ignored by Git; retain both files
when attaching evidence to a review or audit. A report only proves the run
described by its recorded environment and scenarios.

## Integration suite map

The configured driver packages use the `integration` build tag. They connect
to real services and create uniquely named temporary tables or collections.
The SQL driver packages share common CRUD/bound-value, restricted-`Where()`
grammar rejection, transaction rollback,
pagination/grouping, and extended-coverage helpers where those scenarios are
implemented for that driver. MongoDB and ScyllaDB use separate NoSQL scenarios.

| Package | Main scenarios | Coverage details |
| --- | --- | --- |
| [`sqlite/`](./sqlite/) | CRUD and bound values; transaction rollback; **8 parallel SQL transactions**; pagination and grouping; extended ORM coverage | Join/Preload, nullable mapping, pagination boundaries, multi-row insert failure, live-query cancellation, Upsert, invalid ORM inputs |
| [`mysql/`](./mysql/) | Server version and charset; CRUD and bound values; transaction rollback; **8 parallel SQL transactions**; pagination and grouping; extended ORM coverage | Same extended SQL scenarios, including MySQL-specific Upsert behavior |
| [`postgres/`](./postgres/) | Server version; CRUD and bound values; transaction rollback; **8 parallel SQL transactions**; pagination and grouping | Join/Preload, nullable mapping, pagination boundaries, multi-row statement atomicity, live-query cancellation, Upsert matrix, PostgreSQL row-level security |
| [`oracle/`](./oracle/) | Server version; CRUD and bound values; transaction rollback; **8 parallel SQL transactions**; pagination and grouping; extended ORM coverage | Join/Preload, nullable mapping, pagination boundaries, multi-row insert failure, live-query cancellation, Upsert, invalid ORM inputs |
| [`sqlserver/`](./sqlserver/) | Server version; CRUD and bound values; transaction rollback; **8 parallel SQL transactions**; pagination and grouping; extended ORM coverage | Join/Preload, nullable mapping, pagination boundaries, multi-row insert failure, live-query cancellation, Upsert, invalid ORM inputs |
| [`mongo/`](./mongo/) | Server version; CRUD and bound values; extended ORM coverage | Alternate-unique-key Upsert, unfiltered-write guards, nullable mapping, canceled-context propagation |
| [`scylla/`](./scylla/) | Server version; CRUD; extended ORM coverage | Composite-key Upsert validation, unfiltered-write guards, nullable mapping, canceled-context propagation |
| [`httpapi/`](./httpapi/) | Managed HTTP CRUD, bound-value search, and 12-way concurrent Create/GET/Delete checks per driver | Requires both `integration` and `integrationcontainers` tags; use the Make targets above |
| [`app/`](./app/) | App startup and signal-driven graceful shutdown with an in-flight request | Helper-process test of the public `app.Start()` path |
| [`log/`](./log/) | Configured file output, rotation, and redaction | Uses isolated temporary files |
| [`redis/`](./redis/) | Operations, concurrent pool use, operation/context timeouts, startup failure, and TLS rejection | Requires a Redis endpoint; TLS checks need a TLS-enabled endpoint |

Run just the parallel SQL transaction scenario after configuring the selected
driver's `GOREST_IT_*` environment variables:

```sh
GOREST_IT_REQUIRED=1 go test -race -tags=integration \
  ./integration_test/postgres -run '^TestORMParallelTransactions$' -count=1 -v
```

Replace `postgres` with `mysql`, `oracle`, `sqlserver`, or `sqlite`. The test
starts eight independent transactions together, inserts a distinct row in
each, commits them, and verifies all eight rows. MongoDB and ScyllaDB do not
run this SQL transaction test because GOREST's transaction methods do not
provide transaction atomicity for those drivers. The `-race` flag checks Go
memory accesses exercised by this scenario; it does not measure database
throughput or prove server-side isolation behavior. Remove `-race` if the
selected driver environment cannot be built with Go's race detector.

The extended SQL scenarios are not identical to every possible dialect query
or schema. PostgreSQL has its own named coverage tests, including RLS. The
multi-row atomicity check covers a single multi-row `INSERT` statement; it does
not prove all-or-nothing behavior for transactions spanning multiple
statements or chunks. The NoSQL canceled-context checks use an already-canceled
context and do not simulate cancellation of a request already executing on a
server. NoSQL `BeginTx`/`Commit`/`Rollback` behavior is not evidence of SQL-like
transaction atomicity.

The managed HTTP scenarios demonstrate parameter binding for the specific
search value used by the test. The configured ORM driver suites also exercise
special characters (`'`, `?`, `--`, and `/* ... */`) as bound values and
verify that `Where()` rejects SQL literals/comments in the condition text with
HTTP 400 across all seven drivers. The rejection assertion runs at the ORM
validation boundary; it does not send the invalid condition to the database.
These checks do not prove that arbitrary dynamic table,
column, sort, or clause expressions are safe, and they do not test application
authorization, secret management, or a full TLS configuration matrix. The
concurrent HTTP check is bounded to 12 simultaneous requests per operation;
it is not a sustained load, throughput, or soak test. Parallel SQL transaction
coverage runs on the five SQL drivers. MongoDB and ScyllaDB receive concurrent
CRUD requests through the HTTP runners, without a SQL-like transaction
guarantee.

To run the service-configured database package tests, set the relevant
`GOREST_IT_*` variables and use `go test -tags=integration`; detailed setup and
examples follow. The broad command `go test -tags=integration
./integration_test/...` also includes Redis, so a Redis service must be
available. Managed HTTP tests are excluded from that command unless the
`integrationcontainers` tag is added.

## PostgreSQL HTTP runner details

The managed end-to-end runner starts a temporary PostgreSQL container,
registers a small GOREST HTTP handler that uses the ORM, sends HTTP requests,
checks the responses and persisted rows, and then removes the container. It
does not require a manually installed PostgreSQL server. Docker must be
available, and the first run downloads the PostgreSQL image and Go dependencies.

Run it from the repository root:

```sh
make integration-test-postgres-e2e
```

The test covers HTTP Create, search with an injection-shaped bound value,
required-field validation, duplicate-key conflict handling, Update, Delete,
and a not-found response. It uses an isolated test database and credentials
created for the disposable container. It tests the example handler supplied
by GOREST; it does not test an application's own routes, authorization rules,
or business logic.

The runner writes a JSON Lines test-event file and a JSON summary under
`integration_test/reports/`. The summary records the driver, image, command,
start time, and pass/fail result. The event file includes the test output and
the PostgreSQL server version; it does not print the database password. These
files are ignored by Git and can be attached to an audit run report. A source
test or generated report only describes the environment in which it ran; it
does not establish behavior for another database version or deployment.

## SQLite HTTP runner details

Run the SQLite handler scenario from the repository root:

```sh
make integration-test-sqlite-e2e
```

The runner uses GOREST's SQLite driver and a temporary database file. It needs
no database server, Docker image, or external database credentials. The shared
HTTP scenario checks CRUD, parameter binding, required-field validation,
duplicate-key conflict handling, and not-found behavior. The temporary
database and application log files are removed with the test's temporary
directory.

SQLite run evidence is written to `integration_test/reports/` as a JSON Lines
event log and JSON summary. The summary records the driver, temporary database
mode, command, start time, and result; the event log includes the SQLite
library version. This tests GOREST's example handler and does not exercise an
application's own routes, authorization rules, or business logic.

## MySQL HTTP runner details

Run the MySQL handler scenario from the repository root:

```sh
make integration-test-mysql-e2e
```

The runner starts a disposable MySQL container using the official `mysql:8.4`
image, creates a test database and account, runs the shared HTTP CRUD and
parameter-binding scenario, and removes the container. Set
`GOREST_IT_MYSQL_IMAGE` to choose another compatible image. The runner waits
for the image initialization to finish before connecting, prints image-pull
progress when Docker is available, and streams the test events to the terminal.
The image's initialization variables create a database-scoped test account;
they are for this disposable environment only. See the [official MySQL image
documentation](https://hub.docker.com/_/mysql/) for its initialization options.

MySQL run evidence is written to `integration_test/reports/` as a JSON Lines
event log and JSON summary. The summary records the driver, image, command,
start time, and result; the event log includes the MySQL server version. The
report does not record the database password. This tests GOREST's example
handler and does not exercise an application's own routes, authorization
rules, or business logic.

## MongoDB HTTP runner details

Run the MongoDB handler scenario from the repository root:

```sh
make integration-test-mongo-e2e
```

The runner starts a disposable MongoDB container using the official
`mongo:7.0` image, creates a uniquely named collection and a unique index on
`email`, runs HTTP CRUD and parameter-binding checks, and removes the
collection and container afterward. Set `GOREST_IT_MONGO_IMAGE` to choose
another compatible image. The test model maps its Go `ID` field to MongoDB's
`_id` through the `gorest` tag while exposing it as `id` in JSON; this matches
the ORM's MongoDB primary-key validation.

The runner defaults to MongoDB 7.0 because the Docker VM used for validation
runs a kernel version that prevents MongoDB 8.0 from starting. MongoDB documents
that versions 6.19 through 7.0.13 are affected and recommends a kernel upgrade
to 7.0.14 or later. Check the
[MongoDB 8.0 release notes](https://www.mongodb.com/docs/v8.0/release-notes/8.0/)
before overriding the image on hosts using those kernel versions.

MongoDB run evidence is written to `integration_test/reports/` as a JSON Lines
event log and JSON summary. The summary records the driver, image, command,
start time, and result; the event log includes the MongoDB server version. This
tests GOREST's example handler and does not cover an application's own routes,
authorization rules, or business logic.

## ScyllaDB HTTP runner details

Run the ScyllaDB handler scenario from the repository root:

```sh
make integration-test-scylla-e2e
```

The runner starts a disposable single-node ScyllaDB container using the
`scylladb/scylla:2025.1` image, creates a temporary keyspace and table, runs the
HTTP CRUD and parameter-binding checks, then removes the container. Set
`GOREST_IT_SCYLLA_IMAGE` to choose another compatible image. The container uses
developer mode and one CPU shard for local integration testing; this setup does
not represent a production deployment. The runner waits for the CQL service,
prints image-pull progress when Docker is available, and streams test events to
the terminal.

The test filters by the non-primary-key `email` column with `.AllowFiltering()`
and checks a duplicate full primary key for HTTP 409. ScyllaDB does not enforce
a unique constraint on `email` in this schema, so duplicate email values with
different primary keys are not treated as conflicts.

ScyllaDB run evidence is written to `integration_test/reports/` as a JSON Lines
event log and JSON summary. The summary records the driver, image, command,
start time, and result; the event log includes the server version. This tests
GOREST's example handler and does not cover an application's own routes,
authorization rules, or business logic. See the
[ScyllaDB Docker image documentation](https://hub.docker.com/r/scylladb/scylla)
for image startup options and deployment requirements.

## Oracle HTTP runner details

Run the Oracle handler scenario from the repository root:

```sh
make integration-test-oracle-e2e
```

The runner starts a disposable Oracle Database Free container, creates the
`gorest_test` application user in `FREEPDB1`, runs the same HTTP CRUD,
parameter-binding, required-field, duplicate-key, and not-found checks as the
PostgreSQL example, and removes the container afterward. It defaults to the
versioned `gvenzl/oracle-free:23.26.3-slim-faststart` image. Set
`GOREST_IT_ORACLE_IMAGE` to select another compatible image; the selected image
is recorded in the run summary. The image is maintained in the
[`gvenzl/oci-oracle-free` project](https://github.com/gvenzl/oci-oracle-free).

The Oracle server runs in Docker, but the Go test process still uses GOREST's
`godror` driver and needs Oracle Instant Client on the host. The runner detects
common Instant Client locations, including `GOREST_IT_ORACLE_LIB_DIR`,
`ORACLE_HOME`, library-path variables, and `~/oracle/instantclient*`. If it
cannot find the client library, install Oracle Instant Client or set
`GOREST_IT_ORACLE_LIB_DIR` to its directory. This container runner does not
install or bundle Oracle client libraries. The first run downloads the Oracle
image and can take several minutes to initialize. The runner prints Docker's
image-pull progress when the Docker CLI is available, then streams test events
to the terminal while also saving them to the report. Keep the command running
through image download and database startup.

Oracle run evidence is written to `integration_test/reports/` as a JSON Lines
event log and JSON summary. The summary records the image, command, start time,
and pass/fail result; the event log records the Oracle server banner and test
results. The report avoids recording the Instant Client path and database
password. As with PostgreSQL, this tests GOREST's example handler and does not
exercise an application's own routes or authorization logic.

These tests run against GOREST's public application and ORM APIs with real database
services. They are separate from the default unit-test run and require the
`integration` build tag:

```sh
go test -tags=integration ./integration_test/...
```

SQLite uses an isolated temporary directory and runs without external setup.
MySQL, PostgreSQL, SQL Server, Oracle, MongoDB, and ScyllaDB tests are skipped
when their host setting is absent. Set `GOREST_IT_REQUIRED=1` in CI to make a
missing remote-driver configuration fail instead of skip.

Each remote driver uses the following environment variables, where `DRIVER` is
`MYSQL`, `POSTGRES`, `SQLSERVER`, `ORACLE`, `MONGO`, or `SCYLLA`:

| Variable | Required | Purpose |
| --- | --- | --- |
| `GOREST_IT_<DRIVER>_HOST` | Yes | Database server hostname |
| `GOREST_IT_<DRIVER>_PORT` | No | Server port; GOREST chooses its configured default when empty |
| `GOREST_IT_<DRIVER>_NAME` | Yes | Database/schema, Mongo database, or Scylla keyspace |
| `GOREST_IT_<DRIVER>_USERNAME` | No | Database username |
| `GOREST_IT_<DRIVER>_PASSWORD` | No | Database password |
| `GOREST_IT_<DRIVER>_TLS` | No | Driver-specific GOREST TLS mode |
| `GOREST_IT_<DRIVER>_SSL_CERT` | No | Client certificate path |
| `GOREST_IT_<DRIVER>_SSL_KEY` | No | Client key path |
| `GOREST_IT_<DRIVER>_SSL_CA` | No | CA certificate or Oracle wallet path |
| `GOREST_IT_<DRIVER>_LIB_DIR` | No | Oracle Instant Client library directory |
| `GOREST_IT_<DRIVER>_TRUST_SERVER_CERTIFICATE` | No | SQL Server certificate trust setting |

The configured account must be allowed to create and drop uniquely named test
tables or collections in the selected database/keyspace. Do not point the tests
at production data. ScyllaDB's configured keyspace must already exist.

## PostgreSQL

Start a PostgreSQL service and provide the `GOREST_IT_POSTGRES_*`
environment variables listed above. To use certificate verification with
`verify-full`, run:

```sh
GOREST_IT_REQUIRED=1 \
GOREST_IT_POSTGRES_HOST=HOST_ADDRESS \
GOREST_IT_POSTGRES_PORT=HOST_PORT \
GOREST_IT_POSTGRES_NAME=DB_NAME \
GOREST_IT_POSTGRES_USERNAME=USERNAME_DB \
GOREST_IT_POSTGRES_PASSWORD=PASSWORD_DB \
GOREST_IT_POSTGRES_TLS=verify-full \
GOREST_IT_POSTGRES_SSL_CA=CA_CERT_PATH \
go test -tags=integration ./integration_test/postgres -count=1 -v
```

For `verify-full`, `GOREST_IT_POSTGRES_SSL_CA` should point to the CA
certificate that verifies the PostgreSQL server certificate, and the host
value must match a name in that server certificate. `GOREST_IT_POSTGRES_SSL_CERT`
and `GOREST_IT_POSTGRES_SSL_KEY` are client certificate and key paths; set
both only when the server requires client-certificate authentication (mTLS).
They do not replace the server CA certificate.

Set `GOREST_IT_POSTGRES_TLS` and applicable certificate variables for the TLS
mode used by your PostgreSQL service. Each test run uses one configured TLS
mode; the suite does not provision certificates or iterate over a TLS matrix.
If TLS is omitted, the database driver's configured default applies. See the
database configuration guide for driver TLS options and limitations. Keep
`GOREST_IT_REQUIRED=1` in the command to fail on missing host configuration
instead of skipping the remote database tests.

See [ORM scenarios by driver](#orm-scenarios-by-driver) for test coverage and
driver-specific differences.

The PostgreSQL RLS scenario is
`TestPostgresRowLevelSecurityEnforcesTenantPolicy` in
[`postgres/coverage_test.go`](./postgres/coverage_test.go). It checks
tenant-filtered reads for two tenants, an allowed same-tenant insert, rejection
of a cross-tenant insert, and that a transaction-local tenant setting is cleared
after commit/rollback. It limits the SQL pool to one connection and checks the
PostgreSQL backend PID to confirm the pooled connection is reused across those
transactions.

Run it with `GOREST_IT_POSTGRES_USERNAME` set to the application role you want
to validate, and use an isolated test database. The test logs PostgreSQL's
`current_user` (not its password) and requires a role that is neither a
superuser nor has the `BYPASSRLS` attribute; PostgreSQL bypasses row-security
policies for those roles, so the test explicitly skips when connected with one.
For evidence about the production application role, run the guide using that
same role in the isolated database, or document any differences in role
attributes and grants; a run with a different integration account does not
establish production-role behavior. The test creates and drops its own table,
so the configured role also needs the necessary DDL privileges in that test
database/schema.

## MySQL

Set `GOREST_IT_MYSQL_*` according to the shared environment-variable table.
The MySQL driver connection configures charset, time parsing, timezone, and
TLS using its driver-specific options. For details about configuring TLS with
a custom CA or client certificate, see the database configuration guide.
Run the suite with:

```sh
GOREST_IT_REQUIRED=1 \
GOREST_IT_MYSQL_HOST=HOST_ADDRESS \
GOREST_IT_MYSQL_PORT=3306 \
GOREST_IT_MYSQL_NAME=DB_NAME \
GOREST_IT_MYSQL_USERNAME=USERNAME_DB \
GOREST_IT_MYSQL_PASSWORD=PASSWORD_DB \
GOREST_IT_MYSQL_TLS=disable \
go test -tags=integration ./integration_test/mysql -count=1 -v
```

Configure the test account with permission to create and drop tables in the
selected database. See [ORM scenarios by driver](#orm-scenarios-by-driver)
for test coverage and driver-specific differences.

## Oracle

Set `GOREST_IT_ORACLE_NAME` to the Oracle service name. If Oracle Instant
Client libraries are not available through the system library path, set
`GOREST_IT_ORACLE_LIB_DIR` to their directory. For TCPS, configure the Oracle
wallet path using `GOREST_IT_ORACLE_SSL_CA` and set the driver TLS mode as
described in the database configuration guide.

```sh
GOREST_IT_REQUIRED=1 \
GOREST_IT_ORACLE_HOST=HOST_ADDRESS \
GOREST_IT_ORACLE_PORT=1521 \
GOREST_IT_ORACLE_NAME=SERVICE_NAME \
GOREST_IT_ORACLE_USERNAME=USERNAME_DB \
GOREST_IT_ORACLE_PASSWORD=PASSWORD_DB \
GOREST_IT_ORACLE_TLS=disable \
GOREST_IT_ORACLE_LIB_DIR=INSTANT_CLIENT_DIRECTORY \
go test -tags=integration ./integration_test/oracle -count=1 -v
```

Ensure the configured account can create and drop tables.
`GOREST_IT_ORACLE_LIB_DIR` may be omitted if Oracle Instant Client libraries
are already available through the system library path. For TCPS, change TLS
to `enable` and set `GOREST_IT_ORACLE_SSL_CA` to the Oracle wallet directory.
See [ORM scenarios by driver](#orm-scenarios-by-driver) for test coverage.

## SQL Server HTTP runner details

The SQL Server HTTP runner uses the Microsoft SQL Server 2022 Developer
container image, starts a disposable instance, runs the shared HTTP CRUD and
parameter-binding scenario, then removes the container. It defaults to
`mcr.microsoft.com/mssql/server:2022-latest`; set
`GOREST_IT_SQLSERVER_IMAGE` to choose a compatible image.

Microsoft requires acceptance of the SQL Server container license. Review the
[Microsoft SQL Server licensing terms](https://www.microsoft.com/licensing/terms/productoffering/SQLServer/all)
and explicitly opt in when starting the runner:

```sh
GOREST_IT_SQLSERVER_ACCEPT_EULA=Y make integration-test-sqlserver-e2e
```

The runner prints image-pull progress when the Docker CLI is available and
streams test events to the terminal. It waits for an authenticated `sqlcmd`
query inside the container before initializing GOREST; an open TCP port alone
does not guarantee that SQL Server has finished preparing the `sa` login. It
also writes a JSON Lines event log and JSON summary to
`integration_test/reports/`; the report records the driver, image, command,
start time, and result without recording the database password. The test uses
the `sa` account only inside the disposable container and a test-only password.

Microsoft documents SQL Server Linux containers for x86-64 Linux hosts.
Apple silicon, other ARM64 hosts, and emulated x86 containers are not validated
by this runner. SQL Server also needs sufficient memory and disk space; the
first image pull and database initialization may take several minutes. The Go
test connects with TLS disabled to this isolated local test container; do not
use that setting for a remote or production SQL Server.

This exercises GOREST's example handler and does not cover an application's
own routes, authorization rules, or business logic.

## SQL Server remote integration tests

Set the SQL Server connection variables from the shared table. Configure
`GOREST_IT_SQLSERVER_TLS` and
`GOREST_IT_SQLSERVER_TRUST_SERVER_CERTIFICATE` for the certificate policy
required by your server. For production-like verification, use a server
certificate trusted by the client rather than trusting an unverified
certificate.

```sh
GOREST_IT_REQUIRED=1 \
GOREST_IT_SQLSERVER_HOST=HOST_ADDRESS \
GOREST_IT_SQLSERVER_PORT=1433 \
GOREST_IT_SQLSERVER_NAME=DB_NAME \
GOREST_IT_SQLSERVER_USERNAME=USERNAME_DB \
GOREST_IT_SQLSERVER_PASSWORD=PASSWORD_DB \
GOREST_IT_SQLSERVER_TLS=disable \
GOREST_IT_SQLSERVER_TRUST_SERVER_CERTIFICATE=false \
go test -tags=integration ./integration_test/sqlserver -count=1 -v
```

The configured account needs permission to create and drop tables. See
[ORM scenarios by driver](#orm-scenarios-by-driver) for test coverage and
driver-specific differences.

## MongoDB

Set `GOREST_IT_MONGO_*` using the shared environment-variable table. The
configured database account must be able to create and drop collections; the
audit checks also create a unique index.

```sh
GOREST_IT_REQUIRED=1 \
GOREST_IT_MONGO_HOST=HOST_ADDRESS \
GOREST_IT_MONGO_PORT=27017 \
GOREST_IT_MONGO_NAME=DB_NAME \
GOREST_IT_MONGO_USERNAME=USERNAME_DB \
GOREST_IT_MONGO_PASSWORD=PASSWORD_DB \
GOREST_IT_MONGO_TLS=false \
go test -tags=integration ./integration_test/mongo -count=1 -v
```

See [ORM scenarios by driver](#orm-scenarios-by-driver) for MongoDB coverage
and SQL-only scenarios that do not apply. Its context check uses an
already-canceled context; it does not model cancellation of an operation
already executing on the server.

## ScyllaDB

Create the keyspace before running the tests, then set
`GOREST_IT_SCYLLA_HOST`, `GOREST_IT_SCYLLA_NAME` (keyspace), and any required
port or authentication values. The account needs permission to create and
drop tables in that keyspace.

```sh
GOREST_IT_REQUIRED=1 \
GOREST_IT_SCYLLA_HOST=HOST_ADDRESS \
GOREST_IT_SCYLLA_PORT=9042 \
GOREST_IT_SCYLLA_NAME=KEYSPACE_NAME \
GOREST_IT_SCYLLA_USERNAME=USERNAME_DB \
GOREST_IT_SCYLLA_PASSWORD=PASSWORD_DB \
go test -tags=integration ./integration_test/scylla -count=1 -v
```

Conditional Upsert is not implemented by GOREST's ScyllaDB ORM path; use
native CQL lightweight transactions when conditional writes are required.
GOREST's database TLS setting does not currently configure ScyllaDB transport
TLS. See [ORM scenarios by driver](#orm-scenarios-by-driver) for coverage and
SQL-only scenarios that do not apply.

## SQLite

SQLite tests use a temporary database directory created for each test; no
external service or connection variables are required. Run the full SQLite
integration package with:

```sh
go test -tags=integration ./integration_test/sqlite -count=1 -v
```

The package includes CRUD, transaction rollback, pagination/grouping, and the
shared ORM scenarios described below.

## ORM scenarios by driver

This section describes which ORM scenarios are exercised by each driver and
how to run the shared SQL audit subset separately. The full per-driver
commands above run the complete package, including the scenarios applicable
to that driver.

Run only the shared SQL ORM scenarios against MySQL, Oracle, SQL Server, or
SQLite with:

```sh
GOREST_IT_REQUIRED=1 go test -tags=integration ./integration_test/mysql ./integration_test/oracle ./integration_test/sqlserver -run TestORMCoverage -count=1 -v
go test -tags=integration ./integration_test/sqlite -run TestORMCoverage -count=1 -v
```

The SQL scenarios cover explicit joins, belongs-to/has-many preload mapping,
nullable results, pagination boundaries, single-statement multi-row insert
failure, live-query cancellation, representative Upsert behavior, and invalid
ORM input rejection. Upsert behavior varies by driver; the tests cover
supported ORM behavior, including MySQL's rejection of conditional Upsert,
not every constraint and dialect combination. SQL cancellation uses a
driver-specific delay mechanism and depends on server permissions and driver
cancellation behavior. The batch scenario concerns one multi-row statement,
not transactions spanning multiple statements or chunks.

PostgreSQL uses separate tests in [`postgres/coverage_test.go`](./postgres/coverage_test.go):

| Scenario | PostgreSQL test |
| --- | --- |
| Explicit JOIN and belongs-to/has-many preload mapping | `TestPostgresJoinAndRelationPreloadMapping` |
| Nullable result mapping | `TestPostgresNullableColumnMapping` |
| Pagination boundary inputs | `TestPostgresPaginationBoundaries` |
| Multi-row INSERT statement atomicity | `TestPostgresBulkInsertFailureIsStatementAtomic` |
| Live-query context cancellation | `TestPostgresContextCancellationStopsLiveQuery` |
| Primary-key, alternate-unique-key, and conditional Upsert | `TestPostgresUpsertBehaviorMatrix` |

There is currently no dedicated PostgreSQL integration test for invalid ORM
input rejection. That scenario is covered by the shared `TestORMCoverage` in
the MySQL, Oracle, SQL Server, and SQLite packages, not by the PostgreSQL
package.

MongoDB checks include nullable document mapping, alternate-unique-key Upsert,
unfiltered-write rejection, and propagation of an already-canceled context.
ScyllaDB checks include composite-primary-key Upsert validation, nullable
column mapping, unfiltered-write rejection, and propagation of an
already-canceled context. Relational joins/preloads and SQL pagination have no
direct equivalents for MongoDB or ScyllaDB. The NoSQL context checks do not
represent cancellation of a server operation already in progress.

## Application and logger integration tests

These tests use GOREST's public app and logger APIs and isolated temporary
directories; they do not require external database or Redis services. Run them
with:

```sh
go test -tags=integration ./integration_test/app ./integration_test/log -count=1
```

The application test starts `app.Start()` in a helper process, sends it
`SIGTERM`, and verifies that an in-flight HTTP request completes before the app
reports shutdown. This exercises signal handling, Fiber graceful shutdown, and
the application shutdown path together. The logger test writes through the
configured Zap/Lumberjack sink, forces size-based rotation, checks the backup
limit, and scans active and rotated files to verify recognized sensitive
values are redacted. These tests validate representative local lifecycle and
file-output behavior, not every deployment signal environment, logger sink,
filesystem, or application-specific sensitive-data pattern.

The separate unit test `TestAppCloseGracefullyShutsDownHTTPServer` in
`app_lifecycle_test.go` constructs a Fiber server directly and calls
`app.Close()`. It verifies that GOREST waits for an active request and stops
the listener, but it does not exercise the public `app.Start()` signal-waiting
path. Use the helper-process integration test to exercise that path. A test
cannot fully reproduce a consuming application's runtime:
process supervisors and containers may deliver or forward signals differently,
and real traffic, handlers, dependencies, reverse proxies, and deployment
timeouts vary by environment. Application owners should validate shutdown in
their own deployment setup and ensure the orchestrator's termination grace
period is compatible with GOREST's 30-second graceful-shutdown window.

## Redis integration tests

The Redis integration suite uses the local service at `127.0.0.1:6379` by
default and requires the service to be running. Override its connection with:

| Variable | Required | Purpose |
| --- | --- | --- |
| `GOREST_IT_REDIS_HOST` | No | Redis host; defaults to `127.0.0.1` |
| `GOREST_IT_REDIS_PORT` | No | Redis port; defaults to `6379` |
| `GOREST_IT_REDIS_USERNAME` | No | Redis ACL username |
| `GOREST_IT_REDIS_PASSWORD` | No | Redis password |
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

The Redis tests exercise app registration and shutdown, SET/GET/DEL using
unique keys, concurrent use under the configured connection-pool limit,
operation and parent-context timeouts, and initialization failure against an
unavailable local endpoint. TLS connection and certificate-validation checks
require a TLS-enabled Redis endpoint and the corresponding environment
configuration. The tests do not stop or interrupt a running Redis service;
test recovery behavior against the outage conditions relevant to your
deployment separately.

Coverage differs by database driver; the scenarios and non-equivalent NoSQL
checks are listed above. Integration tests are not a substitute for
application-handler E2E tests. Run the applicable suites against the same
database versions, schema, TLS, and permissions used by the consuming
application when validating deployment-specific behavior.
