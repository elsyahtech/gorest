# Changelog

This changelog summarizes changes recorded between GOREST's Git version tags.
Each release links to its GitHub comparison or tag so readers can inspect the
corresponding source changes. The latest release is expanded by default; older
releases can be opened as needed.

The public release history begins with `v0.0.71`. Earlier versions
(`v0.0.1`–`v0.0.70`) were development iterations and are not included here.

<details open>
<summary><strong>v1.1.8</strong> - 2026-10-31</summary>

[Compare v1.1.7 to v1.1.8](https://github.com/elsyahtech/gorest/compare/v1.1.7...v1.1.8)

### Added

- Added grouped operational endpoints at `/gorest/live`, `/gorest/ready`, and
  `/gorest/metrics`.
- Added `DisableMonitoringEndpoints` to let applications opt out of exposing
  these endpoints.
- Added human-readable uptime and readiness reporting for configured services.

</details>

<details>
<summary><strong>v1.1.7</strong> - 2026-10-31</summary>

[Compare v1.1.6 to v1.1.7](https://github.com/elsyahtech/gorest/compare/v1.1.6...v1.1.7)

### Changed

- Refined the startup banner layout for the displayed GOREST version.

</details>

<details>
<summary><strong>v1.1.6</strong> - 2026-10-31</summary>

[Compare v1.1.5 to v1.1.6](https://github.com/elsyahtech/gorest/compare/v1.1.5...v1.1.6)

### Added

- Added runtime GOREST version detection to the startup banner, including a
  development label when release metadata is unavailable.

</details>

<details>
<summary><strong>v1.1.5</strong> - 2026-10-31</summary>

[Compare v1.1.4 to v1.1.5](https://github.com/elsyahtech/gorest/compare/v1.1.4...v1.1.5)

### Changed

- Expanded the JWT filter unavailable message with guidance on where to
  configure the filter and when to access it.

</details>

<details>
<summary><strong>v1.1.4</strong> - 2026-10-31</summary>

[Compare v1.1.3 to v1.1.4](https://github.com/elsyahtech/gorest/compare/v1.1.3...v1.1.4)

### Changed

- Preserved the filter defaults when applying application configuration
  defaults.
- Clarified the JWT filter unavailable error message.

</details>

<details>
<summary><strong>v1.1.3</strong> - 2026-10-31</summary>

[Compare v1.1.2 to v1.1.3](https://github.com/elsyahtech/gorest/compare/v1.1.2...v1.1.3)

### Changed

- Refined optional JWT initialization behavior: an invalid signing method or
  token duration is logged and leaves the filter unavailable. Applications
  that do not use JWT can continue; JWT-dependent route setup still requires
  an initialized filter.

</details>

<details>
<summary><strong>v1.1.2</strong> - 2026-10-31</summary>

[Compare v1.1.1 to v1.1.2](https://github.com/elsyahtech/gorest/compare/v1.1.1...v1.1.2)

### Changed

- Registered operational routes during application startup rather than during
  application construction.
- Updated the HTTP handler adapter to use Fiber's current context interface.

</details>

<details>
<summary><strong>v1.1.1</strong> - 2026-10-31</summary>

[Compare v1.1.0 to v1.1.1](https://github.com/elsyahtech/gorest/compare/v1.1.0...v1.1.1)

### Changed

- Strengthened nil and service-availability handling across application,
  database, Redis, logger, server, and ORM adapters.
- Improved configuration and service errors with more actionable setup
  guidance.
- Expanded JWT initialization and route-registration checks, with additional
  unit and integration coverage.

</details>

<details>
<summary><strong>v1.1.0</strong> - 2026-10-31</summary>

[Compare v1.0.6 to v1.1.0](https://github.com/elsyahtech/gorest/compare/v1.0.6...v1.1.0)

### Added

- Added the optional JWT filter for issuing and validating access and refresh
  tokens, including cookie configuration and authentication middleware.
- Added filter configuration and usage documentation.
- Expanded database migration state tracking and recovery safeguards, including
  checksums and startup locking.
- Added broad integration coverage across supported database adapters.

### Changed

- Expanded application lifecycle validation, database/ORM behavior, and
  operational error reporting.
- Improved structured logging and sensitive-value redaction.

</details>

<details>
<summary><strong>v1.0.6</strong> - 2026-10-24</summary>

[Compare v1.0.5 to v1.0.6](https://github.com/elsyahtech/gorest/compare/v1.0.5...v1.0.6)

### Documentation

- Clarified database and Redis `Disable` configuration behavior and defaults.

</details>

<details>
<summary><strong>v1.0.5</strong> - 2026-10-24</summary>

[Compare v1.0.4 to v1.0.5](https://github.com/elsyahtech/gorest/compare/v1.0.4...v1.0.5)

### Changed

- Standardized optional database and Redis activation around the `Disable`
  configuration field. Its zero value (`false`) enables initialization.

</details>

<details>
<summary><strong>v1.0.4</strong> - 2026-10-24</summary>

[Compare v1.0.3 to v1.0.4](https://github.com/elsyahtech/gorest/compare/v1.0.3...v1.0.4)

### Fixed

- Preserved primary-key values when performing MongoDB upserts.

</details>

<details>
<summary><strong>v1.0.3</strong> - 2026-10-24</summary>

[Compare v1.0.2 to v1.0.3](https://github.com/elsyahtech/gorest/compare/v1.0.2...v1.0.3)

### Added

- Added application getters for the active database, server, and logger
  configurations.

</details>

<details>
<summary><strong>v1.0.2</strong> - 2026-10-21</summary>

[Compare v1.0.1 to v1.0.2](https://github.com/elsyahtech/gorest/compare/v1.0.1...v1.0.2)

### Added

- Added migration and seeder execution-state tracking with file checksums and
  startup locking to coordinate runners and support explicit recovery after an
  interrupted run.

### Changed

- Expanded migration, seeder, and database integration tests and operational
  documentation.

</details>

<details>
<summary><strong>v1.0.1</strong> - 2026-10-20</summary>

[Compare v1.0.0 to v1.0.1](https://github.com/elsyahtech/gorest/compare/v1.0.0...v1.0.1)

### Changed

- Removed OS-generated `.DS_Store` files from the repository.

</details>

<details>
<summary><strong>v1.0.0</strong> - 2026-10-20</summary>

[Compare v0.0.99 to v1.0.0](https://github.com/elsyahtech/gorest/compare/v0.0.99...v1.0.0)

### Added

- Expanded native SQL and ORM query capabilities, including additional
  filtering, grouping, ordering, joins, and return-value handling.
- Added structured logging redaction and broader database and Redis lifecycle
  coverage.
- Added integration test scenarios for supported database drivers and Redis.
- Expanded API, configuration, production validation, and operational
  documentation.

### Changed

- Strengthened application lifecycle, database access, validation, and error
  handling across the framework.

</details>

<details>
<summary><strong>v0.0.99</strong> - 2026-10-16</summary>

[Compare v0.0.75 to v0.0.99](https://github.com/elsyahtech/gorest/compare/v0.0.75...v0.0.99)

### Added

- Expanded application lifecycle and configuration management.
- Added extensive ORM query and database support, including data validation,
  safer query parameter handling, and database-specific return-value support.
- Added security guidance and expanded configuration, ORM, and querying
  documentation.

### Changed

- Improved database connection setup, migration handling, and structured
  operational errors.

</details>

<details>
<summary><strong>v0.0.75</strong> - 2026-10-13</summary>

[Compare v0.0.74 to v0.0.75](https://github.com/elsyahtech/gorest/compare/v0.0.74...v0.0.75)

### Added

- Expanded ORM update return handling to support selecting all returned
  columns.

</details>

<details>
<summary><strong>v0.0.74</strong> - 2026-10-13</summary>

[Compare v0.0.73 to v0.0.74](https://github.com/elsyahtech/gorest/compare/v0.0.73...v0.0.74)

### Added

- Added Oracle `UPDATE ... RETURNING INTO` support for ORM update operations.

</details>

<details>
<summary><strong>v0.0.73</strong> - 2026-10-13</summary>

[Compare v0.0.72 to v0.0.73](https://github.com/elsyahtech/gorest/compare/v0.0.72...v0.0.73)

### Changed

- Expanded database adapter and migration execution behavior, with
  corresponding ORM updates for SQL and ScyllaDB operations.

</details>

<details>
<summary><strong>v0.0.72</strong> - 2026-10-13</summary>

[Compare v0.0.71 to v0.0.72](https://github.com/elsyahtech/gorest/compare/v0.0.71...v0.0.72)

### Changed

- Refined application and ORM adapter behavior, including query helper
  adjustments.

</details>

<details>
<summary><strong>v0.0.71</strong> - 2026-10-12</summary>

### Initial release

- Established the initial tagged GOREST framework baseline for application
  setup, HTTP services, database access, and ORM operations.
- Documented support for Oracle, SQL Server, PostgreSQL, MySQL, SQLite,
  MongoDB, and ScyllaDB.

[View tag v0.0.71](https://github.com/elsyahtech/gorest/releases/tag/v0.0.71)

</details>
