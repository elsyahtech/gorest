# ScyllaDB

GOREST provides a ScyllaDB adapter for supported CQL operations, with native
CQL access and an ORM-style API. See the [ORM feature
matrix](../guide/orm.md#7-driver-feature-matrix) and
[querying guide](../guide/querying.md) for supported query patterns and
constraints.

## Queries and transaction behavior

Use the ORM for supported CRUD and CQL query patterns. Use
`RawQueryScylla()` for native CQL operations and driver-specific features.
GOREST's integration coverage includes CQL operations, migrations and
seeders, and application lifecycle.

GOREST's generic `BeginTx()`, `Commit()`, and `Rollback()` methods do not
provide a ScyllaDB transaction boundary. ScyllaDB does not provide a general
transaction across arbitrary operations; use native conditional statements
and lightweight transactions where appropriate, following ScyllaDB's
partition and statement constraints.

## TLS

Enable ScyllaDB TLS with `TLS: "enable"`. Configure `SSLCAPath` with the CA
used to verify the server certificate and hostname. If the service requires
client-certificate authentication, configure both `SSLCertPath` and
`SSLKeyPath`.

See the [database configuration guide](../configuration.md) and
[integration test guide](../../integration_test/README.md) for connection
settings and test setup.

Validate consistency requirements, cluster topology, failover, backup and
restore, and capacity against your environment.
