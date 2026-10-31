# SQLite

SQLite is an embedded database supported by GOREST's database adapter. It
provides native SQL and ORM APIs without a separate database service or
network connection.

## Queries and transactions

Use `QuerySQL()` and `ExecSQL()` for native SQL, or use the ORM for supported
CRUD, filters, joins, pagination, grouping, and writes. Bind values as SQL
parameters. GOREST's SQLite integration coverage includes database
operations, transactions, migrations, seeders, and application lifecycle.

## Storage and deployment

SQLite stores data in a local file and does not use network TLS. Use
`DatabaseDirectory` and `Name` to select the database file location and keep
file permissions restrictive. Plan disk capacity and backups for the
application's data lifecycle.

SQLite is suited to local or single-node application deployments. It does not
provide a shared remote database service for horizontally scaled instances.
See the [database configuration guide](../configuration.md) and
[integration test guide](../../integration_test/README.md) for setup details.
