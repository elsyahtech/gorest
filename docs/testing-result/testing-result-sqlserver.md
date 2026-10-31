# SQL Server

GOREST provides SQL Server connectivity through its database adapter, with
native SQL and ORM APIs. The ORM supports common CRUD and query operations;
consult the [driver feature matrix](../guide/orm.md#7-driver-feature-matrix)
for driver-specific behavior.

## Queries and transactions

Use `QuerySQL()` and `ExecSQL()` for native SQL, or use the ORM for supported
CRUD, filters, joins, pagination, grouping, and writes. Bind data values as
parameters instead of concatenating them into SQL. SQL transactions are
available through `BeginTx()` or `BeginTxContext()`.

GOREST's SQL Server integration coverage includes database operations,
transactions, migrations, and application lifecycle.

## TLS

Configure `TLS` as `mandatory` or `strict` and keep
`TrustServerCertificate` set to `false` when the client should validate the
server certificate. The certificate name must match the hostname used by
GOREST, and its issuing CA must be trusted by the application host.

Strict encryption uses TDS 8.0 and requires SQL Server 2022 or later. See
Microsoft's guidance for [SQL Server on Linux TLS
certificates](https://learn.microsoft.com/en-us/sql/linux/security/encrypted-connections)
and [strict encryption](https://learn.microsoft.com/en-us/sql/relational-databases/security/networking/connect-with-strict-encryption).

See the [database configuration guide](../configuration.md) and
[integration test guide](../../integration_test/README.md) for configuration
and test setup.

## Environment considerations

Check the SQL Server edition, version, host architecture, authentication mode,
TLS policy, connection limits, and availability topology used by your
deployment.
