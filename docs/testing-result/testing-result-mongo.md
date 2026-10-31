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
values, migrations and seeders, and application lifecycle. GOREST's generic
`BeginTx()`, `Commit()`, and `Rollback()` methods do not start a MongoDB
transaction. For transactions, use the MongoDB driver's native session
transaction API and follow its replica-set or sharded-cluster requirements.

## TLS and environment configuration

Configure MongoDB TLS with the driver options in `database.Config`. Use a
trusted CA and verify the server hostname. When the server requires mTLS,
configure both the client certificate and key.

See the [database configuration guide](../configuration.md) and
[integration test guide](../../integration_test/README.md) for connection
variables and test setup.

Validate authentication, MongoDB product/version, replica-set or sharded
topology, indexes, backup/restore, and failover behavior in your environment.
