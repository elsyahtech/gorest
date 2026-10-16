## 🔧 Detailed Configuration Guide

### 1. Logger Configuration (`log.Run(...)`)
In Gorest, logging is a mandatory service and cannot be disabled. GOREST emits its lifecycle and framework messages through Zap; application request and business events are logged only where the application or middleware explicitly emits them.

By default, Gorest automatically initializes a default logger and creates a `logs/` directory at your root path, saving logs directly to `./logs/app.log`.

You can customize the logger behavior by passing a configuration struct:

```go
log.Run(log.Config{
    LogDir:      "./logs",     // Root directory where log files are stored. Automatically created on startup if it doesn't exist.
    LogFile:     "app.log",    // Filename for general/info level logs.
    Caller:      true,         // Enables caller details (file path and line number) in log entries for easier debugging.
    MaxFileSize: 100,          // Maximum size in megabytes (MB) before a log file triggers rotation (split into a new file). Default: 100 MB.
    MaxBackups:  5,            // Maximum number of old log files to retain after rotation.
    MaxAge:      7,            // Maximum number of days to retain old log files based on filename timestamps.
})
```

Logger Parameter Breakdown:
- **LogDir:** The target folder for your log outputs (e.g., ./logs or /var/log/myapp). If the directory is missing, Gorest builds it automatically during startup.
- **LogFile:** The base filename for standard info logs (default is app.log).
- **Caller:** When enabled, appends code execution pointers (exact filenames and line numbers) to your logs—super helpful during development and tracing.
- **MaxFileSize:** Prevents log files from growing infinitely by defining a size limit (in MB) before rotation occurs.
- **MaxBackups:** Controls how many historical backup files are kept on disk, protecting your storage space from overflowing.
- **MaxAge:** Automatically purges log backups older than a specified number of days (e.g., deleting logs older than 7 days).

The logger rotates the configured file by size and enforces the configured backup count and age during rotation. New log directories are created with mode `0700`; new log files use mode `0600`. Existing directories and existing log-file permissions are not tightened automatically, so verify their ownership and access controls in deployment. One process should own a given log file; do not have multiple application processes write to the same rotating file.

`app.Close()` calls Zap's `Sync()` and closes the rotating file writer. The rotating sink does not expose an `fsync` operation, so this is not a guarantee that data has been durably committed to physical storage. Zap does not return a write error from each `Info`/`Error` call; output write failures are reported through Zap's internal error output (stderr by default), and logging calls do not make business operations transactional or durable. The logger initialized by GOREST and exposed through `app.Log(...)` redacts recognized sensitive field names and common message patterns (credential assignments, Bearer/Basic authorization values, JWTs, email addresses, and phone numbers) to `xxxxx`, including fields attached with Zap's `With(...)`. This does not affect separate logger instances or other logging libraries. This heuristic cannot reliably identify arbitrary names, addresses, identifiers, custom encodings, or secrets without recognizable labels, and it may produce false positives. Do not rely on it as a substitute for data minimization; avoid logging credentials, session data, and unnecessary personal data, and redact application-specific values before passing them to `app.Log(...)`.

### 2. HTTP Server Configuration (`server.Run(...)`)
For the HTTP layer, Gorest integrates **Fiber** as its core HTTP engine under the hood, bringing blazing-fast performance, low memory footprint, and an expressive routing API to your application lifecycle.

By default, the server runs smoothly out-of-the-box with sensible defaults. However, you can deeply customize the server engine, address, port, and security/performance middleware using `server.Config`:

When `Core` is omitted (`nil`), Gorest sets finite Fiber timeouts: `ReadTimeout` to 10 seconds, `WriteTimeout` to 30 seconds, and `IdleTimeout` to 120 seconds. These defaults reduce exposure to slow or idle connections that can occupy server resources. If an application uses long-running responses or streaming (for example, SSE), choose timeouts appropriate to that workload. Supplying `Core` replaces the GOREST core defaults; GOREST preserves its values, including zero timeouts. In Fiber, a zero read or write timeout is unlimited, and a zero idle timeout falls back to the read timeout. When `server.New()` receives a `Core` with zero `ReadTimeout` or `WriteTimeout`, it preserves the values and logs a warning; it does not reject startup. Configure finite positive timeouts unless the unlimited behavior is intentional and documented for the application. On `SIGINT` or `SIGTERM`, `app.Start()` gracefully shuts down Fiber before closing Redis, database, and logger resources, waiting up to 30 seconds for active requests. A timeout is reported and remaining services are then closed; `app.Close()` uses the same shutdown sequence. Unit coverage that constructs a server directly verifies `app.Close()` waits for in-flight requests and stops the listener, while a separate helper-process integration test exercises `app.Start()` plus `SIGTERM`. These tests model local behavior and cannot guarantee identical signal delivery, traffic, dependency behavior, or termination timing in every application's deployment; verify shutdown with the application's actual process supervisor/container and real handlers.

`ReadTimeout` applies to reading the full request, including its body, so it bounds how long a slow client can take to send a request. `BodyLimit` limits request size, but does not limit how slowly those bytes can arrive; it is not a substitute for a finite `ReadTimeout`. GOREST's server tests include a local TCP test that sends an incomplete request body and verifies Fiber returns `408 Request Timeout` when a finite read timeout expires. This verifies the configured server behavior in the test environment, not behavior through every reverse proxy, load balancer, network, or production deployment.

Both `server.Run(...)` and `server.New(...)` apply GOREST defaults to omitted sections. This includes all six middleware configurations: CORS, Recover, Request ID, Compress, Helmet, and Limiter. Therefore, `server.New(&server.Config{})` installs the default middleware just like `server.New(nil)`; a nil middleware field means “use the GOREST default,” not “disable it.” `server.New(...)` currently has no nil-based middleware opt-out.

```go
server.Run(server.Config{
    // Server binding address
    // Default: "127.0.0.1"
    Host: "127.0.0.1",

    // Server running port
    // Default: 3000
    Port: 3000,

    // Optional: setting Core replaces the GOREST defaults above.
    // Explicit zero ReadTimeout/WriteTimeout values are preserved as unlimited
    // and cause server.New() to log a warning.
    Core: &fiber.Config{
        ReadTimeout:  10 * time.Second,
        WriteTimeout: 30 * time.Second,
        IdleTimeout:  120 * time.Second,
        BodyLimit:    DefaultServerBodyLimit,
    },

    // Cross-Origin Resource Sharing (CORS) middleware configuration
    Cors: &cors.Config{
        // AllowOrigins: "*",
    },

    // Panic recovery middleware configuration (catches panics to prevent server crashes)
    Recover: &recover.Config{},

    // Request ID generation middleware (injects a unique tracing ID for request tracking)
    Requestid: &requestid.Config{},

    // Response compression middleware (e.g., Gzip, Brotli to reduce payload size)
    Compress: &compress.Config{
        Level: compress.LevelBestSpeed,
    },

    // Security headers middleware (Helmet) to protect against common web vulnerabilities
    Helmet: &helmet.Config{},

    // Rate limiting middleware to restrict excessive requests within a time window
    Limiter: &limiter.Config{
        Max:        100,
        Expiration: 1 * time.Minute,
    },
})
```

HTTP Server Parameter Breakdown:
- **Host & Port:** Defines where your application listens for incoming traffic (default is 127.0.0.1:3000).
- **Core:** Direct access to underlying fiber.Config settings for advanced performance tuning.
- **Cors:** Controls cross-domain resource sharing policies easily.
- **Recover:** Essential safety net that catches unexpected panics during requests to keep your server alive.
- **Requestid:** Injects unique trace IDs for effortless request logging and distributed tracing.
- **Compress:** Reduces network bandwidth by compressing HTTP response payloads.
- **Helmet:** Automatically sets secure HTTP headers to defend your web app against common web vectors.
- **Limiter:** Built-in rate limiting to protect your endpoints from DDoS or abuse.

### 3. Redis Configuration (`redis.Run(...)`)
For caching, session management, and high-speed key-value operations, Gorest provides a robust Redis integration. It supports standard authentication, database index selection, connection pooling fine-tuning, and full **TLS/mTLS encryption** for secure enterprise environments.

You can configure Redis by passing `redis.Config`:

```go
redis.Run(redis.Config{
    // Connection Target & Auth
    Host:     "127.0.0.1",
    Port:     "6379",
    Username: "", // Leave blank if using password-only or no auth (Redis 6.0+ ACL)
    Password: "", // Leave blank if no password is required
    Database: 0,  // Redis database index number (usually 0 to 15)

    // Security & TLS
    TLS:         false,           // Set to true in production for secure encrypted connections
    SSLCAPath:   "",              // Path to CA certificate file for TLS verification
    SSLCertPath: "",              // Path to client certificate file for mutual TLS (mTLS)
    SSLKeyPath:  "",              // Path to client certificate private key file for mTLS

    // Timeouts & Lifetimes
    Timeout:         5 * time.Second,  // Dial, Read, and Write operation timeout limit
    PoolTimeout:     6 * time.Second,  // Time to wait for a connection if the pool is busy
    ConnMaxLifetime: 20 * time.Minute, // Maximum lifetime before a connection is closed and recreated
    ConnMaxIdleTime: 30 * time.Minute, // Maximum idle time before an unused connection is closed

    // Connection Pooling & Performance Tuning
    PoolSize:       0,                 // Maximum number of connections (recommended: 10 * runtime.GOMAXPROCS(0))
    MinIdleConns:   5,                 // Minimum idle connections to keep alive in the pool
    MaxActiveConns: 0,                 // Maximum active connections allocated simultaneously
    MaxRetries:     3,                 // Maximum retries for failed commands (-1 to disable)
    ReadBufferSize:  32 * 1024,        // bufio.Reader buffer size per connection (default: 32 KiB)
    WriteBufferSize: 32 * 1024,        // bufio.Writer buffer size per connection (default: 32 KiB)
})
```

Redis Parameter Breakdown:
- **Connection & Credentials** (Host, Port, Username, Password, Database): Directs the client to your Redis server instance with support for ACL usernames and isolated database indexes.
- **Security & TLS** (TLS, SSLCAPath, SSLCertPath, SSLKeyPath): Enables encrypted transport layers and mutual TLS authentication for secure cloud/cluster deployments.
- **Timeouts & Lifetimes** (Timeout, PoolTimeout, ConnMaxLifetime, ConnMaxIdleTime): Prevents hanging threads, socket leaks, and stale connections by managing strict duration limits.
- **Pool & Buffer Optimization** (PoolSize, MinIdleConns, MaxActiveConns, MaxRetries, ReadBufferSize, WriteBufferSize): Fine-tunes concurrency throughput. Note on PoolSize: When deploying inside Docker or Kubernetes, ensure your CPU container limits are set properly so Go's runtime.GOMAXPROCS doesn't misread the host's total core count!

When `Enabled` is true, `app.Use(redis.Run(config))` initializes Redis and verifies connectivity with `PING`; an initialization error is logged and aborts startup. When Redis is disabled or not registered, `app.Redis().QueryRedis()` returns an explicit unavailable-service error instead of a nil client. `app.Close()` closes an initialized Redis client during application shutdown.

`QueryRedis()` exposes the underlying go-redis client with command context deadlines enabled. Pass a request or operation context to every command; use `RedisSession.NewContext(parent)` to derive a context capped by the configured Redis `Timeout`, then cancel it when the operation finishes:

```go
session := app.Redis()
ctx, cancel := session.NewContext(parentCtx) // parentCtx should be request-scoped when available.
defer cancel()

client, message, code, err := session.QueryRedis()
if err != nil {
    return message, code, err
}

if err := client.Set(ctx, "cache:key", "value", time.Minute).Err(); err != nil {
    return "Redis command failed", 500, err
}
```

TLS verification is enabled by default when TLS is configured; GOREST does not set `InsecureSkipVerify`. `SSLCAPath` adds a CA pool for server verification. Configure both `SSLCertPath` and `SSLKeyPath` together when mutual TLS is required. Connection timeout, retry, and pool options are passed to go-redis, but they do not guarantee recovery from prolonged Redis outages; validate reconnect behavior, resource limits, and application fallback policy against the Redis version and deployment used by your service.

### 4. Database Configuration (`database.Run(...)`)
Gorest provides modular database adapters supporting both SQL and NoSQL environments (**PostgreSQL, MySQL, SQL Server, Oracle, SQLite, MongoDB, and ScyllaDB**). 

`database.ConfigDefault` selects SQLite as the default lightweight database driver. `app.New()` does not automatically register or open a database; register one with `app.Use(...)` as shown below. To use another engine, change the driver and its connection settings:

`database.Run(config)` applies database defaults and returns a normalized `database.Config`; it does not establish a connection. Pass its result to `app.Use(...)` to register and initialize the database. `app.Use(...)` does not call `database.Run(...)` automatically. When building a partial config literal, set `Enabled: true` explicitly or copy `database.ConfigDefault` and modify it; otherwise `Enabled` remains its Go zero value (`false`) and `app.Use(...)` skips database initialization.

GOREST accepts custom `Environment` labels such as `staging`, `uat`, or `preview`; `development`, `testing`, and `production` are conventions, not an allowlist. The TLS warning policy treats every non-empty environment other than `development` and `testing` as production-like, so custom deployment environments do not silently bypass the production database TLS warning.

For compatibility and ease of local development, an omitted `TLS` value defaults to disabled for network database drivers (MongoDB defaults to `false`; SQLite does not use TLS). Gorest does not automatically enable TLS based on the database host: TLS modes and certificate or wallet requirements differ by driver, and automatically enabling one could break existing local or legacy deployments. This is not a secure production default. A remote database connection without TLS can expose credentials and data to interception or modification on an untrusted network. Configure a driver-supported TLS mode explicitly for production and validate the server certificate; do not treat a private IP, VPN, or firewall as a replacement for transport encryption.

For an enabled database, `app.Start()` logs a driver-aware warning when the environment is production-like (anything except `development` or `testing`) and TLS is disabled, optional, lacks required certificate configuration, or cannot be verified from GOREST's TLS field (ScyllaDB). This is a warning, not a startup block; operators must still configure and verify encryption before deployment. Custom labels such as `staging` and `uat` receive the same warning policy.

Driver-specific notes: PostgreSQL supports `require`, `verify-ca`, and `verify-full` (prefer certificate and hostname validation with `verify-full`); SQL Server supports `mandatory` or `strict` and should use `TrustServerCertificate: false`; Oracle uses `enable` for TCPS and requires a wallet via `SSLCAPath`. MySQL requires a non-empty `SSLCAPath` only as a switch for GOREST to pass the `TLS` value to go-sql-driver/mysql; GOREST does not load that path as a CA file. MySQL TLS `true` uses the driver's system certificate roots. For a custom CA or client certificate, register a TLS profile with the MySQL driver and set `TLS` to that profile name (while keeping `SSLCAPath` non-empty); GOREST does not use `SSLCertPath` or `SSLKeyPath` for MySQL. SQLite has no network TLS. ScyllaDB's TLS is not currently configured by this database `TLS` field, so setting it does not enable ScyllaDB transport encryption.

```go
app.Use(database.Run(database.Config{
    Enabled: true,
    // Driver Selection & Target
    Driver: database.MYSQL, // Supported: MYSQL, POSTGRES, SQLSERVER, ORACLE, SQLITE, MONGO, SCYLLA
    Host:   "127.0.0.1",
    Port:   "3306",
    Name:   "default_gorest", // Database name or Oracle Service Name (e.g., "FREEPDB1")

    // Credentials (Load securely from env variables or secrets manager in production)
    Username: "root",
    Password: "your_secure_password",

    // Connection Pools & Lifetimes
    MaxOpenConnections:    50,               // Default: 50; zero uses the default
    MaxIdleConnections:    5,                // Default: 5; zero uses the default
    ConnectionMaxLifetime: 10 * time.Minute, // Default: 10 minutes; zero uses the default
    Timeout:               5 * time.Second,  // Connection and operation timeout limit

    // Security, TLS & Encryption (Driver-aware behavior)
    TLS:                    "disable", // Local/development compatibility default only; explicitly configure production TLS
    SSLCertPath:            "",        // Path to client SSL certificate file (mutual TLS)
    SSLKeyPath:              "",        // Path to client SSL private key file
    SSLCAPath:               "",        // CA path (PostgreSQL/etc.), Oracle Wallet; for MySQL, non-empty enables TLS option but path isn't loaded
    TrustServerCertificate: false,     // MSSQL only: validate the server certificate; configure its CA as needed

    // Driver-Specific Addons
    LibDir:  "",        // Oracle Instant Client library directory path (godror/OCI only)
    Charset: "utf8mb4", // Character set (MySQL default: "utf8mb4")

    // Migrations & Seeders Automation
    MigrationDirectory: "migrations", // Relative to DatabaseDirectory; driver subdirectory is appended
    SeederDirectory:    "seeders",    // Relative to DatabaseDirectory; driver subdirectory is appended
    Migration:          true,         // Auto-run migrations on startup (Dev: true, Prod: false recommended)
    Seeder:             true,         // Auto-run seeders on startup (Dev: true, Prod: false recommended)
    QueryLogging:        false,        // Enable query execution logging/debugging (Slow, dev-only)
}))
```

Database Parameter Breakdown:
- **Driver & Target** (Driver, Host, Port, Name): Specifies the database engine and connection routing. Note that for Oracle, Name acts as the service name instead of the SID.
- **Credentials** (Username, Password): Secure authentication fields. Always load production secrets via environment variables.
- **Connection Pool Management** (MaxOpenConnections, MaxIdleConnections, ConnectionMaxLifetime, Timeout): Controls database socket limits and prevents memory leaks. `database.Run(...)` fills zero-valued pool fields with defaults (50 open, 5 idle, 10 minutes maximum lifetime); negative pool values are invalid.
- **Security** & TLS (TLS, SSLCertPath, SSLKeyPath, SSLCAPath, TrustServerCertificate): Encrypts connections depending on driver specifications. Special note: For Oracle, SSLCAPath repurposes as the Oracle Wallet directory path (TNS_ADMIN) when TLS is set to "enable".
- **Migrations & Seeders** (Migration, Seeder, directories, QueryLogging): Automates database bootstrapping and gives developers direct visibility into query generation during development.