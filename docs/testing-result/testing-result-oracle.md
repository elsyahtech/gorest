# Oracle

GOREST provides Oracle connectivity through its database adapter, with native
SQL and ORM APIs. The ORM supports common CRUD and query operations; consult
the [driver feature matrix](../guide/orm.md#7-driver-feature-matrix) for
driver-specific behavior.

## Queries and transactions

Use `QuerySQL()` and `ExecSQL()` for native SQL, or use the ORM for supported
CRUD, filters, joins, pagination, grouping, and writes. Bind data values as
parameters instead of concatenating them into SQL. SQL transactions are
available through `BeginTx()` or `BeginTxContext()`.

Oracle integration coverage exercises database operations, migrations,
transactions, and application lifecycle. Repository migrations and seeders
are available under the Oracle-specific database directories.

## Oracle client and TLS

GOREST connects through `godror` and Oracle Client libraries. Install a
compatible Oracle Instant Client on the application host and configure
`LibDir` when it is not in the system library path.

For encrypted Oracle Net connections, set `TLS: "enable"` and configure
`SSLCAPath` to the Oracle wallet directory (`TNS_ADMIN`). The wallet and
Oracle Net configuration must be compatible with the target listener and
Oracle Client. Server certificate and hostname validation should remain
enabled. Oracle uses TCPS and wallet configuration; it is not configured by
passing generic PEM file paths through `SSLCertPath` and `SSLKeyPath`.

See the [database configuration guide](../configuration.md) for GOREST
settings and the [integration test guide](../../integration_test/README.md)
for local test prerequisites.

## Environment considerations

Validate the Oracle edition and version, client-library compatibility, wallet
contents, service name, privileges, listener setup, and any RAC or Data Guard
behavior in your environment. Use a disposable schema and dedicated test
account for application integration.
