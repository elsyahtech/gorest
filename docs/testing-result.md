# Database and framework guides

GOREST supports seven database drivers and provides shared ORM, native query,
application lifecycle, Redis, filter, and logger features. These guides
explain how to use and configure those capabilities. They are not a record of
individual test runs or a compatibility guarantee for every server version.

## Database drivers

| Driver | Guide |
|---|---|
| PostgreSQL | [Connection, SQL, ORM, and TLS configuration](./testing-result/testing-result-postgres.md) |
| MySQL | [Connection, SQL, ORM, and TLS configuration](./testing-result/testing-result-mysql.md) |
| SQL Server | [Connection, SQL, ORM, and TLS configuration](./testing-result/testing-result-sqlserver.md) |
| Oracle | [Connection, SQL, ORM, and wallet configuration](./testing-result/testing-result-oracle.md) |
| SQLite | [Embedded database usage and configuration](./testing-result/testing-result-sqlite.md) |
| MongoDB | [Document access, ORM, and TLS configuration](./testing-result/testing-result-mongo.md) |
| ScyllaDB | [CQL access, ORM, and TLS configuration](./testing-result/testing-result-scylla.md) |
| Redis | [Redis integration and TLS configuration](./testing-result/testing-result-redis.md) |

## Shared framework guides

- [Application architecture](./architecture.md) — package roles and
  application lifecycle.
- [ORM feature reference](./guide/orm.md) — common query API and per-driver
  capability matrix.
- [ORM or native queries](./guide/querying.md) — choosing an access style,
  parameter binding, transaction behavior, and resource handling.
- [Database configuration](./configuration.md) — driver settings, TLS,
  connection pools, health endpoints, and environment configuration.
- [Migrations and seeders](./guide.md#migration-seeder) — managing startup
  schema and seed tasks.
- [Filter](./guide/filter.md) — JWT cookies and request authentication.
- [Helpers](./guide/helpers.md) — request-body parsing, conversions, and
  hashing utilities.
- [Logger](./guide/logger.md) — structured logs, rotation, and redaction
  behavior.
- [Response handling](./guide/response/view_response.md) — Fiber responses and
  the optional JSON envelope.
- [API reference](./api-reference.md) — generated Go package documentation.
- [Versioning](./versioning.md) — selecting and reviewing module versions.
- [Support](./support.md) — issue reporting and help expectations.

GOREST's automated tests exercise framework and driver behavior. Application
authorization, business rules, server compatibility, certificate lifecycle,
backups, failover, and workload capacity depend on the consuming application
and deployment; validate them in the target environment.

To run the unit and integration suites, including their prerequisites and
environment variables, see the [integration test guide](../integration_test/README.md).
