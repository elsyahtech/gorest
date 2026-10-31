# PostgreSQL

GOREST provides PostgreSQL connectivity through its database adapter, with
both native SQL and ORM APIs. The ORM supports common CRUD and query
operations; see the [driver feature matrix](../guide/orm.md#7-driver-feature-matrix)
for operation-level details.

## Queries and transactions

Use `QuerySQL()` and `ExecSQL()` for native SQL, or use the ORM for supported
CRUD, filters, joins, pagination, grouping, and writes. Bind data values as
parameters rather than concatenating them into SQL. SQL transactions are
available through `BeginTx()` or `BeginTxContext()`.

The PostgreSQL integration suite exercises database operations, bound
parameters, transaction behavior, migrations, and application lifecycle.
Row-level security depends on PostgreSQL roles and policies: test it using a
dedicated non-superuser role that does not have `BYPASSRLS`.

## TLS

For network connections, configure a TLS mode in `database.Config`. Prefer
`verify-full` with a trusted CA and a hostname that matches the server
certificate. When client-certificate authentication is required, provide the
client certificate and key along with the server trust configuration.

See the [database configuration guide](../configuration.md) for the available
fields and the [integration test guide](../../integration_test/README.md) for
test environment setup.

## Environment considerations

Check PostgreSQL version, enabled extensions, role privileges, RLS policy,
certificate requirements, connection limits, and failover behavior against
your own deployment. Integration tests exercise GOREST's adapter and do not
configure those application or infrastructure policies for you.
