# Integration tests

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

## SQL Server

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
