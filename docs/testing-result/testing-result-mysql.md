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

With the MySQL Go driver, `TLS: "true"` enables TLS using system root
certificates and verifies the hostname derived from the connection address.
No CA path is needed when the server certificate chains to a trusted system
root. The connection fails verification if the certificate is invalid, the
hostname does not match, or its issuing CA is not trusted.

For a private CA, set `SSLCAPath` while `TLS` is `"true"`; GOREST loads the PEM
file as the trust pool and keeps hostname verification enabled. For mTLS, set
both `SSLCertPath` and `SSLKeyPath`, also with `TLS: "true"`. These file paths
are ignored when `TLS` names a registered MySQL TLS profile. For other custom
TLS settings, register a profile with `mysql.RegisterTLSConfig` and set `TLS`
to that profile name.

Avoid `skip-verify` and `preferred` modes where server identity must be
verified. See the [database configuration guide](../configuration.md) for
driver options and the [integration test guide](../../integration_test/README.md)
for test environment setup.

## Environment considerations

Confirm the database product and version, charset, account privileges,
certificate policy, connection limits, and availability topology for your
deployment. MySQL-compatible products may differ in TLS and SQL behavior.
