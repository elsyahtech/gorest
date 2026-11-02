# GOREST features and deployment guide

GOREST provides database adapters, ORM and native query APIs, Redis
integration, request filters, structured logging, and application health
endpoints. Automated unit and integration tests exercise these framework
features. Use the guides below to configure them for your database products,
security policies, and deployment topology.

## Database support

GOREST supports seven database drivers. The ORM uses a common API where
possible, while native query APIs provide access to database-specific
operations.

| Database | Data model | Native access | ORM |
|---|---|---|---|
| PostgreSQL | Relational SQL | `QuerySQL`, `ExecSQL`, and transaction-aware native SQL | CRUD, filters, joins, transactions, and the operations in the [ORM feature matrix](guide/orm.md#7-driver-feature-matrix) |
| MySQL | Relational SQL | `QuerySQL`, `ExecSQL`, and transaction-aware native SQL | CRUD, filters, joins, transactions, and the operations in the [ORM feature matrix](guide/orm.md#7-driver-feature-matrix) |
| SQL Server | Relational SQL | `QuerySQL`, `ExecSQL`, and transaction-aware native SQL | CRUD, filters, joins, transactions, and the operations in the [ORM feature matrix](guide/orm.md#7-driver-feature-matrix) |
| Oracle | Relational SQL | `QuerySQL`, `ExecSQL`, and transaction-aware native SQL | CRUD, filters, joins, transactions, and the operations in the [ORM feature matrix](guide/orm.md#7-driver-feature-matrix) |
| SQLite | Embedded SQL | `QuerySQL`, `ExecSQL`, and transaction-aware native SQL | CRUD, filters, joins, transactions, and the operations in the [ORM feature matrix](guide/orm.md#7-driver-feature-matrix) |
| MongoDB | Documents | Native MongoDB operations and aggregation through the configured driver | CRUD and document queries through GOREST's ORM API |
| ScyllaDB | Wide-column/CQL | Native CQL through the configured driver | CRUD and supported CQL query patterns through GOREST's ORM API |

Use the ORM when its supported operations fit the query. Use native access for
database-specific syntax and operations that need direct driver control.
Review the [ORM feature matrix](guide/orm.md#7-driver-feature-matrix) for
driver-specific limits and the [querying guide](guide/querying.md) for API
selection, parameter binding, and resource handling.

SQL transaction APIs are available for the relational drivers. GOREST's
`BeginTx`, `Commit`, and `Rollback` methods do not provide transaction
atomicity for MongoDB or ScyllaDB; use native database transaction features
where available and follow their database-specific constraints.

See the configuration guides for [PostgreSQL](testing-result/testing-result-postgres.md),
[MySQL](testing-result/testing-result-mysql.md),
[SQL Server](testing-result/testing-result-sqlserver.md),
[Oracle](testing-result/testing-result-oracle.md),
[SQLite](testing-result/testing-result-sqlite.md),
[MongoDB](testing-result/testing-result-mongo.md), and
[ScyllaDB](testing-result/testing-result-scylla.md).

## Other framework features

| Feature | What GOREST provides | Guidance |
|---|---|---|
| Migrations and seeders | Startup tasks for supported database drivers, migration tracking, and repeat-run handling | Use a dedicated migration account where practical; test schema changes against a disposable or staging database. See the [migration and seeder guide](guide.md#migration-seeder). |
| Redis | A Redis client integration with connection configuration, TLS options, and application dependency/readiness behavior | Decide whether Redis is required or optional, set explicit timeouts, and configure TLS and certificates according to your Redis service. See the [Redis guide](testing-result/testing-result-redis.md) and [configuration guide](configuration.md). |
| Filter | Optional JWT access/refresh token creation and authentication middleware | Set a strong secret outside source control, enable secure cookies over HTTPS, and implement account authorization in application code. See the [filter guide](guide/filter.md). |
| Logger | Structured application logging, rotating file output, shutdown handling, and heuristic redaction of recognized sensitive values | Avoid logging secrets and personal data; redaction is a safeguard, not a guarantee. See the [logger guide](guide/logger.md). |
| Health and metrics | Liveness, readiness, runtime metrics, and a driver-specific estimate of configured database storage usage | Restrict endpoint access and treat storage values as point-in-time estimates, not free disk or maximum capacity. Connect probes and metrics scraping to the policy used by your orchestrator or monitoring system. See [operational endpoints](configuration.md#operational-endpoints-and-environment-configuration). |

GOREST does not provide a built-in file upload/storage service or background
job queue. Choose application-level or third-party components for those
requirements.

These features are also exercised by GOREST's automated tests. The
[integration test guide](../integration_test/README.md) describes how to run
those tests and the local or service prerequisites.

## Configure for your environment

1. Select the GOREST driver and configure its host, port, database or service
   name, credentials, connection pool, and timeout. Load credentials from
   environment-specific protected configuration or a secrets manager.
2. Set TLS explicitly for network services. Validate the server certificate
   and hostname; configure a client certificate when your service requires
   mTLS. Oracle TCPS uses an Oracle wallet and Oracle Instant Client. MySQL
   custom CA or mTLS setup uses a registered driver TLS profile. SQLite is
   embedded and does not use network TLS.
3. Use a least-privilege application account. Keep schema changes and
   administrative work separate from normal application access when your
   deployment supports it.
4. Run unit and relevant integration tests with the same GOREST configuration
   and database product/version intended for deployment. Use a disposable
   database or schema for tests that create or modify data.
5. Validate application-specific authorization, startup and migration
   behavior, readiness, recovery, backups, capacity, and high-availability
   behavior in a representative non-production environment.

For general connection setup, TLS options, and operational endpoints, see the
[configuration guide](configuration.md). These steps help evaluate GOREST
with a particular environment; they do not replace application or
infrastructure-specific qualification.
