# ORM Tutorial: Try a CRUD Flow, Then Explore the ORM

This tutorial gives you a small, manual CRUD path and then explains the choices behind it. It complements the [ORM reference guide](orm.md), which documents the full API and driver-specific details. For a neutral comparison of ORM and native queries, start with the [Querying Guide](querying.md).

There are two ways to try GOREST database support:

| Option | Use it to | Start here |
| --- | --- | --- |
| Automated integration suite | Repeat the provided scenarios and inspect pass/fail reports against real database services. | [Integration test guide](../../integration_test/README.md) |
| Manual CRUD walkthrough | Connect GOREST to a database you control, run one create/read/update/delete flow, and inspect the stored row. | Continue with this tutorial and the [schema examples](database/schema.md). |

The `main.go` example below is a small standalone manual runner for all seven drivers. It uses one shared ORM path for the five SQL drivers and dedicated paths for MongoDB and ScyllaDB. It performs one CRUD flow and removes its test record. Use a development database or a disposable database; do not point the example at production data.

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

## 2. Run one manual CRUD flow

Create the `users` table using the schema for your selected SQL driver in the [schema guide](database/schema.md). Copy the complete code into `main.go` in a Go module that can import GOREST. In a new directory, initialize the module and add GOREST before running it:

```sh
go mod init manual-gorest
go get github.com/elsyahtech/gorest
```

The example uses a generated ID and email so repeated runs do not collide with earlier test data. The password value is deliberately fake test data; this flow does not demonstrate authentication or password hashing.

```go
package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/elsyahtech/gorest"
	"github.com/elsyahtech/gorest/database"
)

type User struct {
	ID           string `gorest:"id, primary_key, required"`
	Email        string `gorest:"email, required"`
	Password     string `gorest:"password, required"`
	DepartmentID string `gorest:"department_id, required"`
	IsActive     bool   `gorest:"is_active"`
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	driver := strings.ToUpper(os.Getenv("GOREST_DB_DRIVER"))
	if driver == "" {
		driver = database.SQLITE
	}

	dbConfig := database.ConfigDefault
	dbConfig.Disable = false
	dbConfig.Driver = driver
	if value := os.Getenv("GOREST_DB_NAME"); value != "" {
		dbConfig.Name = value
	}
	dbConfig.Host = os.Getenv("GOREST_DB_HOST")
	dbConfig.Port = os.Getenv("GOREST_DB_PORT")
	dbConfig.Username = os.Getenv("GOREST_DB_USER")
	dbConfig.Password = os.Getenv("GOREST_DB_PASSWORD")
	dbConfig.LibDir = os.Getenv("GOREST_DB_LIBDIR")

	app := gorest.New(gorest.Config{AppName: "GOREST manual CRUD", Debug: true})
	app.Use(database.Run(dbConfig))
	defer func() {
		if message, _, err := app.Close(); err != nil {
			log.Printf("application cleanup failed: %s: %v", message, err)
		}
	}()

	switch driver {
	case database.MONGO:
		return RunMongoCRUD(app)
	case database.SCYLLA:
		return RunScyllaCRUD(app)
	default:
		return RunCRUD(app)
	}
}

func RunCRUD(app *gorest.App) error {
	id := fmt.Sprintf("manual-%d", time.Now().UnixNano())
	email := id + "@example.test"
	user := User{
		ID:           id,
		Email:        email,
		Password:     "test-only-value",
		DepartmentID: "dept-manual-test",
		IsActive:     true,
	}

	created := app.Database().Table("users").Create(&user)
	if created.Error != nil {
		return fmt.Errorf("create user (HTTP %d): %w", created.HTTPCode, created.Error)
	}

	// Always try to remove the disposable row, including when a later step fails.
	cleanupNeeded := true
	defer func() {
		if !cleanupNeeded {
			return
		}
		deleted := app.Database().Table("users").Delete(&User{ID: id})
		if deleted.Error != nil {
			app.Log(gorest.Map{"error": deleted.Error, "id": id}).Error("manual CRUD cleanup failed")
		}
	}()

	var loaded User
	read := app.Database().Table("users").Where("id = ?", id).Find(&loaded)
	if read.Error != nil {
		return fmt.Errorf("read user (HTTP %d): %w", read.HTTPCode, read.Error)
	}
	if loaded.Email != email {
		return fmt.Errorf("read returned email %q, want %q", loaded.Email, email)
	}

	updatedEmail := id + "-updated@example.test"
	patch := User{ID: id, Email: updatedEmail}
	updated := app.Database().Table("users").
		Where("id = ?", id).
		Select("email").
		Update(&patch)
	if updated.Error != nil {
		return fmt.Errorf("update user (HTTP %d): %w", updated.HTTPCode, updated.Error)
	}

	loaded = User{}
	read = app.Database().Table("users").Where("id = ?", id).Find(&loaded)
	if read.Error != nil {
		return fmt.Errorf("verify update (HTTP %d): %w", read.HTTPCode, read.Error)
	}
	if loaded.Email != updatedEmail {
		return fmt.Errorf("updated email is %q, want %q", loaded.Email, updatedEmail)
	}

	deleted := app.Database().Table("users").Delete(&User{ID: id})
	if deleted.Error != nil {
		return fmt.Errorf("delete user (HTTP %d): %w", deleted.HTTPCode, deleted.Error)
	}
	cleanupNeeded = false

	var missing User
	verifyDelete := app.Database().Table("users").Where("id = ?", id).Find(&missing)
	if verifyDelete.Error == nil || verifyDelete.HTTPCode != http.StatusNotFound {
		return errors.New("delete verification did not return HTTP 404")
	}

	fmt.Printf("CRUD passed for user %s; cleanup is complete\n", id)
	return nil
}

type MongoUser struct {
	ID       string `gorest:"_id, primary_key, required"`
	Email    string `gorest:"email, required"`
	Name     string `gorest:"name, required"`
	IsActive bool   `gorest:"is_active"`
}

func RunMongoCRUD(app *gorest.App) error {
	id := fmt.Sprintf("manual-%d", time.Now().UnixNano())
	user := MongoUser{ID: id, Email: id + "@example.test", Name: "Manual Test", IsActive: true}

	created := app.Database().Table("gorest_manual_users").Create(&user)
	if created.Error != nil {
		return fmt.Errorf("create MongoDB document (HTTP %d): %w", created.HTTPCode, created.Error)
	}

	cleanupNeeded := true
	defer func() {
		if cleanupNeeded {
			deleted := app.Database().Table("gorest_manual_users").Delete(&MongoUser{ID: id})
			if deleted.Error != nil {
				app.Log(gorest.Map{"error": deleted.Error, "id": id}).Error("manual MongoDB cleanup failed")
			}
		}
	}()

	var loaded MongoUser
	read := app.Database().Table("gorest_manual_users").Where("_id = ?", id).Find(&loaded)
	if read.Error != nil {
		return fmt.Errorf("read MongoDB document (HTTP %d): %w", read.HTTPCode, read.Error)
	}
	if loaded.Email != user.Email {
		return fmt.Errorf("MongoDB email is %q, want %q", loaded.Email, user.Email)
	}

	updatedEmail := id + "-updated@example.test"
	patch := MongoUser{ID: id, Email: updatedEmail}
	updated := app.Database().Table("gorest_manual_users").
		Where("_id = ?", id).Select("email").Update(&patch)
	if updated.Error != nil {
		return fmt.Errorf("update MongoDB document (HTTP %d): %w", updated.HTTPCode, updated.Error)
	}
	loaded = MongoUser{}
	read = app.Database().Table("gorest_manual_users").Where("_id = ?", id).Find(&loaded)
	if read.Error != nil {
		return fmt.Errorf("verify MongoDB update (HTTP %d): %w", read.HTTPCode, read.Error)
	}
	if loaded.Email != updatedEmail {
		return fmt.Errorf("MongoDB email is %q, want %q", loaded.Email, updatedEmail)
	}

	deleted := app.Database().Table("gorest_manual_users").Delete(&MongoUser{ID: id})
	if deleted.Error != nil {
		return fmt.Errorf("delete MongoDB document (HTTP %d): %w", deleted.HTTPCode, deleted.Error)
	}
	cleanupNeeded = false

	var missing MongoUser
	verifyDelete := app.Database().Table("gorest_manual_users").Where("_id = ?", id).Find(&missing)
	if verifyDelete.Error == nil || verifyDelete.HTTPCode != http.StatusNotFound {
		return errors.New("MongoDB delete verification did not return HTTP 404")
	}

	fmt.Printf("MongoDB CRUD passed for document %s\n", id)
	return nil
}

type ScyllaUser struct {
	ID           string `gorest:"id, primary_key, required"`
	Email        string `gorest:"email, required"`
	Password     string `gorest:"password, required"`
	DepartmentID string `gorest:"department_id, required"`
	IsActive     bool   `gorest:"is_active"`
	IsDeleted    bool   `gorest:"is_deleted"`
}

func RunScyllaCRUD(app *gorest.App) error {
	id := fmt.Sprintf("manual-%d", time.Now().UnixNano())
	user := ScyllaUser{
		ID: id, Email: id + "@example.test", Password: "test-only-value",
		DepartmentID: "dept-manual-test", IsActive: true,
	}

	created := app.Database().Table("users").Create(&user)
	if created.Error != nil {
		return fmt.Errorf("create ScyllaDB row (HTTP %d): %w", created.HTTPCode, created.Error)
	}

	cleanupNeeded := true
	defer func() {
		if cleanupNeeded {
			deleted := app.Database().Table("users").Delete(&ScyllaUser{ID: id})
			if deleted.Error != nil {
				app.Log(gorest.Map{"error": deleted.Error, "id": id}).Error("manual ScyllaDB cleanup failed")
			}
		}
	}()

	var loaded ScyllaUser
	read := app.Database().Table("users").Where("id = ?", id).Find(&loaded)
	if read.Error != nil {
		return fmt.Errorf("read ScyllaDB row (HTTP %d): %w", read.HTTPCode, read.Error)
	}
	if loaded.Email != user.Email {
		return fmt.Errorf("ScyllaDB email is %q, want %q", loaded.Email, user.Email)
	}

	updatedEmail := id + "-updated@example.test"
	patch := ScyllaUser{ID: id, Email: updatedEmail}
	updated := app.Database().Table("users").Where("id = ?", id).
		Select("email").Update(&patch)
	if updated.Error != nil {
		return fmt.Errorf("update ScyllaDB row (HTTP %d): %w", updated.HTTPCode, updated.Error)
	}
	loaded = ScyllaUser{}
	read = app.Database().Table("users").Where("id = ?", id).Find(&loaded)
	if read.Error != nil {
		return fmt.Errorf("verify ScyllaDB update (HTTP %d): %w", read.HTTPCode, read.Error)
	}
	if loaded.Email != updatedEmail {
		return fmt.Errorf("ScyllaDB email is %q, want %q", loaded.Email, updatedEmail)
	}

	deleted := app.Database().Table("users").Delete(&ScyllaUser{ID: id})
	if deleted.Error != nil {
		return fmt.Errorf("delete ScyllaDB row (HTTP %d): %w", deleted.HTTPCode, deleted.Error)
	}
	cleanupNeeded = false

	var missing ScyllaUser
	verifyDelete := app.Database().Table("users").Where("id = ?", id).Find(&missing)
	if verifyDelete.Error == nil || verifyDelete.HTTPCode != http.StatusNotFound {
		return errors.New("ScyllaDB delete verification did not return HTTP 404")
	}

	fmt.Printf("ScyllaDB CRUD passed for row %s\n", id)
	return nil
}
```

The runner selects the code path from `GOREST_DB_DRIVER` (`SQLITE` by default). The SQL table must have the columns used above, with defaults for any additional `NOT NULL` columns. MongoDB creates a separate `gorest_manual_users` collection on first insert; use an environment/database where that collection is not subject to a conflicting validator. The ScyllaDB path expects the `users` table from the [ScyllaDB schema example](database/schema.md#scylla-db-cassandra), with `id` as its partition key. Each path uses a generated key, checks every operation, verifies the deleted record returns HTTP 404, and attempts cleanup after an early failure.

Run SQLite with:

```sh
GOREST_DB_DRIVER=SQLITE GOREST_DB_NAME=gorest_manual go run main.go
```

For a remote SQL database, MongoDB, or ScyllaDB, also set `GOREST_DB_HOST`, `GOREST_DB_PORT`, `GOREST_DB_USER`, `GOREST_DB_PASSWORD`, and `GOREST_DB_NAME` as required by that driver. The example reads credentials from the environment; do not commit real secrets. This is a development sample and leaves TLS at the configured default. Configure and verify the driver's TLS settings before connecting across an untrusted network; see the [Database Guide](database.md).

For example, after exporting the connection variables for your service, select the driver and run the same file:

```sh
GOREST_DB_DRIVER=POSTGRES go run main.go
```

Use `MYSQL`, `SQLSERVER`, `ORACLE`, `SQLITE`, `MONGO`, or `SCYLLA` for the other drivers. Oracle may also require `GOREST_DB_LIBDIR` to point to Oracle Instant Client. The SQL drivers use the `users` schema from the schema guide; ScyllaDB also uses its documented `users` table, while MongoDB creates a separate sample collection.

This manual runner exercises the ORM. Native query examples and their additional responsibilities are in the [Querying Guide](querying.md). For a ready-made automated run with disposable database containers, use the [managed HTTP integration runners](../../integration_test/README.md#managed-http-end-to-end-runners).

## 3. Read data

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

## 4. Create, update, and delete

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

Each row in a bulk update must have a distinct primary key. Duplicate keys are rejected with HTTP 400 and listed in the error because multiple payloads for one record would make the update ambiguous. Gorest then checks that all supplied IDs exist. If one or more are missing, it returns HTTP 404 and lists the missing IDs; no updates are issued. SQL and MongoDB perform this check in one lookup, while ScyllaDB checks each full primary key. This prevents a missing ID from being silently ignored while the other rows update.

The preflight check handles missing IDs, but bulk updates can still partially complete if a later write fails or another process changes the records concurrently. SQL callers can use a transaction where supported. Review the [driver-specific bulk update notes](orm.md#update) before relying on atomic behavior.

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

Do not infer which fields are updated from the method name. A payload's zero values may overwrite existing values, and conflicting keys outside the specified target can still fail. Verify the before/after behavior for the target driver; details are in [Upsert](orm.md#upsert).

## 5. Exercise failure paths

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

## 6. Diagnose and verify

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

## Driver-specific considerations

The SQL path uses a string primary key and the `users` table schemas shared by the five SQL examples. The MongoDB path uses a string `_id` in a separate collection; a UUID in an `id` document field is a different mapping. ScyllaDB requires filters and writes to respect the table's primary-key design; an ordinary non-key email column is not unique. See the [MongoDB primary-key notes](../guide.md#mongodb-object_id-and-_id), [ScyllaDB schema example](database/schema.md#scylla-db-cassandra), and [driver feature matrix](orm.md#7-driver-feature-matrix) before adapting the sample.

If you want native queries instead, use the [Querying Guide](querying.md) for SQL, MongoDB, and ScyllaDB examples. Native APIs require the application to own query text, parameter syntax, result mapping, resource cleanup, and driver-specific error handling.

## Continue learning

- [ORM reference: all features, limits, and driver matrix](orm.md)
- [Database connection and schema setup](database.md)
- [Gorest struct tags](../guide.md#entity-mapping)
