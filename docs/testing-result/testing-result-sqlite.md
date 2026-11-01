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
`DatabaseDirectory` and `Name` to select the database file location. When
GOREST creates a new database directory, it requests mode `0700`; when it
creates a new database file, it requests mode `0600`. On POSIX systems, these
modes restrict access to the application process owner, subject to the
operating system and umask.

GOREST does not change permissions or ownership of an existing database file
or directory. Ensure their access controls are appropriately restrictive in
the deployment environment. These defaults do not replace operating-system
access controls, and processes running as the same user as the application
can still access the database. Plan disk capacity and backups for the
application's data lifecycle.

SQLite is suited to local or single-node application deployments. It does not
provide a shared remote database service for horizontally scaled instances.
See the [database configuration guide](../configuration.md) and
[integration test guide](../../integration_test/README.md) for setup details.
