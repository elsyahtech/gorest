# Seeder compensation examples

Every seeder can have one matching compensation file in the same driver directory. Keep the basename identical and insert `.down` before the file extension:

| Up seeder | Compensation |
| --- | --- |
| `001_example.sql` | `001_example.down.sql` |
| `001_example.cql` | `001_example.down.cql` |
| `001_example.json` | `001_example.down.json` |

GOREST ignores `.down` files during normal execution and attempts them in reverse execution order if a startup seeder batch fails. SQL and CQL files contain native delete statements. Make each predicate identify only records inserted by its matching seeder; broad predicates can remove unrelated application data.

## Driver examples

Each folder contains the up seeders and matching compensation examples for that driver:

- [MongoDB](./mongo/): `.json` up files; `.down.json` requires a collection and non-empty `delete` filters.
- [MySQL](./mysql/): `.sql` up and down files.
- [Oracle](./oracle/): `.sql` up and down files.
- [PostgreSQL](./postgres/): `.sql` up and down files.
- [ScyllaDB](./scylla/): `.cql` up and down files.
- [SQLite](./sqlite/): `.sql` up and down files.
- [SQL Server](./sqlserver/): `.sql` up and down files.

MongoDB seeder compensation JSON accepts this shape:

```json
{
  "collection": "users",
  "delete": [
    { "id": "seeded-record-id-1" },
    { "id": "seeded-record-id-2" }
  ]
}
```

Each filter is passed to MongoDB `DeleteMany`. Use unique, immutable identifiers for the inserted records; never use an empty or non-unique filter. GOREST does not infer which records a partially failed insert created.

Compensation is best-effort, not a guarantee of atomicity. A database outage, permission failure, timeout, or engine-specific transaction behavior may prevent cleanup. In that case GOREST reports incomplete rollback and requires operator inspection/reconciliation.
