# MySQL

GOREST provides MySQL connectivity through its database adapter, with native
SQL and ORM APIs. The ORM supports common CRUD and query operations; consult
the [driver feature matrix](../guide/orm.md#7-driver-feature-matrix) for
driver-specific behavior.

## Queries and transactions

Use `QuerySQL()` and `ExecSQL()` for native SQL, or use the ORM for supported
CRUD, filters, joins, pagination, grouping, and writes. Bind data values as
parameters instead of concatenating them into SQL. SQL transactions are
available through `BeginTx()` or `BeginTxContext()`.

GOREST's MySQL integration coverage includes connection setup, ORM and native
operations, transactions, migrations, and application lifecycle.

## TLS

With the MySQL Go driver, `TLS: "true"` uses system root certificates and
verifies the hostname derived from the connection address. GOREST requires a
non-empty `SSLCAPath` to enable forwarding the TLS option, but does not load
that path as a custom CA certificate.

For a private CA or mTLS, register a TLS profile with
`mysql.RegisterTLSConfig` before initializing the database. Configure its
`RootCAs`, `ServerName`, and, for mTLS, client certificate. Set `TLS` to that
profile name and keep `SSLCAPath` non-empty. GOREST does not map
`SSLCertPath` or `SSLKeyPath` into the MySQL driver profile.

Avoid `skip-verify` and `preferred` modes where server identity must be
verified. See the [database configuration guide](../configuration.md) for
driver options and the [integration test guide](../../integration_test/README.md)
for test environment setup.

## Environment considerations

Confirm the database product and version, charset, account privileges,
certificate policy, connection limits, and availability topology for your
deployment. MySQL-compatible products may differ in TLS and SQL behavior.
