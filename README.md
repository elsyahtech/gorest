# GOREST

An opinionated Go application framework for building websites, REST APIs, and backend services.

GOREST provides a structured application foundation with integrated infrastructure for HTTP servers, databases and data access.

Its goal is simple: reduce repetitive infrastructure work, establish consistent application conventions, and help development teams focus on business functionality without giving up control of Go.



---

## ⚙️ Requirements

GOREST requires **Go 1.26.5** or higher.

Additional requirements depend on the components used by your application.

If you need to install or upgrade Go, visit the [official Go download page](https://go.dev/dl/).

### Supported Databases
GOREST provides modular support for:
- Oracle
- SQL Server
- PostgreSQL
- MySQL
- SQLite
- MongoDB
- ScyllaDB

---

## 💡 Why GOREST?

Go gives developers a simple language, strong performance, and the freedom to choose their own tools. However, building maintainable backend applications also requires consistent conventions for application structure, configuration, logging, data access, and lifecycle management.

GOREST provides an opinionated foundation for these recurring concerns, helping teams reduce repetitive infrastructure work and keep applications consistent as they grow.

### What GOREST Provides

- **Consistent Application Structure** — Establishes predictable conventions that make projects easier to navigate, maintain, and hand over between developers.
- **Integrated Infrastructure** — Brings together application lifecycle management, HTTP server integration, configuration, structured logging, Redis, and database connectivity.
- **Flexible Data Access** — Supports both Native SQL and ORM, allowing developers to choose the appropriate abstraction for each use case.
- **Explicit and Predictable Behavior** — Validates configuration and query-related constraints where applicable, helping detect problems early instead of silently guessing developer intent.
- **Developer Control** — Keeps application and business logic in regular Go code. Developers can still use the standard library and other packages whenever GOREST's abstractions are not suitable.

GOREST provides the foundation, not a replacement for Go. Its purpose is to reduce repetitive infrastructure work while leaving implementation decisions and business requirements under the developer's control.

---

### Focus on Business Logic
GOREST handles common application infrastructure so developers can spend more time implementing application-specific requirements.

Depending on the application, GOREST provides:
- HTTP server and routing integration
- Centralized application bootstrapping
- Configuration management
- Structured logging
- Redis integration
- Database connection management
- Database migrations and seeders
- Native SQL and ORM-based data access
- REST API and Web view support
- Application lifecycle management

The application remains regular Go code. Developers can still use Go's standard library and other packages when a requirement falls outside GOREST's abstractions.

---

## 🛠️ What GOREST Provides

### Opinionated Application Structure
GOREST provides a consistent application structure and conventions for organizing backend applications.

A common structure can help teams:
- onboard new developers faster
- make projects easier to navigate
- reduce architectural differences between applications
- make knowledge easier to transfer between team members
- reduce dependency on individual developers

The goal is not to enforce every implementation detail.

The goal is to establish a predictable foundation for the parts of an application that are commonly repeated.

---

### Fail-Fast Configuration
GOREST validates important application and database configuration during initialization and query construction where applicable.

The intention is to detect invalid configuration and query-related problems as early as possible instead of allowing invalid state to propagate deeper into application execution.

---

### Native SQL and ORM
GOREST supports both Native SQL and ORM-based data access.
Use Native SQL when you need:
* direct control over SQL
* database-specific capabilities
* complex queries
* predictable SQL behavior
* minimal abstraction

Use the ORM when you need:

* structured model mapping
* query building
* relationships
* preloading
* filtering
* ordering
* pagination
* parameter binding

This allows developers to choose the appropriate abstraction level for each use case instead of forcing every query through a single approach.

---

### Integrated Application Lifecycle
GOREST provides a centralized application lifecycle for initializing infrastructure and starting the application.

```text
Install
    ↓
Configuration
    ↓
Logging
    ↓
Database / Redis
    ↓
HTTP Server
    ↓
Application
```

The specific components used depend on the module you need.

---

## 🛠️ Features

### 1. Application Lifecycle & Initialization

- **Centralized Bootstrapping** — Coordinates application initialization and startup.
- **Thread-Safe Initialization** — Protects critical initialization paths from concurrent execution.
- **Startup Timeout** — Uses a startup context timeout to prevent initialization from waiting indefinitely on slow external dependencies.
- **Database Timezone Configuration** — Passes the configured timezone to database drivers that support it without changing the process-wide timezone.

### 2. Integrated Infrastructure

- **HTTP Server & Routing** — Integrates HTTP server and route registration into the application lifecycle.
- **Structured Logging** — Provides a common logging foundation across application components.
- **Redis Integration** — Supports Redis connectivity for caching, sessions, and key-value operations.
- **Multi-Database Support** — Provides modular adapters for Oracle, SQL Server, PostgreSQL, MySQL, SQLite, MongoDB, and ScyllaDB.

### 3. Database Migrations & Seeders

- **Database Migrations** — Supports controlled schema migration workflows.
- **Database Seeders** — Supports inserting initial or development data through application-managed seeders.
- **Driver-Specific Migrations** — Accommodates the capabilities and requirements of individual database drivers.

### 4. Data Access & ORM

- **Native SQL** — Allows direct SQL execution and database-specific query behavior.
- **Query Builder & ORM** — Supports model mapping, query construction, filtering, ordering, pagination, and parameter binding.
- **Relationship Preloading** — Supports configured relationships and preloading.
- **Explicit Relationship Configuration** — Validates relationship metadata where applicable instead of relying on silent inference.
- **Pluralization Support** — Handles common and irregular pluralization rules when mapping models to tables.
- **Exact Column Matching** — Validates selected columns and aliases through exact matching.
- **SQL-Aware Parameter Parsing** — Accounts for SQL string literals, comments, and PostgreSQL dollar-quoted sections when parsing placeholders.
- **Strict Parameter Binding** — Validates placeholder and argument alignment before query execution.

---

## ⚙️ Installation & Getting Started

To get started with **Gorest**, follow these simple steps to set up your local development environment.

### 1. Create new project
```bash
mkdir myapp
```

```bash
cd myapp
```

```bash
go mod init myapp
```

```bash
go get github.com/elsyahtech/gorest
```

---

### 2. Create `main.go`

The simplest way to start a GOREST application is to use its built-in defaults. You do not need to register the logger or HTTP server manually.

```go
package main

import "github.com/elsyahtech/gorest"

func main() {
    app := gorest.New()
    app.Start()
}
```

### 3. Run the application

```bash
go run main.go
```

Open `http://127.0.0.1:3000` in your browser.

<img src="./docs/home-page.png" alt="Gorest Home Page" width="600">

GOREST initializes the logger and HTTP server with their default configurations. SQLite is the default database driver, so no external database server is required for the basic setup.

**This quick start uses defaults.** You only need to configure individual subsystems when your application requires different settings.

---

### Looking for a Complete Application Using Gorest?
The minimal setup above is all you need to get GOREST running. If you want to explore a more complete application with practical implementation examples, check out the official starter project:

👉 **[Golang REST API Starter](https://github.com/elsyahtech/golang-rest-api-starter)**

The starter provides practical examples of CRUD operations, database transactions, migrations, and native database queries to help you understand how to build an application with GOREST.

---

## 🔧 Customize When You Need To

When you need to customize application settings, you can pass a configuration object to gorest.New(...). This keeps configuration explicit and accessible in your Go code.

---

### 1. Application Core Configuration (gorest.Config)
```go
app := gorest.New(gorest.Config{
    // Application identifier, ideal for microservices architecture, service discovery, and tracing.
    AppName: "My App",

    // Version tag, commonly used for CI/CD pipelines, release tracking, and artifact management.
    Version: "1.0.0",

    // Operating environment mode (e.g., "development", "staging", "production").
    Environment: "development",

    // Timezone used by database drivers that support application timezone configuration.
    Timezone: "Asia/Jakarta",

    // Debug mode. When true, the HTTP server is bypassed and only the CLI/console runner executes,
    Debug: false,
})
```

- **AppName:** Identifies your service name. Crucial for microservice architectures, log tracing, and service meshes.
- **Version:** Tracks your deployment version, heavily integrated into CI/CD release tagging.
- **Environment:** Controls runtime behaviors (development, staging, production).
- **Timezone:** Configures the timezone for database drivers that support it (currently MySQL and Oracle). It does not change Go's process-wide `time.Local`; configure timezone handling explicitly in application code for logs, token expiration, and other values.
- **Debug:** When set to true, halts the HTTP server listener so developers can safely run local debugging, dry-run SQL migrations, or test data seeders via console.

---

### 2. Custom Subsystem Configuration (`app.Use`)

Use `app.Use(...)` when you need to register or customize subsystem configurations. It accepts supported configuration values or pointers, including `log.Config`, `database.Config`, `redis.Config`, and `server.Config`.

The logger and HTTP server are already initialized by `gorest.New()`. Register them again only when you want to apply explicit subsystem configurations.

#### Configure the database

The following example explicitly registers the default database configuration. SQLite remains the selected driver unless you change it.

```go
package main

import (
    "github.com/elsyahtech/gorest"
    "github.com/elsyahtech/gorest/database"
)

func main() {
    app := gorest.New()

    dbConfig := database.ConfigDefault
    dbConfig.Driver = database.SQLITE

    app.Use(database.Run(dbConfig))

    app.Start()
}
```

`database.Run(config)` applies database configuration defaults and returns the normalized configuration. It does not establish the database connection by itself; `app.Use(...)` registers the returned configuration with the application.

When starting from `database.ConfigDefault`, the database is enabled by default. If you instead construct a partial `database.Config` from scratch, set `Enabled: true` explicitly when you want `app.Use(...)` to initialize it.

#### Configure Redis only when needed

Redis is optional and disabled by default. To enable it, set `Enabled: true` and ensure the Redis service is reachable.

```go
redisConfig := redis.ConfigDefault
redisConfig.Enabled = true

app.Use(redis.Run(redisConfig))
```

Add the `github.com/elsyahtech/gorest/redis` import to use this example. Configure the host, credentials, and TLS settings as required by your environment.

#### Customize logging or the HTTP server

You can pass explicit configurations when you need to change their settings:

```go
app.Use(
    log.Run(log.ConfigDefault),
    server.Run(server.ConfigDefault),
)
```

This example reapplies the default logger and server settings; it is not required for a basic application. To customize them, modify the corresponding configuration values before passing them to `app.Use(...)`.

For configuration fields, environment-specific settings, database drivers, migrations, and seeders, see the [Configuration Guide](./docs/configuration.md).

---

### 3. Server & Router (app.Use(...))

```go
routers := []server.RouterRegistrar{
    UserRoutes,
    ProductRoutes,
    // Add more modular route registrars here effortlessly.
}
app.RegisterServices(routers)
```

See server & router configuration detail: [Router Technical Guide](./docs/guide/router.md)

---

# 🔧 Configuration
Read the full configuration breakdown: [Configuration Documentation](./docs/configuration.md)

---

# 📚 Guides
Read the full guide: [Guide Documentation](./docs/guide.md)

# 🔧 Troubleshoot
Read the full troubleshoot guide: [Troubleshoot Documentation](./docs/guide.md)

# 🎧 Support

If you need help, have questions, or want to report an issue, you can reach out through the following channels:

* **Issues & Bug Reports:** Use the [GitHub Issues](https://github.com/elsyahtech/gorest/issues) page to report bugs or request new features.
* **Security Vulnerabilities:** Report suspected vulnerabilities privately using the process in [SECURITY.md](./SECURITY.md); do not post them in public issues.
* **Discussions:** Join our [GitHub Discussions](https://github.com/elsyahtech/gorest/discussions) to ask questions, share ideas, or discuss best practices with other community members.
* **Documentation:** Explore the full [Guides & Documentation](./docs/guide.md) for detailed architecture, configuration, and API references.

### Contributing
Contributions are always welcome! If you'd like to contribute to GOREST, please check out our [Contribution Guidelines](./CONTRIBUTING.md) to get started.

# Partner
Hello elmansyah, thanks a lot.