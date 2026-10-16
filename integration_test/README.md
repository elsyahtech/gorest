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
| `GOREST_IT_<DRIVER>_NAME` | Yes | Database/schema, Mongo database, or Scylla keyspace |
| `GOREST_IT_<DRIVER>_USERNAME` | No | Database username |
| `GOREST_IT_<DRIVER>_PASSWORD` | No | Database password |
| `GOREST_IT_<DRIVER>_PORT` | No | Server port; GOREST chooses its configured default when empty |
| `GOREST_IT_<DRIVER>_TLS` | No | Driver-specific GOREST TLS mode |
| `GOREST_IT_<DRIVER>_SSL_CERT` | No | Client certificate path |
| `GOREST_IT_<DRIVER>_SSL_KEY` | No | Client key path |
| `GOREST_IT_<DRIVER>_SSL_CA` | No | CA certificate or Oracle wallet path |
| `GOREST_IT_<DRIVER>_LIB_DIR` | No | Oracle Instant Client library directory |
| `GOREST_IT_<DRIVER>_TRUST_SERVER_CERTIFICATE` | No | SQL Server certificate trust setting |

The configured account must be allowed to create and drop uniquely named test
tables or collections in the selected database/keyspace. Do not point the tests
at production data. ScyllaDB's configured keyspace must already exist.

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
path. Use the helper-process integration test above as the evidence for that
path. Even that test cannot fully reproduce a consuming application's runtime:
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
go test -tags=integration ./integration_test/redis -count=1
```

The Redis tests exercise app registration and shutdown, SET/GET/DEL using
unique keys, concurrent use under the configured connection-pool limit,
operation and parent-context timeouts, and initialization failure against an
unavailable local endpoint. TLS connection and certificate-validation checks
require a TLS-enabled Redis endpoint and the corresponding environment
configuration. The tests do not stop or interrupt a running Redis service, so
they do not prove recovery during a prolonged post-startup outage.

Current coverage exercises CRUD, adversarial values passed through ORM
bindings, SQL placeholder parsing in a condition containing a quoted question
mark and comments, basic Upsert paths, SQL transaction rollback, guarded
unfiltered SQL writes, and basic SQL pagination/grouping. The suite does not yet
cover every audit scenario (such as joins, relation/preload mapping, nullable
mapping, pagination boundary/overflow cases, batch failure atomicity, context
cancellation, or all driver-specific Upsert behavior), nor does it replace
application-handler E2E tests. Run the applicable tests against the same
database versions, schema, TLS, and permissions used by the consuming
application before treating those behaviors as verified.
