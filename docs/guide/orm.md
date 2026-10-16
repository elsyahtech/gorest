# ORM Guide

Gorest ORM provides a chainable API for SQL databases, MongoDB, and ScyllaDB. This guide describes the behavior implemented by the ORM and calls out driver differences so you can choose the right query shape.

Supported drivers:

- SQL: MySQL, PostgreSQL, SQL Server, Oracle, SQLite
- Document: MongoDB
- Wide-column: ScyllaDB

For a neutral comparison of ORM and native query APIs, see the [Querying Guide](querying.md). For database connection and migration setup, see [Database Guide](database.md). For the complete struct-tag reference, see [the main guide](../guide.md#5-defining-entities--struct).

New to the ORM? Follow the sequential [ORM Tutorial](orm-tutorial.md) from defining a model through reads, writes, failure cases, and troubleshooting. Use this page as the feature reference.

## 1. Start a query

`Table()` starts a new ORM chain from the application's `DBSession`. The table name is a SQL table name or MongoDB collection name; Scylla uses the configured keyspace and table.

```go
var users []User

query := app.Database().
	Table("users").
	Where("department_id = ?", departmentID).
	OrderBy("email", "ASC").
	Limit(20).
	Find(&users)

if query.Error != nil {
	return query.Message, query.HTTPCode, query.Error
}
```

`Table()` and chain methods such as `Where()`, `Select()`, `OrderBy()`, `Limit()`, `Offset()`, `GroupBy()`, and `Having()` configure the query; they do not fetch or write records. A query runs when a terminal operation is called: `Find()`, `Create()`, `Update()`, or `Delete()`. `BeginTx()` is an exception: it starts the SQL transaction immediately and returns a session for the subsequent chains.

Terminal operations return the ORM object. Read `Error`, `Message`, and `HTTPCode` from that object; `Result`, `RowsAffected`, and `LastInsertId` depend on the operation and driver. `Table()` creates a fresh ORM chain, so build a new chain for each independent query.

### Transactions

SQL transactions are started from a `DBSession`. `BeginTx()` returns a new session bound to the transaction. Use only that session for the transaction's ORM calls. This example creates a user, updates that user, rolls back if either write fails, and returns a commit error instead of reporting success:

`BeginTx()` uses the timeout configured in `database.Config.Timeout` without requiring each handler to create another timeout context. To also bind the transaction to the current request's cancellation and deadline, call `BeginTxContext(ctx)`; the earlier of the request deadline and configured database timeout applies. The transaction context remains active until `Commit()` or `Rollback()`, and cancellation before then causes the SQL transaction to be rolled back by `database/sql`.

```go
// The surrounding handler imports "fmt" and gorest; the verification test below also uses "net/http" and "testing".

txSession, message, httpCode, err := app.Database().BeginTx()
if err != nil {
	return message, httpCode, fmt.Errorf("%w", err)
}

txFinished := false
defer func() {
	if txFinished {
		return
	}

	rollbackMessage, _, rollbackErr := txSession.Rollback()
	if rollbackErr != nil {
		app.Log(gorest.Map{"error": rollbackErr}).Error(rollbackMessage)
	}
}()

createQuery := txSession.Table("users").Create(req)
if createQuery.Error != nil {
	return createQuery.Message, createQuery.HTTPCode, fmt.Errorf("create user: %w", createQuery.Error)
}

updateQuery := txSession.Table("users").
	Select("is_active").
	Where("id = ?", req.ID).
	Update(&User{ID: req.ID, IsActive: true})
if updateQuery.Error != nil {
	return updateQuery.Message, updateQuery.HTTPCode, fmt.Errorf("activate user: %w", updateQuery.Error)
}

message, httpCode, err = txSession.Commit()
txFinished = true // Commit ends the transaction even when it returns an error.
if err != nil {
	return message, httpCode, fmt.Errorf("commit txSession failed: %w", err)
}

return message, httpCode, nil
```

The deferred rollback runs only when the function exits before attempting Commit; rollback failures are logged. A successful `Rollback()` call reports that the driver accepted the rollback; it does not itself reread the row. In an integration test, verify the state using a fresh session after the handler returns:

```go
var persisted User
verify := app.Database().Table("users").Where("id = ?", req.ID).Find(&persisted)
if verify.Error == nil || verify.HTTPCode != http.StatusNotFound {
	t.Fatalf("expected rolled-back user to be absent; HTTP %d, error: %v", verify.HTTPCode, verify.Error)
}
```

> **Transaction support is SQL-only.** MongoDB and ScyllaDB currently treat `BeginTx()`, `Commit()`, and `Rollback()` as no-ops that return success. They do not provide atomicity or rollback through these ORM methods. Do not assume the SQL transaction example is safe for those drivers.

For raw SQL inside this transaction, call `txSession.RawQuerySQLWithTx()`. On a transaction-bound session, its callback receives an executor backed by the active `*sql.Tx`, so its statements are committed or rolled back together with the ORM and native `QuerySQL()`/`ExecSQL()` operations. On a session without a transaction, it uses the `*sql.DB` pool. The legacy `RawQuerySQL()` callback always targets the pool and is rejected when called from a transaction-bound session; it is deprecated and should only be used for work intentionally outside a transaction. See the [Querying Guide](querying.md#native-sql) for examples and SQL safety requirements.

MySQL `Update().Return()` starts an internal transaction only when the session has no active SQL transaction. When it reuses a handler transaction, the handler still owns commit/rollback.

Use pointers to structs or slices of structs as destinations and write payloads. Fields that map to database fields need `gorest` tags. A minimal model is:

```go
type User struct {
	ID           string `gorest:"id, primary_key"`
	Email        string `gorest:"email"`
	DepartmentID string `gorest:"department_id"`
	IsActive     bool   `gorest:"is_active"`
}
```

Use `gorest:"-"` for relation fields or fields that should not be mapped to stored columns.

## 2. Find records

`Find(&user)` stores only the first matching record in the struct. `Find(&users)` loads a slice. With no filter or explicit `Limit()`, SQL and ScyllaDB add an implicit limit of one for a struct destination; MongoDB also limits a struct destination to one. With a filter, SQL and ScyllaDB may read more than one matching row but the struct destination still keeps only the first.

Choose the destination based on the result you want:

- Use a unique-key filter (such as a primary key) with `Find(&user)` when the query should identify exactly one record.
- If several records may match but you intentionally want one, specify both `OrderBy()` and `Limit(1)` so the selected row is predictable.
- Use `Find(&users)` when you want every matching record. A slice destination without a limit returns all matches.

```go
var user User
query := app.Database().Table("users").Where("id = ?", id).Find(&user)

var users []User
query = app.Database().Table("users").Where("is_active = ?", true).Find(&users)
```

If a single-struct find matches no row, ORM returns `404 Not Found`. An empty slice result is a successful query with an empty slice. Update and delete also return not-found when no records match. Check `query.HTTPCode`; ORM validation errors commonly use 400, not-found uses 404, and database-operation errors use the shared status mapping described in [Error contract](#shared-database-error-contract).

After a successful query, the destination variable contains the loaded records and can be passed into application business logic. Only process it after checking `query.Error`:

```go
var users []User

query := app.Database().
	Table("users").
	Where("is_active = ?", true).
	Find(&users)

if query.Error != nil {
	return query.Message, query.HTTPCode, query.Error
}

for _, user := range users {
	// Apply application-specific business logic to each loaded user.
	if user.IsActive {
		// ...
	}
}
```

`Find(&users)` populates the slice in place. `query.Result` also refers to the destination, but application code can normally use `users` directly without a type assertion.

### Select columns

```go
db.Table("users").Select("id, email").Find(&users)
db.Table("users").Select("id", "email").Find(&users)
db.Table("users").Select("*").Find(&users)
```

Both comma-separated and variadic column names are accepted. `Find()` rejects empty expressions such as `Select("")`, `Select(" ")`, or `Select("email", "")` with 400. Use `Select("*")` to select all columns. For writes, ensure every selected name is a real tagged field. `Select()` is a projection: columns that do not match tagged fields are not written, so verify the final insert/update column set against your schema.

Projection only fills selected fields; unselected Go fields retain their Go zero values. That zero value does **not** prove the database stored an empty string, `false`, or `0`: the field may not have been selected at all. This matters when passing a projected model into business logic or another write operation.

```go
type User struct {
	ID       string `gorest:"id"`
	Email    string `gorest:"email"`
	IsActive bool   `gorest:"is_active"`
}

var user User
query := app.Database().Table("users").
	Select("id, email").
	Where("id = ?", id).
	Find(&user)
if query.Error != nil {
	return query.Message, query.HTTPCode, query.Error
}

// user.IsActive is false because it was not selected. The stored value may
// still be either true or false.
```

Select `is_active` (or use `Select("*")`) if business logic needs its value. Use a dedicated read/patch type, or a pointer field when your application must distinguish an unselected value from an explicitly loaded zero value. When using `Preload`, ORM may also select key columns needed to connect the related records.

### Where filters and injection safety

Bind values with `?` placeholders:

```go
db.Table("users").Where("users.id = ?", id).Find(&user)
db.Table("users").Where("department_id = ? AND is_active = ?", deptID, true).Find(&users)
```

For SQL drivers, placeholder values are passed as query arguments. This protects **values** from SQL injection. It does not make raw SQL fragments or identifiers safe. Never build a clause from untrusted input with `fmt.Sprintf`, string concatenation, or interpolation. Keep table and column names in application code or validate them against an allowlist. `Select`, `Join`, `GroupBy`, and SQL `Having` contain query syntax and should use trusted, developer-controlled expressions.

The ORM joins multiple `Where()` calls with `AND`; pass exactly one argument per placeholder. Placeholder-count validation is not uniform: SQL `Find()` and `Delete()` check for unused arguments, while SQL and ScyllaDB `Update()` builders check for missing arguments but have no equivalent explicit surplus-argument check. Some paths may therefore report a driver argument error instead. Count placeholders and arguments yourself until validation behavior is standardized across terminal operations. MongoDB translates a supported subset of simple SQL-like predicates into BSON; it is not a general SQL parser. ScyllaDB `Find()` and `Preload()` accept `LIKE` on string columns; matching is case-sensitive and covers the full column value. Non-key filtering may require `.AllowFiltering()` and can be expensive. The ORM still rejects `OR`, `IS NULL`, and parenthesized conditions in the Scylla `Preload()` condition parser. See [ScyllaDB's `LIKE` documentation](https://docs.scylladb.com/manual/stable/cql/dml/select.html#like-operator) and the driver matrix below.

`AllowFiltering()` is for ScyllaDB queries that filter non-key columns and are rejected by CQL without filtering enabled. It can make a query scan a large part of a table, so use it only when the data size and access pattern make that acceptable. It does not override other CQL rules such as the `SELECT DISTINCT` projection restriction or the `GROUP BY` primary-key-prefix rule.

### Distinct

For SQL and ScyllaDB, `Distinct("department_id")` selects distinct values for the listed columns:

```go
type DepartmentOnly struct {
	DepartmentID string `gorest:"department_id" json:"departmentId"`
}

var departments []DepartmentOnly
query := app.Database().Table("users").Distinct("department_id").Find(&departments)
if query.Error != nil {
	return query.Message, query.HTTPCode, query.Error
}
// departments contains one item per distinct department_id.
```

`Distinct(columns)` owns the selected columns. Do not combine it with `Select()`; the ORM returns a 400 conflict instead of silently choosing one. SQL emits `SELECT DISTINCT`. MongoDB uses an aggregation `$group` stage and maps the distinct field combination back to the destination struct. With multiple columns it returns unique tuples. ScyllaDB follows CQL's stricter projection rules. Mongo aggregation groups array values as whole values; this differs from MongoDB's native `distinct` command, which also returns individual array elements. See [MongoDB `$group`](https://www.mongodb.com/docs/manual/reference/operator/aggregation/group/) and [native `distinct`](https://www.mongodb.com/docs/manual/reference/method/db.collection.distinct/).

ScyllaDB applies CQL's `SELECT DISTINCT` restriction: the selection must contain only partition-key and/or static columns. `.AllowFiltering()` does not relax this restriction. Check the table primary key and static columns if Scylla rejects the query.

### GroupBy and Having

SQL drivers append SQL `GROUP BY` and `HAVING` clauses. Select only grouped columns and aggregate expressions. For example:

```go
type DepartmentSummary struct {
	DepartmentID string `gorest:"department_id" json:"departmentId"`
	UserCount    int64  `gorest:"user_count" json:"userCount"`
}

var summaries []DepartmentSummary
query := app.Database().Table("users").
	Select("department_id, COUNT(*) AS user_count").
	GroupBy("department_id").
	Having("COUNT(*) > ?", 2).
	Find(&summaries)
```

The result is populated by matching the aggregate alias to the `gorest` tag, then to the Go field. For example, `COUNT(*) AS user_count` maps to `UserCount` because it has `gorest:"user_count"`; the `json` tag controls the serialized property name:

```json
[
  {"departmentId": "dept-01", "userCount": 3},
  {"departmentId": "dept-02", "userCount": 5}
]
```

With `Having("COUNT(*) > ?", 2)`, only groups with a count greater than two appear in the result.

Database rules still apply: every selected non-aggregate field must be included in `GroupBy()`. SQLite is more permissive than the other SQL engines, so a query that works on SQLite may fail on MySQL, PostgreSQL, SQL Server, or Oracle. ORM reports common invalid grouped selections and unknown columns as 400 errors with guidance.

MongoDB implements grouping through an aggregation pipeline. `GroupBy()` columns must be tagged on the destination struct. `Select("*")` is not valid for grouped results; select group keys and aggregates with aliases, for example `Select("department_id, COUNT(*) AS user_count")`. Supported aggregate expressions are `COUNT`, `SUM`, `AVG`, `MIN`, and `MAX`. A Mongo `Having()` condition supports one comparison and one `?` argument per call, for example `Having("COUNT(*) > ?", 2)`. Each aggregate selected for the output needs an alias and a corresponding tagged destination field. Grouped Mongo results cannot use `Preload()`.

ScyllaDB supports `GROUP BY` only when the grouped columns form a prefix of the primary key: partition key first, then clustering columns in schema order. It does not support `HAVING`; filter aggregate results in application code or use another query strategy. See [ScyllaDB SELECT documentation](https://docs.scylladb.com/manual/stable/cql/dml/select.html).

### OrderBy

```go
db.Table("users").OrderBy("users.email", "DESC").Find(&users)
db.Table("users").OrderBy("email ASC, id DESC").Find(&users)
```

The direction must be `ASC` or `DESC`. The column must match a `gorest`-tagged field on the destination struct. Invalid directions, unknown fields, malformed terms, or a table qualifier that does not match `Table()` return 400 instead of being passed through as raw SQL. A one-argument form such as `OrderBy("email DESC")` is also accepted.

ScyllaDB has additional CQL restrictions: `ORDER BY` requires the partition key to be restricted by `=` or `IN`, and ordering is limited to clustering columns in the table's defined order or its reverse. See [ScyllaDB ordering results](https://docs.scylladb.com/manual/stable/cql/dml/select.html#ordering-results).

### Limit and Offset

```go
db.Table("users").OrderBy("id", "ASC").Limit(20).Offset(0).Find(&users)
```

Negative limit or offset is rejected with 400. `Limit(0)` means zero rows on SQL and regular MongoDB find queries, not “unlimited”; omit `Limit()` to fetch all rows. Offset is zero-based: `Offset(0)` starts at the first row. Add a deterministic `OrderBy()` when paginating so rows do not shift unpredictably between requests. MongoDB grouped aggregation currently does not apply an explicit zero limit, and ScyllaDB may reject `LIMIT 0`; avoid `Limit(0)` on those paths.

SQL Server and Oracle use their own `OFFSET`/`FETCH` syntax. MongoDB maps these options to `skip` and `limit`. ScyllaDB does not support `OFFSET`; use a clustering-key condition such as `Where("created_at < ?", lastSeen)` for paging. CQL `LIMIT` is supported; if `Limit(0)` is rejected by the Scylla server, omit the limit or handle the empty page before querying.

### Join and Preload

`Join()` accepts trusted, static SQL-style clauses for SQL drivers. MongoDB translates a limited form to `$lookup`: one `JOIN` or `LEFT JOIN`, equality between a base collection field and a joined collection field, and explicit qualified selected columns. Aliases on collection names, multiple joins, non-equality joins, wildcard selection, and combining joins with `GroupBy()`, `Having()`, or `Distinct()` are not supported by this path. The destination is a flat result struct whose `gorest` tags match selected names or aliases. MongoDB `$lookup` behavior and restrictions are documented at [MongoDB `$lookup`](https://www.mongodb.com/docs/manual/reference/operator/aggregation/lookup/). ScyllaDB CQL has no `JOIN`; use `Preload()` or issue separate queries. See [ScyllaDB SELECT documentation](https://docs.scylladb.com/manual/stable/cql/dml/select.html).

For MongoDB, the supported form looks like this:

```go
type UserDepartmentRow struct {
	UserID         string `gorest:"user_id" json:"userId"`
	DepartmentName string `gorest:"department_name" json:"departmentName"`
}

var rows []UserDepartmentRow
query := app.Database().Table("users").
	Select("users.id AS user_id, departments.name AS department_name").
	Join("LEFT JOIN departments ON departments.id = users.department_id").
	Find(&rows)
```

For SQL, the same `Join()` clause may be used with a SQL `SELECT`. MongoDB `Join()` only supports this simple equality form; use `Preload()` for nested relation objects or native aggregation for more complex joins.

`Preload()` eagerly loads relations described by the model's relation fields and tags. This complete example uses `users.department_id -> departments.id` and `orders.user_id -> users.id`:

```go
type Department struct {
	ID   string `gorest:"id, primary_key" json:"id"`
	Name string `gorest:"name" json:"name"`
}

type Order struct {
	ID     string  `gorest:"id, primary_key" json:"id"`
	UserID string  `gorest:"user_id" json:"userId"`
	Total  float64 `gorest:"total" json:"total"`
}

type User struct {
	ID           string      `gorest:"id, primary_key" json:"id"`
	Email        string      `gorest:"email" json:"email"`
	DepartmentID string      `gorest:"department_id" json:"departmentId"`
	Department   *Department `gorest:"-" references:"ID" json:"department,omitempty"`
	Orders       []Order     `gorest:"-" references:"UserID" json:"orders,omitempty"`
}

var users []User
query := app.Database().Table("users").
	Preload("Department").
	Preload("Orders").
	Find(&users)
if query.Error != nil {
	return query.Message, query.HTTPCode, query.Error
}
```

The loaded value can then be serialized as a nested object. For example, one user could look like this:

```json
{
  "id": "user-01",
  "email": "user@example.com",
  "departmentId": "dept-01",
  "department": {"id": "dept-01", "name": "Engineering"},
  "orders": [
    {"id": "order-01", "userId": "user-01", "total": 42.5}
  ]
}
```

Relation fields use `gorest:"-"` so they are not written as scalar database columns. For a belongs-to field such as `Department *Department`, the parent needs a foreign-key field named by convention (`DepartmentID` or `Department_ID`); its `gorest` tag determines the database column. `references:"ID"` on the relation points to the related `Department` struct's `ID` field. The reference value may be either that Go field name or its `gorest` column name.

For a has-many field such as `Orders []Order`, the parent needs a `primary_key` field. Here `references:"UserID"` points to the child `Order` struct's foreign-key field; Gorest uses its `gorest` tag (`user_id`) to query orders matching the parent's key. In short, `references` on belongs-to identifies the target key; on has-many it identifies the child foreign key.

Use `Preload()` when you want ORM to populate declared relation fields as nested objects without writing relation joins yourself. It supports the ORM's relation pattern across SQL, MongoDB, and ScyllaDB (MongoDB grouped aggregation cannot be combined with preload). Use `Join()` when you need a flat result assembled from columns in two collections/tables. SQL supports custom join clauses; MongoDB supports the limited `$lookup` form described above. A `Join()` does not populate a nested relation field automatically; select unambiguous columns and map aliases to fields on a result struct.

For example, use a SQL join when the output is a flat row containing fields from both tables:

```go
type UserDepartmentRow struct {
	UserID         string `gorest:"user_id" json:"userId"`
	DepartmentName string `gorest:"department_name" json:"departmentName"`
}

var rows []UserDepartmentRow
query := app.Database().Table("users").
	Select("users.id AS user_id, departments.name AS department_name").
	Join("LEFT JOIN departments ON departments.id = users.department_id").
	Find(&rows)
```

The aliases (`user_id`, `department_name`) must match the result struct's `gorest` tags. For a nested `User.Department` object, use `Preload("Department")` instead.

## 3. Create and update records

### Create

For examples below, assume this model maps fields to matching database columns:

```go
type User struct {
	ID           string `gorest:"id, primary_key" json:"id"`
	Email        string `gorest:"email, required" json:"email"`
	DepartmentID string `gorest:"department_id" json:"departmentId"`
	IsActive     bool   `gorest:"is_active" json:"isActive"`
	Age          int    `gorest:"age" json:"age"`
}
```

An explicit ID makes the example portable across drivers:

```go
user := User{
	ID:           "user-123",
	Email:        "admin@example.com",
	DepartmentID: "dept-01",
	IsActive:     true,
	Age:          30,
}

query := app.Database().Table("users").Create(&user)
if query.Error != nil {
	return query.Message, query.HTTPCode, query.Error
}

fmt.Printf("created %d row(s), key: %s\n", query.RowsAffected, query.LastInsertId)
```

Create accepts a struct or a non-empty slice of structs. Mark at least one field `primary_key` for all currently supported drivers, including MongoDB and ScyllaDB. `required` is opt-in and is validated before Create; see the [struct-tag guide](../guide.md#5-defining-entities--struct) for its empty-value rules. For example, if `email` is omitted from JSON, a Go string becomes `""`, and Create rejects it before sending a query:

```go
user := User{ID: "user-124", DepartmentID: "dept-01"} // Email is empty.
query := app.Database().Table("users").Create(&user)
// query.Error != nil; query.HTTPCode == 400
```

For SQL, duplicate primary-key or unique-key violations detected by the adapter return HTTP 409 Conflict. The database constraint remains the source of truth. MongoDB and ScyllaDB Create errors currently use their general adapter/ORM error classification; do not assume duplicate-key failures have the same HTTP status on every driver.

Use `Select()` to limit the columns written. In SQL, a database default can apply only when its column is omitted from the `INSERT`; an included zero value such as `false`, `0`, or `""` is explicit and does not request the default. A zero-valued SQL primary-key field is omitted by ORM, so the database can generate it only when the column type and schema support generation. MongoDB creates its own `_id`, but ORM does not generate a custom `id` field for you; ScyllaDB requires the primary key. For portable models, assign application-generated key values and use `Select()` deliberately when relying on SQL defaults. MongoDB and ScyllaDB do not share SQL column-default behavior. Keep `NOT NULL`, unique, and other integrity constraints in the database as an additional guard.

For bulk Create, SQL sends one multi-row `INSERT`, MongoDB calls `InsertMany`, and ScyllaDB uses a CQL batch. ORM does not wrap all bulk creates in a shared transaction. Do not assume a failed MongoDB bulk insert or ScyllaDB batch was rolled back as a unit; inspect the database and underlying error before retrying.

```go
newUsers := []User{
	{ID: "user-201", Email: "one@example.com", DepartmentID: "dept-01", IsActive: true, Age: 28},
	{ID: "user-202", Email: "two@example.com", DepartmentID: "dept-02", IsActive: true, Age: 31},
}

query := app.Database().Table("users").Create(&newUsers)
if query.Error != nil {
	return query.Message, query.HTTPCode, query.Error
}
fmt.Printf("created %d users\n", query.RowsAffected)
```

On success, both input records were submitted; the storage and partial-failure guarantees still follow the driver behavior described above.

### Update

```go
user := User{
	ID:           "user-123",
	Email:        "new@example.com",
	DepartmentID: "dept-01",
	IsActive:     false,
	Age:          0,
}
query := app.Database().Table("users").Update(&user)
if query.Error != nil {
	return query.Message, query.HTTPCode, query.Error
}

// Update only Email when that is the intended change.
query = app.Database().Table("users").Select("email").Update(&User{
	ID:    "user-123",
	Email: "new@example.com",
})
```

Update requires a `primary_key` tag. SQL and MongoDB single-record updates can use `.Where()`; bulk updates also use each row's key, and MongoDB combines it with an optional `.Where()` condition. ScyllaDB builds filters from key fields in the payload. SQL and MongoDB accept a struct or slice. ScyllaDB updates slice entries one at a time and uses `IF EXISTS` to detect missing rows. A field named `CreatedAt` and fields tagged `immutable` are excluded from updates. `Select()` restricts writable non-primary-key fields; key fields remain available for matching.

If the stored row has `old@example.com`, `true`, and `25`, the full update above writes these values:

| Field | Stored value before | Update value | Result |
|---|---:|---:|---|
| `Email` | `old@example.com` | `new@example.com` | `new@example.com` |
| `IsActive` | `true` | `false` | `false` |
| `Age` | `25` | `0` | `0` |

Zero values (`""`, `false`, `0`) are values, not indicators that a field was omitted. Use pointers or a dedicated patch model when the request needs to distinguish “not provided” from “set to the zero value.” `Select("email")` is another option when the update should touch only a known subset of fields.

Bulk Update is not uniformly all-or-nothing. SQL splits large updates into chunks; if a later chunk fails, earlier chunks may already be committed unless the caller uses an active SQL transaction. MongoDB uses unordered `BulkWrite`, so some writes may succeed when another fails. ScyllaDB issues one conditional update per input row; an error partway through can leave earlier rows updated. Use a supported transaction where appropriate, or design bulk operations to be safely retried and reconciled.

```go
changes := []User{
	{ID: "user-201", IsActive: false},
	{ID: "user-202", IsActive: false},
}

bulkQuery := app.Database().Table("users").Select("is_active").Update(&changes)
if bulkQuery.Error != nil {
	return bulkQuery.Message, bulkQuery.HTTPCode, bulkQuery.Error
}
// The requested update completed without an ORM error. See the driver-specific
// bulk notes above before assuming every row was changed atomically.
```

### Delete

```go
// Delete one record by its primary key.
query := app.Database().Table("users").Delete(&User{ID: "user-123"})
if query.Error != nil {
	return query.Message, query.HTTPCode, query.Error
}

// Verify the record is gone. A single-struct Find returns 404 when absent.
var deleted User
query = app.Database().Table("users").Where("id = ?", "user-123").Find(&deleted)
// query.HTTPCode == 404
```

Delete requires a primary-key-tagged model and refuses to execute without a usable key/filter. SQL and MongoDB single-record deletes can use `.Where()` or the key in the payload; ScyllaDB builds its filter from key fields in the payload. To delete a batch, pass a slice of identified records. If no record is deleted, ORM returns 404.

An empty payload without a usable key is rejected rather than deleting the whole table or collection:

```go
query := app.Database().Table("users").Delete(&User{})
// query.Error != nil; the ORM refuses an unscoped delete.
```

For SQL, Delete is one statement. MongoDB uses `DeleteOne` or `DeleteMany`; ScyllaDB deletes slice entries one by one, so a mid-operation error can leave earlier deletions in place. Inspect the result before retrying a partially completed operation.

```go
toDelete := []User{{ID: "user-201"}, {ID: "user-202"}}
query := app.Database().Table("users").Delete(&toDelete)
if query.Error != nil {
	return query.Message, query.HTTPCode, query.Error
}
fmt.Printf("deleted %d users\n", query.RowsAffected)
```

## 4. Return values from writes

`Return()` asks ORM to read values produced by a write back into the input object or a separate destination. Its variadic arguments accept a destination pointer and/or column names:

| Pattern | Destination | Columns returned |
|---|---|---|
| `Return("id, email")` | The input object passed to `Create()` or `Update()` | Only `id` and `email` |
| `Return(&result, "id, email")` | The separate `result` struct or slice | Only `id` and `email` |
| `Return()` | The input object | All compatible fields tagged with `gorest` |
| `Return(&result)` | The separate `result` struct or slice | All compatible fields tagged with `gorest` on the destination |

Only returned fields are overwritten in the destination; other fields keep their existing values. The requested columns must map to destination fields tagged with `gorest`. `Return()` is implemented for SQL `Create()` and `Update()` on the five SQL drivers. `Delete().Return()` and MongoDB/ScyllaDB write returns are not implemented.

```go
// Return selected values into the input object passed to Create.
user := User{ID: "user-123", Email: "admin@example.com", DepartmentID: "dept-01", IsActive: true, Age: 30}
query := app.Database().Table("users").Return("id, email").Create(&user)
if query.Error != nil {
	return query.Message, query.HTTPCode, query.Error
}
fmt.Printf("created user: id=%s email=%s\n", user.ID, user.Email)

// Return selected values into a separate destination on Update.
userID := "user-123"
newEmail := "new@example.com"
var updated User
query = app.Database().Table("users").
	Where("id = ?", userID).
	Select("email").
	Return(&updated, "id, email").
	Update(&User{ID: userID, Email: newEmail})
if query.Error != nil {
	return query.Message, query.HTTPCode, query.Error
}
fmt.Printf("updated user: id=%s email=%s\n", updated.ID, updated.Email)
```

The destination must be a pointer to a struct or a slice of structs. For bulk writes, use a slice destination or return into the input slice. When the destination is a struct but the write returns multiple rows, ORM reports an error; use a slice destination instead. If the input object is also the destination, pass it by pointer, for example `Create(&user)`.

Driver details:

- MySQL does not accept `UPDATE ... RETURNING`. Gorest uses a local transaction when no transaction is active: it locks matching keys, updates, then selects the returned fields. This path supports single and bulk updates. If the handler already has an active transaction, the operation uses it; the handler remains responsible for commit/rollback.
- Oracle uses `RETURNING ... INTO` and currently supports single-record `Update().Return()`. Oracle bulk update with Return is rejected; update records individually or omit `Return()`.
- PostgreSQL and SQLite use `RETURNING`; SQL Server uses `OUTPUT`.
- If a separate return destination is a single struct but the write returns multiple rows, use a slice destination instead.

## 5. Upsert

`Upsert()` changes `Create()` into an insert-or-update operation. Conflict-target syntax and behavior vary by driver:

```go
user := User{
	ID:           "user-123",
	Email:        "admin@example.com",
	DepartmentID: "dept-01",
	IsActive:     true,
	Age:          30,
}

query := app.Database().Table("users").Upsert("email").Create(&user)
if query.Error != nil {
	return query.Message, query.HTTPCode, query.Error
}
```

The word “upsert” does not mean that every driver updates the same fields. By default, SQL updates the inserted columns except the conflict columns and primary key. MongoDB updates document fields other than the conflict key and `_id`. ScyllaDB's regular `INSERT` writes the supplied columns for the full primary key. In all cases, fields included in the payload are values: an empty string, `false`, or `0` can replace an existing value.

| Scenario | Behavior to expect |
|---|---|
| No conflict on the target | A new record/document is inserted. |
| Conflict on the target | The existing record is updated using the driver-specific update-column rules above. The primary key is not updated by SQL's default update set; MongoDB's separate custom `id` field can be updated if it is not the conflict key. |
| Conflict target is not a tagged model field or is omitted from the insert | SQL/MongoDB validate that the target is a tagged field included in the write; ScyllaDB requires the full primary key. A zero-valued SQL primary key is omitted from the insert, so it cannot serve as the conflict column in that call. A missing Scylla primary-key value may instead be rejected by CQL. |
| Non-primary-key conflict value is empty | ORM does not universally reject it. Add `required` if the value must be non-empty, and retain the database's unique/not-null constraints. A zero-valued SQL primary key is a separate case: it is omitted from the insert. |
| A different unique key conflicts | Behavior depends on the driver; see the notes below. |

For example, suppose the database has `ID=user-123`, `Email=admin@example.com`, `Password=old-hash`, `IsActive=true`. If an upsert uses that email and sends `Password=""` and `IsActive=false`, the default SQL and MongoDB update sets can overwrite those values. For SQL and MongoDB, you can keep `password` in the insert payload while restricting updates to chosen fields with `UpsertUpdateCols`:

| Driver path | Existing record | Incoming payload | Result on email conflict |
|---|---|---|---|
| SQL default update set | `ID=user-123`, `Email=admin@example.com`, `Password=old-hash`, `IsActive=true` | `ID=user-999`, same email, `Password=""`, `IsActive=false`, `DepartmentID=dept-02` | SQL keeps `ID=user-123`; included non-key fields such as `Password`, `IsActive`, and `DepartmentID` are updated from the payload. |
| MongoDB default update set with a custom `id` field | `_id=mongo-1`, `ID=user-123`, `Email=admin@example.com`, `Password=old-hash`, `IsActive=true` | `ID=user-999`, same email, `Password=""`, `IsActive=false`, `DepartmentID=dept-02` | MongoDB keeps `_id=mongo-1`, but the custom `id` field and other non-conflict fields can be updated from the payload. |

For ScyllaDB, `email` is not a valid conflict target unless it is the table's full primary key. A Scylla `INSERT` upserts on the full primary key and writes the columns included in that statement; see the Scylla notes below.

```go
upsert := app.Database().Table("users").Upsert("email")
upsert.UpsertUpdateCols = []string{"department_id", "is_active"}
query := upsert.Create(&user)
```

This field list is set on the ORM before the terminal `Create()` call. ScyllaDB interprets an explicit update-column list as a CQL `UPDATE`; a new row then receives the primary key and listed columns only. For other cases, `Select()` limits the non-primary-key model columns written; for SQL, those columns also determine the default upsert update set. Primary-key fields are kept for matching/insertion when non-zero. Be careful: omitting a required field from `Select()` may make the insert invalid or prevent a database default from being used as expected.

SQL drivers provide dialect-specific conflict handling. PostgreSQL and SQLite target the specified conflict columns; SQL Server and Oracle match rows using that target in `MERGE`. MySQL's `ON DUPLICATE KEY UPDATE` is different: any primary-key or unique-key collision can trigger its update path, even if it is a unique key other than the column named in `Upsert("email")`. Do not assume a MySQL upsert only reacts to the named column.

For PostgreSQL, SQLite, SQL Server, and Oracle, a collision on a different unique key is generally an error because it does not match the specified conflict target. The adapter classifies detected SQL duplicate/unique violations as HTTP 409. On MongoDB, a collision with another unique index can make the upsert fail with a duplicate-key error. ScyllaDB upserts only on the full primary key; it does not support SQL-style unique conflict targets.

On SQL, `Upsert()` without columns uses the model's primary key. For MongoDB, without explicit conflict columns the implementation uses `_id`, which must be present and non-zero in the document; do not try to update MongoDB `_id`. If the model uses a separate custom `id` field, `Upsert("id")` and `Upsert("email")` have distinct conflict semantics.

Conditional upsert also varies. MySQL does not support the conditional `Upsert("column = ?", value)` form. PostgreSQL, SQLite, SQL Server, and Oracle have conditional upsert paths. MongoDB accepts only simple comparisons (`=`, `!=`, `<`, `<=`, `>`, `>=`, `IS NULL`, `IS NOT NULL`) joined with `AND`; if the condition prevents a match, MongoDB may attempt an insert and hit a duplicate-key error. ScyllaDB does not support conditional upsert because that requires a lightweight transaction. If Scylla conflict columns are supplied, they must exactly match the full primary key.

## 6. Delete and update safety

> **Warning:** A query can be syntactically valid and parameterized yet still change more records than intended. Placeholders protect values from being interpreted as SQL; they do not verify the scope or authorization of the operation.

For example, with a model that has a `primary_key` tag, this SQL Delete has a non-empty filter and passes the ORM's unscoped-delete guard:

```go
query := db.Table("users").
	Where("is_active = ?", false).
	Delete(&User{})
```

It can delete every inactive user in the SQL table. The ORM checks that a filter or key is present; it cannot know whether deleting every matching record was the caller's intent. MongoDB's single-record Delete path translates `.Where()` and deletes one matching document, while bulk deletion uses key fields from the input slice. ScyllaDB applies its own CQL key restrictions. Check the behavior for the selected driver and operation.

Keep these three protection layers distinct:

1. **ORM guard:** requires a usable filter/key and prevents unscoped writes. It does not determine whether a valid filter is too broad.
2. **Business authorization:** handler or library code must verify that the authenticated user may change these records. Include tenant/account ownership in the operation's scope where applicable; do not rely on a client-supplied ID alone.
3. **Database constraints:** primary keys, foreign keys, unique indexes, `NOT NULL`, and check constraints preserve data integrity if application code has a bug or concurrent requests race. Constraints do not replace authorization checks.

Tag the real database key with `primary_key` and use placeholders for values supported by `Where()` and `Having()`. The condition strings themselves are SQL/CQL fragments, not values, and GOREST does not sanitize them. `Join()`, `GroupBy()`, and `Select()` also accept query fragments that are incorporated into the generated statement; `Table()` accepts a table/collection name that is not an argument placeholder. Do not pass request-controlled strings to these APIs. Choose dynamic table names, columns, sort choices, join fragments, and projection expressions from fixed application constants or explicit allowlists. `OrderBy()` validates identifiers against tagged model columns and permits only `ASC`/`DESC`, but callers should still map external sort choices to a known set of columns. Keep schema constraints and indexes in migrations; ORM validation is an additional guard, not a replacement for database rules.

### Unit-test coverage and integration testing

GOREST's ORM has unit tests for core query-building and validation behavior. They exercise driver-specific placeholder formats, SQL literal/comment handling in the placeholder parser, placeholder/argument count checks, parameter binding (including values that resemble SQL injection payloads), OrderBy identifier validation, Create/Update/Delete/Upsert validation, pagination SQL generation, relation and preload-alias mapping, and query-state isolation. The repository's full Go unit-test suite passes with `go test ./...`; a focused race-detector test also exercises concurrent query building with independent ORM instances.

These tests verify GOREST logic in isolation. They do not connect to every supported database, prove that every generated query is accepted by every database version, or establish that an application is free from SQL injection. Parameter binding protects argument values, not SQL text assembled from external input. Raw fragments and identifiers supplied to `Where()`, `Having()`, `Join()`, `GroupBy()`, `Select()`, and `Table()` remain the caller's responsibility; `OrderBy()` has identifier validation, but dynamic choices should still use an application allowlist.

Application developers should add integration tests for their own handlers using a real database configured to match the target environment as closely as practical. GOREST cannot reproduce each application's database engine and version, schema, extensions, TLS and authentication settings, permissions, data shape, network conditions, or operational configuration. Those differences can affect whether a query succeeds and what it does, so only tests against the application's actual database setup can validate the end-to-end behavior for that deployment.

Integration tests should exercise representative success and failure cases through the handler and verify persisted state, not just the returned ORM status. Include the relevant authorization and tenant scoping, schema constraints, transactions and rollback behavior, pagination boundaries, relation/preload data, and concurrent operations for your workload. Run them against isolated test data and never rely on a unit test alone as evidence that production queries, security policies, or database configuration are correct.

## 7. Driver feature matrix

The labels describe the Gorest ORM API, not every capability of the underlying database:

- **Supported** — the ORM has a path for this feature; database rules and valid query construction still apply.
- **Limited** — the feature works only for the linked syntax, schema, or operation variants.
- **Not supported** — this ORM API has no implementation for that feature on the driver.
- **—** — the feature is driver-specific and does not apply to that column.

| Feature | MySQL | PostgreSQL | SQL Server | Oracle | SQLite | MongoDB | ScyllaDB |
|---|---|---|---|---|---|---|---|
| Find / Create / Update / Delete | Supported | Supported | Supported | Supported | Supported | Supported | Supported |
| Select / parameterized Where | Supported | Supported | Supported | Supported | Supported | [Limited: predicate subset](#where-filters-and-injection-safety) | [Limited: CQL predicates](#where-filters-and-injection-safety) |
| OrderBy | Supported | Supported | Supported | Supported | Supported | Supported | [Limited: CQL key/order rules](#orderby) |
| Limit | Supported | Supported | Supported | Supported | Supported | [Limited: zero limit in grouped aggregation](#limit-and-offset) | [Limited: CQL server rules](#limit-and-offset) |
| Offset | Supported | Supported | Supported | Supported | Supported | Supported | [Not supported](#limit-and-offset) |
| Join | Supported | Supported | Supported | Supported | Supported | [Limited: one equality `$lookup` with explicit projection](#join-and-preload) | [Not supported through this API](#join-and-preload) |
| Preload | Supported | Supported | Supported | Supported | Supported | [Limited: unavailable for grouped aggregation](#join-and-preload) | Supported |
| AllowFiltering | — | — | — | — | — | — | [Supported; use carefully](#where-filters-and-injection-safety) |
| Distinct | Supported | Supported | Supported | Supported | Supported | [Limited: aggregation tuple semantics](#distinct) | [Limited: CQL projection rules](#distinct) |
| GroupBy | [Supported: SQL grouping rules](#groupby-and-having) | [Supported: SQL grouping rules](#groupby-and-having) | [Supported: SQL grouping rules](#groupby-and-having) | [Supported: SQL grouping rules](#groupby-and-having) | [Supported: SQL grouping rules](#groupby-and-having) | [Limited: aggregation pipeline](#groupby-and-having) | [Limited: primary-key prefix](#groupby-and-having) |
| Having | Supported | Supported | Supported | Supported | Supported | [Limited: aggregate comparisons](#groupby-and-having) | [Not supported](#groupby-and-having) |
| Create/Update Return | Supported | Supported | Supported | [Limited: Update single only](#return-values-from-writes) | Supported | [Not supported](#return-values-from-writes) | [Not supported](#return-values-from-writes) |
| Upsert | [Supported: any unique-key collision can trigger update](#upsert) | [Supported](#upsert) | [Supported](#upsert) | [Supported](#upsert) | [Supported](#upsert) | [Limited: document/conflict semantics](#upsert) | [Limited: full primary key only](#upsert) |
| Conditional upsert clause | [Not supported](#upsert) | [Supported](#upsert) | [Supported](#upsert) | [Supported](#upsert) | [Supported](#upsert) | [Limited: condition subset](#upsert) | [Not supported](#upsert) |

Follow the links in limited or unsupported cells for the specific restriction and a workaround where available. A supported method can still fail when the query violates the database's syntax, schema, or key rules.

## 8. Troubleshooting

Use the sequence below to diagnose common failures. Reproduce the symptom with the shown query shape, make the suggested change, then verify the returned data or status before changing other parts of the query.

### Shared database error contract

For database-operation failures in `Create()`, `Update()`, `Delete()`, and `Find()`, Gorest keeps the existing ORM response fields (`Error`, `Message`, and `HTTPCode`) and standardizes the error category and status across drivers:

| Error category in `query.Error` | HTTP status | Meaning |
|---|---:|---|
| `database: conflict` | 409 | A primary-key or unique constraint rejected the write. |
| `database: invalid_query` | 400 | A driver-specific error code or recognized query message indicates invalid syntax, a missing table/column, invalid grouping, or an unsupported Scylla query rule. |
| `database: timeout` | 504 | The database operation exceeded its deadline or timed out. |
| `database: unavailable` | 503 | The driver reported a network or connection failure. |
| `database: execution_failed` | 500 | The database operation failed for another reason. |

`query.Error` wraps the original driver error, so it remains available to `errors.Is`, `errors.As`, and server-side logs. `query.Message` gives the operation and troubleshooting guidance and links to the configured vendor's documentation. Do not use raw driver error text as a public API response.

Use the relevant official reference when investigating a category or driver-specific error:

| Driver | Official error or query reference |
|---|---|
| Oracle | [Oracle Database Error Messages](https://docs.oracle.com/en/database/oracle/oracle-database/23/errmg/) |
| SQL Server | [Database Engine events and errors](https://learn.microsoft.com/en-us/sql/relational-databases/errors-events/database-engine-events-and-errors?view=sql-server-ver17) |
| MySQL | [MySQL Server Error Reference](https://dev.mysql.com/doc/mysql-errors/8.4/en/) |
| PostgreSQL | [PostgreSQL Error Codes](https://www.postgresql.org/docs/current/errcodes-appendix.html) |
| SQLite | [SQLite Result and Error Codes](https://www.sqlite.org/rescode.html) |
| MongoDB | [MongoDB Error Codes](https://www.mongodb.com/docs/manual/reference/error-codes/) |
| ScyllaDB | [ScyllaDB CQL Reference](https://docs.scylladb.com/manual/stable/cql/) |

| Symptom / reproduction | Diagnosis | Action | Verify |
|---|---|---|---|
| `Find(&user)` returns 404 | A single-struct Find matched no record. | Check the table/collection, key value and type, filter column, and active database. | Retry with a known existing key; expect one populated struct and HTTP 200. |
| `Find(&users)` returns an empty slice with 200 | No records matched; this is successful slice behavior. | Check filter arguments and database/collection selection. | Retry without the filter or with a known matching value; confirm the slice contents. |
| `Select("", "email")` reports an empty column expression | A Select argument is empty or whitespace. | Remove the empty argument, or use `Select("*")` when all columns are intended. | Rerun and inspect that the requested columns are populated. |
| `Select("id, email")` leaves another field at its zero value | The field was not part of the projection; zero does not tell you whether the stored value was zero or the field was not read. | Add that column to Select, or use `Select("*")`. | Inspect the returned field and compare it with a query that selects the column. |
| `.Where("id = %s", id)` reports unused arguments, or placeholder mismatch | The filter has no recognized placeholder for the supplied argument, or the placeholder and argument counts differ. | Use `.Where("id = ?", id)` and pass one argument per active `?` across all Where clauses. | Retry with a known ID; verify the query matches only that record. |
| SQL `GroupBy("department_id")` fails with a non-grouped column error | The projection includes ordinary columns that are neither grouped nor aggregated. | Select only the group key and aggregate expressions, such as `department_id, COUNT(*) AS user_count`. | Confirm the destination's `gorest` tags match selected aliases and test on the same SQL engine as production. |
| Mongo grouped query rejects a selected field | The field is not tagged on the destination, an aggregate alias is missing, or `Select("*")`/`Preload()` was used. | Project only group keys and supported aggregates with aliases; remove Preload. | Check that each result field maps to a destination `gorest` tag. |
| Scylla reports `Group by non-primary-key column` | GroupBy does not follow the table's primary-key prefix. | Start with the partition key, then use clustering columns in schema order. | Compare the requested group columns against the table schema and rerun. |
| Scylla rejects `SELECT DISTINCT` despite `.AllowFiltering()` | The projection contains a column other than permitted partition-key/static columns. AllowFiltering does not waive this rule. | Limit `Distinct()` to columns allowed by the Scylla table's key and static-column definitions. | Verify the projection against the schema, then rerun without expecting non-key values. |
| Scylla rejects `ORDER BY` | The partition key is not restricted by `=` or `IN`, or the requested sort conflicts with clustering-key order. | Restrict the partition key and order by supported clustering columns in schema order or reverse order. | Check the partition/clustering key definition and rerun with a valid key filter. |
| Scylla rejects Join, Having, or Offset | The ORM/CQL path does not support that clause. | Use Preload or separate queries for relations; filter aggregate results in application code; use key-based paging instead of Offset. | Replace the unsupported clause and verify against the Scylla schema and query rules. |
| MySQL update with Return fails near `RETURNING` | MySQL does not implement native `UPDATE ... RETURNING`. | Use the ORM's MySQL transaction-and-select path; ensure the table has a usable tagged primary key. If already in a transaction, let the handler own commit/rollback. | Check that the returned destination is populated and that the surrounding transaction completes successfully. |
| Create writes `""` when JSON omitted a string field | JSON decoding gives a Go string its zero value, and the field is included in the write. | Add `required` when the value must be non-empty; use pointers or a patch model when omitted differs from empty; keep DB constraints. | Submit a payload without the field; confirm required validation returns 400 before a database write. |
| Mongo Find returns no records although documents exist | The active database/collection or field mapping may not match the stored BSON document. | Confirm database and collection names and `gorest` tags. Remember `gorest:"id"` maps the `id` field; Mongo's `_id` needs the `_id` tag. | Query the same collection directly and compare document field names/types with the model tags. |
| Mongo `object_id` filter does not match | The tag requests ObjectID conversion but the stored BSON value or input format differs. | Use `object_id` only with a compatible 24-hex ObjectID; keep UUID-like string keys as strings. | Check the BSON type in the stored document and retry with a value in the matching format. |

### Keep internal errors out of public responses

`query.Error` contains the underlying failure and `query.Message` provides troubleshooting guidance. Database errors can include table, column, constraint, or driver details. Log those values for diagnosis, but map them to a stable public message when returning an API response. Preserve `query.HTTPCode` so the client still receives the classified status:

```go
query := handler.App.Database().Table("users").Where("id = ?", userID).Find(&user)
if query.Error != nil {
	handler.App.Log(gorest.Map{"error": query.Error}).Error(query.Message)

	response.WithMessage(
		"FAILED TO GET USER",
		errors.New("Unable to retrieve the user."),
		query.HTTPCode,
	)
	return response.SendJSON(ctx, response, nil)
}
```

In this example, logs keep the original ORM message and underlying error, while the client receives a safe, consistent message. Adapt the public message to the endpoint; do not return raw SQL/CQL/Mongo errors or schema details unless the endpoint is intentionally a development-only interface.
