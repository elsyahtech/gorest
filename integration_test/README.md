# ORM integration tests

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
