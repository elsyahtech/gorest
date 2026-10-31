# GOREST Documentation

This index organizes GOREST documentation by topic. The [root
README](../README.md) introduces the framework and provides the installation
quick start.

## Start here

- [Install GOREST and run a minimal application](../README.md#quick-start)
- [Understand GOREST components and lifecycle](./architecture.md)

## Learn and configure

### Configure the application

- [Configuration guide](./configuration.md) — application and subsystem
  settings for logging, HTTP, Redis, and databases.
- [Logger guide](./guide/logger.md) — log output, rotation, and redaction.
- [Server and router guide](./guide/router.md) — route registration and HTTP
  middleware.
- [Application architecture](./architecture.md) — package responsibilities,
  startup, request flow, and framework scope.

### Build application features

- [JWT filter guide](./guide/filter.md) — token issuance, claims, and filter
  responsibilities.
- [Helper package guide](./guide/helpers.md) — body parsing, conversions,
  and hash helpers.
- [Response guide](./guide/response/view_response.md) — Fiber responses and
  the optional JSON response envelope.

### Work with databases

For ORM-based data access, use this sequence:

1. [Querying guide](./guide/querying.md) — choose ORM or native APIs.
2. [ORM tutorial](./guide/orm-tutorial.md) — follow a complete CRUD example.
3. [ORM reference](./guide/orm.md) — look up operations, relationships, and
   driver support.

Related references:

- [Database query overview](./guide/database.md)
- [Database configuration](./configuration.md#4-database-configuration-databaserun)
- [Schema examples](./guide/database/schema.md)
- [Migration and seeder guide](./guide.md#migration-seeder) — execution,
  coordination, and recovery.
- [Migration compensation examples](../database/migrations/README.md) and
  [seeder compensation examples](../database/seeders/README.md)

### Operate and troubleshoot

- [GOREST guide](./guide.md) — application lifecycle, startup behavior,
  migrations and seeders, entity mapping, and response handling.
- [Troubleshooting guide](./troubleshoot.md) — diagnose common errors and find
  driver-specific references.
- [Support guide](./support.md) — prepare a question or issue report.

### Security, testing, and operations

- [OWASP ASVS mapping](./security/asvs-matrix.md) — framework scope and
  application/deployment responsibilities.
- [Security policy](../SECURITY.md) — privately report suspected
  vulnerabilities.
- [GOREST features and deployment guide](./production-validation.md) —
  database, ORM, native query, Redis, filter, logger, and deployment guidance.
- [Database and framework guides](./testing-result.md) — configuration and
  usage guidance by driver and shared framework feature.
- [Integration test guide](../integration_test/README.md) — run framework
  integration suites.
- [API reference](./api-reference.md) — exported Go packages and symbols.
- [Versions and change history](./versioning.md) — Go module versions and
  upgrade guidance.

### Contribute

- [Contribution guide](../CONTRIBUTING.md)

## How to read the documentation

- **Guides** explain how to use a feature. Use the database sequence above
  when learning the ORM; use the reference when looking up a specific API.
- **Configuration** documents settings and defaults. Registering a
  configuration does not by itself mean an optional subsystem is enabled;
  follow the setup steps in the relevant guide.
- **Testing instructions** in the integration guide explain how to run
  framework suites; passing those tests does not qualify every application or
  deployment.
- **Security documentation** distinguishes framework controls from
  authentication, authorization, and deployment responsibilities that belong
  to the application.
