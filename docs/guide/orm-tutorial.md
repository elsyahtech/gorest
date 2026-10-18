# ORM Tutorial: From Model to Troubleshooting

This tutorial walks through one `User` model from setup to common failure cases. It complements the [ORM reference guide](orm.md): use that guide for the full API behavior and driver-specific details. For help deciding between ORM and native queries, start with the [Querying Guide](querying.md). The examples assume the `users` table or collection already exists; see the [Database Guide](database.md) for connection and schema setup.

The ORM supports MySQL, PostgreSQL, SQL Server, Oracle, SQLite, MongoDB, and ScyllaDB, but query capabilities differ. The query examples below use the SQL-style ORM API; MongoDB and ScyllaDB have driver-specific query and write constraints. Check the driver matrix and feature notes in the reference before relying on a feature across drivers.

## 1. Define a model and destinations

The first `gorest` tag item is the stored column or document field. `primary_key` identifies the field used by ORM write guards and record matching. `required` opts into validation before `Create()`; it does not replace database constraints or business validation.

```go
type User struct {
	ID           string `gorest:"id, primary_key, required" json:"id"`
	Email        string `gorest:"email, required" json:"email"`
	Password     string `gorest:"password, required" json:"password"`
	DepartmentID string `gorest:"department_id, required" json:"departmentId"`
	IsActive     bool   `gorest:"is_active" json:"isActive"`
}
```

Use a single struct when expecting one row, and a slice when expecting zero or more rows:

```go
var user User
var users []User
```

The `gorest` tag controls ORM mapping; the `json` tag controls JSON names. For MongoDB, map the actual field name in the document. An application UUID stored in `id` is not MongoDB's built-in `_id`; see the [MongoDB `object_id` notes](../guide.md#mongodb-object_id-and-_id) in the main guide.

## 2. Read data

Chain methods configure a query. The terminal method `Find()` executes it and populates the destination:

```go
var users []User

query := app.Database().
	Table("users").
	Where("is_active = ?", true).
	Select("id, email, department_id, is_active").
	OrderBy("email", "ASC").
	Limit(20).
	Offset(0).
	Find(&users)

if query.Error != nil {
	return query.Message, query.HTTPCode, query.Error
}

for _, user := range users {
	// Use each loaded user in application logic.
	_ = user.Email
}
```

Use `Where()` placeholders for values. Keep column names and SQL fragments developer-controlled; placeholders do not validate identifiers. `Offset(0)` starts at the first row. Add a stable `OrderBy()` when paging so rows do not shift unpredictably between requests.

Use `Find(&user)` with a unique filter when looking for one record. A single-struct destination stores at most one match; if multiple rows could match and one is intentionally wanted, use a deterministic order and `Limit(1)`. Use a slice for all matches. A missing single record returns 404, while no matches in a slice is a successful empty result.

`Select()` is a projection. Here `user.Password` is not fetched and remains `""`; a zero value may mean the field was not selected, not that the stored value is empty. Do not pass a partially selected model into update logic unless that is intentional. Use `Select("*")` or include every field needed by the next step.

For pagination, validate client-provided limit and offset before calling the ORM, and always define a stable order. Driver-specific pagination limitations are listed in the [feature matrix](orm.md#7-driver-feature-matrix).

## 3. Create, update, and delete

### Create one record

```go
user := User{
	ID:           "user-1001",
	Email:        "alex@example.com",
	Password:     "hashed-password",
	DepartmentID: "dept-01",
	IsActive:     true,
}

query := app.Database().Table("users").Create(&user)
if query.Error != nil {
	return query.Message, query.HTTPCode, query.Error
}
```

If a `required` string is omitted or blank, ORM validation returns a client error before executing the insert. A duplicate primary or unique key typically returns 409 when the adapter can identify the conflict. Keep database constraints: they protect data under concurrent requests and outside Gorest too.

Go zero values such as `false`, `0`, and `""` are real values and may be written. Do not assume they mean “field omitted.” Database-generated defaults apply only when the ORM omits a column from the insert; exact zero-value and `Select()` behavior can vary by driver. Check the [Create reference](orm.md#3-create-and-update-records) for details.

For bulk create, pass a slice:

```go
newUsers := []User{
	{ID: "user-1002", Email: "sam@example.com", Password: "hash-1", DepartmentID: "dept-01", IsActive: true},
	{ID: "user-1003", Email: "lee@example.com", Password: "hash-2", DepartmentID: "dept-02", IsActive: true},
}

query := app.Database().Table("users").Create(&newUsers)
if query.Error != nil {
	return query.Message, query.HTTPCode, query.Error
}
// query.RowsAffected reports the count when supported by the driver.
```

Do not assume bulk operations are all-or-nothing across drivers. Batch size, driver behavior, and where an error occurs can affect whether earlier rows were already written. Consult the driver notes and verify persisted state when partial success matters.

### Update a record

`Update()` uses the supplied model values, including zero values. For example, this deliberately changes `IsActive` from `true` to `false`:

```go
patch := User{ID: "user-1001", IsActive: false}
query := app.Database().Table("users").
	Where("id = ?", "user-1001").
	Select("is_active").
	Update(&patch)
if query.Error != nil {
	return query.Message, query.HTTPCode, query.Error
}
```

Use `Select()` to constrain the columns written, as above. For APIs that must distinguish an omitted field from an explicit `false`, `0`, or empty string, decode into a patch type with pointer fields and build the update deliberately. `immutable` fields such as `created_at` are skipped by ORM updates.

For bulk update, use a slice and a deliberate filter. Verify the target set before executing, and check `RowsAffected` where the driver reports it:

```go
patches := []User{
	{ID: "user-1001", IsActive: false},
	{ID: "user-1002", IsActive: false},
}
query := app.Database().Table("users").Select("is_active").Update(&patches)
if query.Error != nil {
	return query.Message, query.HTTPCode, query.Error
}
```

Before a bulk update, Gorest checks that all supplied IDs exist. If one or more are missing, it returns HTTP 404 and lists the missing IDs; no updates are issued. SQL and MongoDB perform this check in one lookup, while ScyllaDB checks each full primary key. This prevents a missing ID from being silently ignored while the other rows update.

The preflight check handles missing IDs, but bulk updates can still partially complete if a later write fails or another process changes the records concurrently. SQL callers can use a transaction where supported. Review the [driver-specific bulk update notes](orm.md#4-update) before relying on atomic behavior.

### Delete records safely

For SQL, MongoDB, and ScyllaDB ORM `Delete()`, provide the key in the payload struct. The ORM builds the delete condition from fields tagged `primary_key`:

```go
query := app.Database().Table("users").
	Delete(&User{ID: "user-1001"})
if query.Error != nil {
	return query.Message, query.HTTPCode, query.Error
}
```

No matching record returns 404. A broad but valid filter can still delete many rows in query APIs that accept arbitrary filters. For example, a delete condition equivalent to `is_active = false` targets every inactive user; parameterization prevents value injection, but it does not guarantee the target set is narrow enough for the business operation. The ORM `Delete()` examples here use primary-key fields from the payload rather than an arbitrary `Where()` condition. Apply authorization and scope checks in application logic, and inspect the intended target set before destructive bulk operations. ORM guards, business authorization, and database constraints are separate protection layers.

### Return and Upsert

`Return()` can populate the input object or a separate destination, depending on its arguments and the operation. For example, a separate destination makes returned values explicit:

```go
var updated User
query := app.Database().Table("users").
	Where("id = ?", "user-1001").
	Return(&updated, "id, email, is_active").
	Update(&User{ID: "user-1001", Email: "new@example.com"})
if query.Error != nil {
	return query.Message, query.HTTPCode, query.Error
}
// Use updated only after the error check.
```

Return support and cardinality limits differ by driver. MySQL uses an internal transaction plus a follow-up select when it owns the transaction; an existing handler transaction remains owned by the handler. Oracle update-return currently has single-record limitations. MongoDB and ScyllaDB do not support SQL-style `Return()` through this API. See [Return values from writes](orm.md#4-return-values-from-writes).

Upsert inserts when its conflict target has no conflict and follows driver-specific conflict handling when one exists:

```go
query := app.Database().Table("users").Upsert("email").Create(&user)
if query.Error != nil {
	return query.Message, query.HTTPCode, query.Error
}
```

Do not infer which fields are updated from the method name. A payload's zero values may overwrite existing values, and conflicting keys outside the specified target can still fail. Verify the before/after behavior for the target driver; details are in [Upsert](orm.md#5-upsert).

## 4. Exercise failure paths

Use a development database and disposable records to check these cases before depending on an endpoint:

| Scenario | Expected behavior to inspect |
| --- | --- |
| Single-row `Find()` with an unknown ID | 404, with no populated record |
| Slice `Find()` with no matches | Success with an empty slice |
| `Create()` with a missing required field | 400 before a database insert |
| `Create()` with a duplicate unique value | Conflict response where recognized by the adapter; verify the original row remains intact |
| Invalid `Select()` or unsupported clause | Validation error with a client status when rejected by ORM |
| SQL transaction fails before commit | Rollback; verify from a fresh session that the write is absent |
| Destructive filter matches multiple rows | Confirm the intended scope and authorization before running it |

For transactions, use the complete SQL lifecycle example in [Transactions](orm.md#transactions): rollback on every operation failure, handle commit failure, and verify the final database state. MongoDB and ScyllaDB transaction methods are currently no-ops in this ORM and do not provide rollback or atomicity.

## 5. Diagnose and verify

ORM calls return an ORM object. Inspect all three error fields at the layer that handles the operation:

```go
query := app.Database().Table("users").Where("id = ?", id).Find(&user)
if query.Error != nil {
	app.Log(gorest.Map{
		"error": query.Error,
		"httpCode": query.HTTPCode,
	}).Error(query.Message)

	// The handler can map this to an API response or a stable public message.
	return query.Message, query.HTTPCode, query.Error
}
```

`Error` is the underlying error for control flow and internal logging; `Message` is troubleshooting context; `HTTPCode` is the suggested status. Database errors can contain table, constraint, or driver details. If those details should not be exposed to clients, log them internally and return a stable public message from the handler while preserving the appropriate status.

After a write error or rollback, query the database again through a fresh session to establish what persisted. Do not rely only on the returned error or `RowsAffected`, whose availability and meaning vary by driver. For repeated issues, follow the [troubleshooting guide](orm.md#8-troubleshooting), then review the relevant driver's feature limitations.

## Continue learning

- [ORM reference: all features, limits, and driver matrix](orm.md)
- [Database connection and schema setup](database.md)
- [Gorest struct tags](../guide.md#5-defining-entities--struct)
