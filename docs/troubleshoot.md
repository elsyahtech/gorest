# Troubleshooting Guide

This guide is the starting point for diagnosing GOREST application and database failures. Follow the symptom-to-check-to-action steps below, then use the linked feature guides for driver-specific details.

## How to read an operation error

For ORM operations, inspect `HTTPCode`, `Message`, and `Error`:

- `HTTPCode` is the suggested HTTP status for the operation result.
- `Message` describes what to check or do next.
- `Error` contains the underlying cause for logs and diagnosis.

Log internal errors on the server. Map them to a stable, endpoint-appropriate message before returning a response; raw database errors can expose schema, table, column, or constraint details. Native database methods return similar diagnostic values, but the application is responsible for operation-specific classification and public response handling.

## Common database and ORM failures

| Symptom or status | What to check | Next step |
| --- | --- | --- |
| Application fails during startup | Driver, connection settings, credentials, network access, and whether the database is ready. | Correct the configuration or database availability, then restart. If migration or seeding failed, inspect the database for partial changes before retrying. |
| Migration/seeder reports an active or unfinished run | Another current GOREST runner may hold the startup lock, or a previous run may have stopped before completion. | Stop migration runners; inspect the affected schema/data, migration history, and `gorest_migration_state`. Reconcile the recorded state with the database before manually clearing the lock. Never clear it blindly. |
| `400 Bad Request` from ORM | The request or clause failed ORM validation, such as an empty `Select()`, missing required value/key, unsupported clause syntax, or invalid operation shape. | Read `Message`, correct the input or use the documented clause grammar, then repeat with a minimal request. For complex database-specific expressions, use the native API with bound values. |
| `404 Not Found` for a single record | A single-struct `Find()`, or an update/delete that requires an existing target, did not match a record. | Verify the active database/table, key value and type, and filter. Retry with a known existing key. A slice `Find()` with no matches is normally an empty successful result, not a 404. |
| `409 Conflict` on a write | A primary-key or unique constraint rejected the submitted value. | Check the conflicting key in the database and confirm that the intended `Upsert()` conflict target matches the relevant constraint. Do not retry unchanged data as a plain create. |
| `503 Service Unavailable` | The requested database service may not be initialized or reachable. | Check startup logs, driver configuration, service health, DNS/network access, and credentials; retry only after connectivity is restored. |
| `504 Gateway Timeout` | The database operation exceeded its configured deadline. | Check database health and query duration, review indexes/query plans, and set a suitable context/configuration timeout. Avoid blindly increasing timeouts without investigating the query. |
| `500 Internal Server Error` from a database operation | Common causes include schema/model mismatch, missing table/column, permissions, conversion errors, or an unclassified driver failure. | Use server logs and the wrapped `Error` to find the driver detail; compare schema, `gorest` tags, selected aliases, and Go field types. Keep raw driver details out of public responses. |
| Placeholder count or binding error | The number or syntax of bound arguments does not match the query. | Use placeholders for values and supply one argument per active placeholder. Keep request values out of SQL/CQL strings; use a native query when the ORM's documented `Where()` grammar is insufficient. |
| Selected fields are empty or have zero values | A projection may omit those fields, or result aliases/tags/types may not match. | Compare the selected columns and aliases with destination `gorest` tags and Go types. Test with `Select("*")` to distinguish projection issues from mapping issues. |
| ScyllaDB rejects `GROUP BY`, `DISTINCT`, `ORDER BY`, or another clause | CQL applies key- and query-shape restrictions; `.AllowFiltering()` does not remove every restriction. Some ORM clauses are unsupported for ScyllaDB. | Check the table's partition/clustering key and the specific clause requirements; change the query shape or use an appropriate native CQL strategy. |
| MongoDB query returns no data or mapping fails | Database/collection, BSON field names/types, or destination tags may differ. MongoDB's `_id` is distinct from an application field named `id`. | Inspect a stored document and compare its BSON fields and types with the model's `gorest` tags; verify the selected database and collection. |
| NoSQL sequence appears to commit or roll back | GOREST `BeginTx()`, `Commit()`, and `Rollback()` are no-ops for MongoDB and ScyllaDB; they do not provide transaction atomicity. | Use the database's native transaction/conditional-write API where supported, or redesign the operation so it does not rely on a multi-operation transaction. |
| Native query returns scan/decode or resource errors | Native APIs do not map rows/documents into a model automatically. | Match the number and order of SQL scan destinations to selected columns; decode MongoDB documents using the provided helper or explicit BSON tags; close rows/cursors/iterators and check iteration errors. |

## Verify the diagnosis

1. Reproduce the issue with the smallest request and record the driver, operation, returned status, and server-side error.
2. Confirm that the application is connected to the intended database, schema, and table/collection.
3. Compare the query projection and predicates with the model tags, database schema, field types, and constraints.
4. Change one cause at a time, rerun the operation, and verify persisted state as well as the returned response.
5. For writes that failed partway through, inspect database state before retrying; a returned error does not guarantee that every driver or script rolled back prior changes.

## Detailed references

- [ORM troubleshooting, shared error contract, driver references, and clause diagnostics](./guide/orm.md#8-troubleshooting)
- [Native database querying, parameter binding, resource cleanup, and transaction behavior](./guide/querying.md)
- [Database migration and seeder startup, coordination, and recovery](./guide.md#4-database-migration--seeder)
- [Security vulnerability reporting](../SECURITY.md)

If the issue is not covered here or in the linked guides, [submit an issue or bug report](https://github.com/elsyahtech/gorest/issues) with the GOREST version, database driver and version, a minimal reproduction, and sanitized error details. Do not include credentials, personal data, or secrets. Report suspected security vulnerabilities through the private process in [SECURITY.md](../SECURITY.md), not in a public issue.

