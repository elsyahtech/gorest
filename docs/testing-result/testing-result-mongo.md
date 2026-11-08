# MongoDB

GOREST provides MongoDB connectivity through its database adapter, with native
document operations and an ORM-style API for supported queries and writes.
See the [ORM feature matrix](../guide/orm.md#7-driver-feature-matrix) and
[querying guide](../guide/querying.md) for operation details and limitations.

## Queries and transaction behavior

Use the ORM for supported CRUD and document queries. Use native MongoDB
operations through `RawQueryMongo()` for operations such as aggregation
pipelines or driver-specific options that are outside the ORM API.

GOREST's MongoDB integration coverage includes document operations, bound
values, migrations and seeders, and application lifecycle. `BeginTx()` and
`BeginTxContext()` start a native MongoDB session transaction, and ORM plus
`RawQueryMongo()` operations on the returned session are bound to it. MongoDB
transactions require a replica set or sharded cluster; a standalone deployment
does not support them. `MongoCollection()` cannot bind its returned raw handle
to the active transaction and is rejected on a transaction-bound session.

## TLS and environment configuration

Configure MongoDB TLS with the driver options in `database.Config`. Use a
trusted CA and verify the server hostname. When the server requires mTLS,
configure both the client certificate and key.

See the [database configuration guide](../configuration.md) and
[integration test guide](../../integration_test/README.md) for connection
variables and test setup.

Validate authentication, MongoDB product/version, replica-set or sharded
topology, indexes, backup/restore, and failover behavior in your environment.
