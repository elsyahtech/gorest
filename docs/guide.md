## 1. Logger Guide
Gorest integrated with Zap as a core logger.

* **Overview:** A high-performance, built-in logging system designed to give you absolute operational visibility without sacrificing speed. 
* **Key Benefits:** Zero-allocation philosophy, strict structured logging, and automated log file rotation out-of-the-box. Every operational action (server startup, database migrations, security warnings) is safely tracked.
* **Sample Output Preview:**
```text
{"level":"info","timestamp":"2026-10-06T22:44:31.036+0700","message":"starting gorest","service":"My App"}
{"level":"info","timestamp":"2026-10-06T22:44:31.036+0700","message":"starting server","service":"My App"}
{"level":"info","timestamp":"2026-10-06T22:44:31.138+0700","message":"Endpoints running ['GET /', 'GET /users', 'GET /users/:id', 'GET /product/', 'GET /product/:id', 'POST /product/']","service":"My App"}
{"level":"info","timestamp":"2026-10-06T22:44:31.138+0700","message":"server started successfully","service":"My App"}
{"level":"info","timestamp":"2026-10-06T22:44:31.138+0700","message":"gorest is running","service":"My App"}
```
Read the full logger technical breakdown: [Logger Technical Guide](./guide/logger.md)

---

## 2. Server & Router Guide

* **Overview:** Built on top of the lightning-fast Fiber HTTP engine, Gorest provides an intuitive, robust routing and middleware ecosystem.
* **Key Benefits:** Blazing-fast request context handling, clean modular route grouping, built-in security middlewares (OAuth, CORS, Recovery), and seamless ecosystem integration.
* **Sample Output Preview:**

<img src="./bootable-screen.png" alt="Bootable Preview" width="600">

Read the full Server & Router technical breakdown: [Router Technical Guide](./guide/router.md)

---

## 3. Database, ORM & Querying Guide
* **Overview:** The crown jewel of Gorest. A multi-driver data persistence layer featuring auto-migrations, smart seed protection, and a custom reflection-powered ORM.
* **Key Benefits:** Supports 7 database drivers (MySQL, PostgreSQL, SQL Server, Oracle, SQLite, MongoDB, ScyllaDB) with ORM features such as custom `gorest` tags and `Preload()`, plus native query APIs. The best fit depends on the operation; the Querying Guide compares their tradeoffs and explains when each may be appropriate.
* **Sample Output Preview:**

Start with the [Querying Guide](./guide/querying.md) to compare ORM and native queries, then use the [ORM Tutorial](./guide/orm-tutorial.md) for a guided ORM path. The [Database Guide](./guide/database.md) covers connection and schema setup; the [ORM Guide](./guide/orm.md) is the ORM feature reference.

---

## 4. Database Migration & Seeder

### How to Run Migration:

By default, migrations and seeders can be triggered automatically upon application startup by simply setting `Migration: true` and `Seeder: true` inside your database configuration block in `main.go`:
```go
database.Run(database.Config{
     Driver:    database.SQLITE,
     Name:      "default_gorest",
     Migration: true, // Set to true to auto-run migrations on boot
     Seeder:    true, // Set to true to auto-run seeders on boot
})
```

### Startup behavior when migration or seeding fails

When enabled, Gorest runs database startup tasks synchronously before starting the HTTP server. Migration runs before seeding when both are enabled. The `Migration` and `Seeder` flags are independent, so seeding can be enabled without running migrations.

If a migration or seeder returns an error, Gorest logs the failure and aborts application startup; the application is not started. If migration fails, seeding is skipped. Check the logged error and message, resolve the database or script issue, then restart the application. A failed task may already have made database changes; rollback behavior depends on the database driver and migration script.

> 🛡️ **Smart Engine Protection (No Overwrite Worries):**
> You don't need to worry about accidentally wiping your production or development data if you leave migration or seeder flags enabled (`true`) by accident:
> * **For Migrations:** Gorest is smart enough to check table existence. If a table does not exist, it will automatically generate it. If the table already exists, Gorest safely ignores it.
> * **For Seeders:** Similarly, if a table has already been seeded previously, Gorest will automatically ignore subsequent seeding attempts even if the seeder flag remains active. Your existing data is 100% safe from being overwritten!

---

### Create Migration File:

Create a new directory in your project root with the structure shown below. You can adjust it based on the database driver you choose.

```text
./database/
├── migrations/
│   ├── mongo/
│   ├── mysql/
│   ├── oracle/
│   ├── postgres/
│   ├── scylla/
│   ├── sqlite/
│   └── sqlserver/
└── seeders/
    ├── mongo/
    ├── mysql/
    ├── oracle/
    ├── postgres/
    ├── scylla/
    ├── sqlite/
    └── sqlserver/
```

Create .sql, .cql, or .json files according to your chosen database driver inside each of those respective directories.

Read Schema Sample for migration and seeder needs [SQL Schema](./guide/database/schema.md)

---

## 5. Defining Entities & Struct

Gorest uses the `gorest` struct tag to map Go fields to database columns or document fields. The first item is the database field name; comma-separated items after it are options. The `json` tag is separate and controls JSON input/output names.

```go
type User struct {
	ID           string      `gorest:"id, primary_key, required" json:"id"`
	Email        string      `gorest:"email, required" json:"email"`
	Password     string      `gorest:"password, required" json:"password"`
	DepartmentID string      `gorest:"department_id, required" json:"departmentId"`
	IsActive     bool        `gorest:"is_active" json:"isActive"`
	CreatedAt    time.Time   `gorest:"created_at, immutable" json:"createAt"`
	UpdatedAt    *time.Time  `gorest:"updated_at" json:"updatedAt"`
	DeletedAt    *time.Time  `gorest:"deleted_at" json:"deletedAt"`
	Department   *Department `gorest:"-" references:"ID" json:"department,omitempty"`
}
```

### Tag items

1. **Database field name** — The first item maps the Go field to a column/document field. For example, `DepartmentID` maps to `department_id` with `gorest:"department_id"`. Use `gorest:"-"` to exclude a field from ORM mapping.
2. **`primary_key`** — Marks the mapped field as a primary key for ORM operations that need to identify a record, such as update and delete. It does not change the field name.
3. **`required`** — Opt-in validation performed before `Create()`. Add it to fields that are required by your database schema. Required strings cannot be empty or whitespace-only; nil pointers and empty slices/maps are rejected. This catches an omitted JSON string field, which decodes to `""` in Go. A SQL `NOT NULL` constraint rejects SQL `NULL`, but most SQL databases still accept `''`, so `required` prevents that empty value from being silently inserted. Fields without this option keep their existing behavior. `false` and numeric zero remain valid values; use pointer fields when you need to distinguish an omitted zero-valued field from an explicitly supplied one.
4. **`immutable`** — Prevents the field from being changed by ORM `Update()` operations. Use it for values such as `created_at` that should not change after insertion.
5. **`object_id`** — MongoDB option for query filters. If the mapped field is tagged with `object_id`, a valid 24-character hexadecimal string passed as a filter value is converted to BSON `ObjectID`. This option does not convert values during `Create()`.
6. **`references`** — Relationship metadata used with relation fields. For example, `gorest:"-" references:"ID"` excludes the relation object as a database column and identifies the referenced field for preload mapping.

### MongoDB `object_id` and `_id`

The first tag item is the actual MongoDB field name. Therefore, `gorest:"id, primary_key, object_id"` refers to the document's `id` field; it does not refer to MongoDB's built-in `_id` field. A handler variable named `id` does not change the queried field. The field in `Where()` must match the field mapped by the tag.

If `id` stores an application UUID such as `e0d62583-2d5a-4817-aa9e-fa9cc257cc01`, keep it as a string and omit `object_id`:

```go
ID string `gorest:"id, primary_key, required" json:"id"`
```

MongoDB still has its separate `_id` field. To filter by that built-in field, map a struct field to `_id` and use `_id` in the query:

```go
ID string `gorest:"_id, primary_key, object_id" json:"id"`
// ORM query: Where("_id = ?", id)
```

The filter value must be a valid 24-character hex ObjectID. If a collection uses a string `_id`, do not use `object_id` for that field.

--- 

## 6. Response Handling Guide

* **Overview:** True to Gorest's philosophy—“A lightweight, opinionated framework for building websites, REST APIs, and backend services”—response rendering is completely flexible.
* **Key Benefits:** Unified response format for lightning-fast REST JSON APIs, alongside built-in server-side rendering support for traditional HTML web applications and templates.
* **Sample Output Preview:**

```text
// JSON API vs Traditional HTML Rendering
return ctx.JSON(gorest.Map{"status": "success", "data": users})

// OR
return ctx.Render("templates/index", gorest.Map{"Title": "Welcome to Gorest"})
```

Read the full response handler technical breakdown: [Response Handler Guide](./guide/response/view_response.md)
