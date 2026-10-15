# Choosing ORM or Native Queries

Gorest provides two ways to access a database: the chainable ORM and native queries through `DBSession`. Both are supported paths. Neither is the right choice for every operation; choose based on the query shape, the control you need, and the behavior your business logic requires.

This page is a decision guide and a starting point for native query APIs. For ORM syntax and its driver matrix, see the [ORM reference](orm.md) and [ORM tutorial](orm-tutorial.md). For connection setup and schema management, see the [Database Guide](database.md).

The examples use this small result type where needed; add the `gorest` tags if you also use it as an ORM destination:

```go
type UserSummary struct {
	ID    string `gorest:"id"`
	Email string `gorest:"email"`
}
```

## Decide before implementing

| Consideration | ORM | Native query APIs |
| --- | --- | --- |
| Query construction | Chain methods such as `Table`, `Where`, `Select`, and `OrderBy` | Write SQL/CQL or build BSON filters and driver operations directly |
| Struct mapping | Uses `gorest` tags to map columns/fields to Go structs | SQL requires `Scan`; MongoDB offers Gorest BSON encode/decode helpers; Scylla mapping is handled by the caller |
| Common CRUD | Convenient for supported, conventional operations | More code, but direct access to query and driver capabilities |
| Specialized operations | Limited to features implemented by the ORM and the active driver | Appropriate when ORM has no method or query shape for the operation |
| Portability | The API is more uniform, but SQL dialects and NoSQL rules still differ | Query text, result mapping, and driver calls are typically database-specific |
| Control and responsibility | ORM builds statements and applies its validations and error classification | Developer owns query construction, result iteration/mapping, resource cleanup, and operation-specific error handling |
| Best fit | Repeated CRUD and queries that match the ORM's supported API | Complex or database-specific queries, pipelines, CQL operations, and cases needing direct driver options |

### Prefer ORM when

- The operation fits the ORM API and the driver supports the needed behavior.
- You want `gorest`-tag based mapping and shared CRUD/query code.
- The query is conventional and the ORM's guardrails and error handling are useful.

### Prefer native APIs when

- The needed feature is unavailable or limited in the ORM for your selected driver.
- You need a database-specific construct, MongoDB aggregation pipeline, specialized driver option, or exact SQL/CQL statement.
- You have measured a need for more control and can take responsibility for mapping, cleanup, and driver-specific behavior.

### Tradeoffs to weigh

- **ORM advantages:** less repeated mapping and CRUD code, a consistent chain API, `gorest` tag mapping, and ORM-level validation/guardrails.
- **ORM costs:** the API cannot express every database feature; generated behavior still varies by driver, and developers need to understand the database rules behind a query.
- **Native advantages:** direct access to SQL/CQL syntax, MongoDB operations and pipelines, and driver-specific options; query intent is explicit in the statement or operation.
- **Native costs:** more database-specific code, manual result mapping and resource handling, and more responsibility for consistent validation, error classification, and safe query construction.

Native does not automatically mean faster or safer. ORM-generated and native operations both need appropriate indexes, query plans, authorization, input validation, and database constraints. Choose on the requirements of the operation; use both approaches in one application when that is the clearest fit.

## Shared safety and lifecycle rules

- Bind values as parameters. Do not concatenate client input into SQL/CQL text or BSON field names. Identifiers and sort directions usually cannot be bound as values; choose them from fixed application values or validate them against an allowlist.
- Create a context with an appropriate deadline and always call its cancel function.
- Check the returned `error`; use `message` as troubleshooting context and `httpCode` as a suggested status. Decide separately what message is appropriate for the public API.
- Close result resources. SQL rows must be closed and checked with `rows.Err()`. Mongo cursors must be closed. Scylla iterators must be closed and the close error checked.
- Use `DBSession.BeginTx()` for SQL transactions; it applies `database.Config.Timeout`. Use `BeginTxContext(ctx)` when the transaction should also follow the request's cancellation or deadline. `QuerySQL()`, `ExecSQL()`, and `RawQuerySQLWithTx()` use the active transaction session. The legacy `RawQuerySQL()` pool callback is rejected when called on a transaction-bound session; use `RawQuerySQLWithTx()` for raw SQL that must participate in the transaction. MongoDB and ScyllaDB `BeginTx`/`Commit`/`Rollback` are currently no-ops in Gorest and do not make a sequence of operations atomic.

## Native SQL

Use `QuerySQL()` for result sets and `ExecSQL()` for statements that do not return rows. These methods return the driver result or rows plus a troubleshooting message, HTTP status code, and error.

### SQL safety boundary

Native SQL is an intentional flexibility feature, not evidence of a SQL injection in Gorest. `QuerySQL()` and `ExecSQL()` pass query text and arguments to the configured database driver; they do not make arbitrary SQL text assembled by the application safe. The application remains responsible for how it constructs that text.

Bind every user-controlled **value** as an argument using the placeholder syntax supported by the configured driver. Do not interpolate or concatenate request values into SQL. Placeholders generally cannot represent SQL identifiers or syntax, such as table names, column names, or `ASC`/`DESC`; choose those from fixed application-defined options or validate them against a strict allowlist before including them in the query. Validation or escaping is not a substitute for binding values.

Avoid building SQL by concatenating user input:

```go
// Assume ctx and userInput are already available.
// Do not use: userInput becomes part of the SQL syntax.
query := "SELECT * FROM customers WHERE name = '" + userInput + "'"
rows, message, httpCode, err := app.Database().QuerySQL(ctx, query)
```

Pass the value separately through `QuerySQL()` instead:

```go
// Assume ctx and userInput are already available.
query := "SELECT id, name FROM customers WHERE name = ?"
rows, message, httpCode, err := app.Database().QuerySQL(ctx, query, userInput)
if err != nil {
	return message, httpCode, fmt.Errorf("find customer: %w", err)
}
defer rows.Close()
```

Here `?` is illustrative; use the placeholder syntax required by the configured SQL driver (for example, PostgreSQL commonly uses `$1`). The driver binds `userInput` as a value rather than interpreting it as SQL syntax. Continue to check scan and iteration errors and close rows, as in the complete example below.

```go
ctx, cancel := app.NewContext(5 * time.Second)
defer cancel()

rows, message, httpCode, err := app.Database().QuerySQL(ctx,
	"SELECT id, email FROM users WHERE department_id = ?",
	departmentID,
)
if err != nil {
	return message, httpCode, fmt.Errorf("list users: %w", err)
}
defer rows.Close()

var users []UserSummary
for rows.Next() {
	var user UserSummary
	if err := rows.Scan(&user.ID, &user.Email); err != nil {
		return "Could not read user query results", http.StatusInternalServerError,
			fmt.Errorf("scan user row: %w", err)
	}
	users = append(users, user)
}
if err := rows.Err(); err != nil {
	return "Could not read user query results", http.StatusInternalServerError,
		fmt.Errorf("iterate user rows: %w", err)
}
```

The example uses `?` for illustration. Placeholder syntax is driver-specific; use the syntax supported by the configured SQL driver and pass values through `args`. `ExecSQL()` accepts the same context and variadic arguments and returns `sql.Result`, message, status, and error. Use `RowsAffected()` only where its meaning is useful and supported by the chosen driver. The snippets omit imports; add the standard library and driver packages used in your application, such as `context`, `database/sql`, `fmt`, `net/http`, `time`, and MongoDB's `bson` package.

For a native write, use `ExecSQL()` with bound values as well:

```go
result, message, httpCode, err := app.Database().ExecSQL(ctx,
	"UPDATE users SET email = ? WHERE id = ?",
	newEmail,
	userID,
)
if err != nil {
	return message, httpCode, fmt.Errorf("update user email: %w", err)
}

rowsAffected, err := result.RowsAffected()
if err != nil {
	app.Log(gorest.Map{"error": err}).Error("could not read affected row count")
} else if rowsAffected == 0 {
	// Decide whether this means not found for this operation's business rules.
}
```

`QuerySQL()` and `ExecSQL()` run through the `DBSession`; when that session is transaction-bound, they execute inside its SQL transaction. Use the same transaction session for all statements in the unit of work, then handle both rollback and commit errors. See the complete [ORM transaction lifecycle example](orm.md#transactions); the same session lifecycle applies to native SQL calls.

Prefer `RawQuerySQLWithTx()` when the wrapped helpers are insufficient. If called on a transaction-bound `DBSession`, it passes a small `SQLExecutor` interface backed by that session's active `*sql.Tx`. Therefore statements executed through the callback are part of the same transaction and are committed or rolled back by `txSession.Commit()` or `txSession.Rollback()`. If the session has no active transaction, the executor is backed by the underlying `*sql.DB` pool. A callback error is returned to the caller; Gorest does not automatically roll back the transaction, so handle the error and call `Rollback()` as part of the normal transaction lifecycle:

```go
err := txSession.RawQuerySQLWithTx(ctx, func(ctx context.Context, executor gorest.SQLExecutor) error {
	_, err := executor.ExecContext(ctx,
		"UPDATE users SET email = ? WHERE id = ?",
		newEmail,
		userID,
	)
	return err
})
if err != nil {
	return "Could not update user", http.StatusInternalServerError,
		fmt.Errorf("raw update user: %w", err)
}
```

`RawQuerySQL()` is retained for compatibility with callbacks that specifically require `*sql.DB`, but it is deprecated and returns an error when called on a transaction-bound session. Use it only for operations intentionally outside a transaction. Gorest does not sanitize SQL assembled inside either callback: bind values with the driver's parameter syntax and allowlist dynamic identifiers. If a raw callback runs a query that returns rows, it must close them and check `rows.Err()` just as it would with `QuerySQL()`.

The example uses `?` illustratively; use the placeholder syntax supported by the configured SQL driver.

## Native MongoDB

`MongoCollection()` returns the configured collection handle. It does not run a query. Use the MongoDB driver's collection methods and BSON filters/options directly. `EncodeMongoCollection()` converts a struct to `bson.M` using the first item in each `gorest` tag; `DecodeMongoCursor()` maps BSON document fields back using those tags.

```go
ctx, cancel := app.NewContext(5 * time.Second)
defer cancel()

collection, message, httpCode, err := app.Database().MongoCollection("users")
if err != nil {
	return message, httpCode, fmt.Errorf("get users collection: %w", err)
}

filter := bson.M{"is_active": true}
cursor, err := collection.Find(ctx, filter)
if err != nil {
	return "Could not query users", http.StatusInternalServerError,
		fmt.Errorf("find users: %w", err)
}
defer func() {
	if err := cursor.Close(ctx); err != nil {
		app.Log(gorest.Map{"error": err}).Error("failed to close users cursor")
	}
}()

var users []User
message, httpCode, err = app.Database().DecodeMongoCursor(ctx, cursor, &users)
if err != nil {
	return message, httpCode, fmt.Errorf("decode users: %w", err)
}
```

`DecodeMongoCursor()` fills a pointer to a struct or slice of structs, but the caller owns the cursor and must close it. `EncodeMongoCollection()` is a conversion helper, not a write wrapper; call methods such as `InsertOne`, `UpdateOne`, `ReplaceOne`, `DeleteOne`, or `BulkWrite` on the collection as required. Inspect and classify driver errors at the call site, and consider whether database details should be exposed to clients.

```go
doc, message, httpCode, err := app.Database().EncodeMongoCollection(&user)
if err != nil {
	return message, httpCode, err
}

if _, err := collection.InsertOne(ctx, doc); err != nil {
	return "Could not save user", http.StatusInternalServerError,
		fmt.Errorf("insert user: %w", err)
}
```

The encoder includes exported fields with `gorest` tags and uses their mapped names. A nil pointer field is encoded as `null`; `json:"...,omitempty"` does not control this conversion. Review the resulting document shape against your collection schema, and use driver BSON tags or explicit BSON documents when you need a mapping policy other than this helper provides.

For aggregation pipelines, call the driver's `Aggregate()` directly. The ORM's `GroupBy()`/`Having()` methods do not currently translate into a MongoDB aggregation pipeline.

`RawQueryMongo()` is an escape hatch that passes the underlying MongoDB client to a callback. Prefer `MongoCollection()` for ordinary collection work so the collection is selected from the configured database.

## Native ScyllaDB

Use `QueryCQL()` for result iteration, `ExecCQL()` for statements, and `ScanCQL()` for conditional statements whose applied/not-applied result matters.

```go
ctx, cancel := app.NewContext(5 * time.Second)
defer cancel()

iter, message, httpCode, err := app.Database().QueryCQL(ctx,
	"SELECT id, email FROM users WHERE department_id = ?",
	departmentID,
)
if err != nil {
	return message, httpCode, fmt.Errorf("query users: %w", err)
}

users := make([]UserSummary, 0)
row := make(map[string]any)
for iter.MapScan(row) {
	users = append(users, UserSummary{
		ID:    fmt.Sprint(row["id"]),
		Email: fmt.Sprint(row["email"]),
	})
	row = make(map[string]any)
}
if err := iter.Close(); err != nil {
	return "Could not finish reading users", http.StatusInternalServerError,
		fmt.Errorf("close users iterator: %w", err)
}
```

The table's partition-key and clustering-key design determines which CQL filters, ordering, and grouping are legal. Native CQL does not bypass ScyllaDB's data-model rules. Check the [ScyllaDB query limitations](orm.md#7-driver-feature-matrix) and ScyllaDB's documentation before designing a query that scans or groups non-key columns.

Use `ScanCQL()` for conditional statements such as `UPDATE ... IF EXISTS`. A `false` applied result with no error means the condition did not hold; map that outcome to the business result you need, such as not found or conflict. For queries not covered by these helpers, `RawQueryScylla()` passes the underlying session directly to your callback.

## Keep the choice local to each operation

An application does not need to choose ORM or native exclusively. A practical pattern is to use ORM for common CRUD and use native APIs only for operations whose requirements do not fit the ORM or need explicit driver behavior. Keep the boundary visible in code review: document why a native operation is used, bind values, close resources, classify errors, and verify the resulting database state.

The final decision belongs to the application team. Consider the business requirements, selected database, performance measurements, portability needs, and the team's ability to maintain driver-specific code.
