# Migration compensation examples

Every migration can have one matching compensation file in the same driver directory. Keep the basename identical and insert `.down` before the file extension:

| Up migration | Compensation |
| --- | --- |
| `001_example.sql` | `001_example.down.sql` |
| `001_example.cql` | `001_example.down.cql` |
| `001_example.json` | `001_example.down.json` |

GOREST ignores `.down` files during normal execution. On startup-task failure, it attempts compensation in reverse execution order after rolling back the failed SQL transaction when the driver can reliably do so. A compensation file must be valid for the driver's native engine; GOREST does not translate or infer inverse operations.

## Driver examples

Each folder contains the up migrations and matching compensation examples for that driver:

- [MongoDB](./mongo/): `.json` up files; `.down.json` uses `dropCollections` and/or `dropIndexes`.
- [MySQL](./mysql/): `.sql` up and down files.
- [Oracle](./oracle/): `.sql` up and down files.
- [PostgreSQL](./postgres/): `.sql` up and down files.
- [ScyllaDB](./scylla/): `.cql` up and down files.
- [SQLite](./sqlite/): `.sql` up and down files.
- [SQL Server](./sqlserver/): `.sql` up and down files.

MongoDB migration compensation JSON accepts this shape:

```json
{
  "dropCollections": ["users"],
  "dropIndexes": [
    { "collection": "accounts", "name": "email_unique" }
  ]
}
```

At least one action must be present. Indexes are dropped before collections. Dropping a collection also deletes its documents, so only do this when the migration owns that collection and its data. Prefer named `dropIndexes` actions when undoing indexes on a pre-existing collection.

Treat compensation as an explicit, potentially destructive database operation: verify its effect and scope for the target environment. MySQL and Oracle may commit DDL implicitly, and other engine/driver behavior can also limit atomic rollback. If the database is unavailable or compensation fails, GOREST reports incomplete rollback and requires operator reconciliation rather than claiming success.
