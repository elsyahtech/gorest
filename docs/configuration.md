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

The default server configuration provides initial timeout and middleware settings. Review them for your application's traffic, security policy, and deployment. You can configure the server engine, address, port, and middleware using `server.Config`:

When `Core` is omitted (`nil`), Gorest sets finite Fiber timeouts: `ReadTimeout` to 10 seconds, `WriteTimeout` to 30 seconds, and `IdleTimeout` to 120 seconds. These defaults reduce exposure to slow or idle connections that can occupy server resources. If an application uses long-running responses or streaming (for example, SSE), choose timeouts appropriate to that workload. Supplying `Core` replaces the GOREST core defaults; GOREST preserves its values, including zero timeouts. In Fiber, a zero read or write timeout is unlimited, and a zero idle timeout falls back to the read timeout. When `server.New()` receives a `Core` with zero `ReadTimeout` or `WriteTimeout`, it preserves the values and logs a warning; it does not reject startup. Configure finite positive timeouts unless the unlimited behavior is intentional and documented for the application. On `SIGINT` or `SIGTERM`, `app.Start()` gracefully shuts down Fiber before closing Redis, database, and logger resources, waiting up to 30 seconds for active requests. A timeout is reported and remaining services are then closed; `app.Close()` uses the same shutdown sequence. Unit coverage that constructs a server directly verifies `app.Close()` waits for in-flight requests and stops the listener, while a separate helper-process integration test exercises `app.Start()` plus `SIGTERM`. These tests model local behavior and cannot guarantee identical signal delivery, traffic, dependency behavior, or termination timing in every application's deployment; verify shutdown with the application's actual process supervisor/container and real handlers.

`ReadTimeout` applies to reading the full request, including its body, so it bounds how long a slow client can take to send a request. `BodyLimit` limits request size, but does not limit how slowly those bytes can arrive; it is not a substitute for a finite `ReadTimeout`. GOREST's server tests include a local TCP test that sends an incomplete request body and verifies Fiber returns `408 Request Timeout` when a finite read timeout expires. This verifies the configured server behavior in the test environment, not behavior through every reverse proxy, load balancer, network, or production deployment.

Both `server.Run(...)` and `server.New(...)` apply GOREST defaults to omitted sections. This includes all six middleware configurations: CORS, Recover, Request ID, Compress, Helmet, and Limiter. Therefore, `server.New(&server.Config{})` installs the default middleware just like `server.New(nil)`; a nil middleware field means “use the GOREST default,” not “disable it.” `server.New(...)` currently has no nil-based middleware opt-out.

These middleware provide HTTP behavior and configurable controls, not a complete application security policy. CORS controls browser cross-origin access; it does not authenticate callers or authorize API operations. Recover handles panics in the request pipeline, Request ID supports request correlation, Compress changes response encoding, Helmet adds configured response headers, and Limiter applies a request limit. Review and tune their settings for the application and deployment. Define authentication and per-resource authorization in application code or an explicitly integrated identity layer; configure TLS, trusted proxies, secrets, network access, and suitable middleware policies for the serving environment. The presence of middleware or default configuration is not proof that an endpoint or deployment is secure.

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

    // Security headers middleware (Helmet); review its policy for your application.
    Helmet: &helmet.Config{},

    // Rate limiting middleware; tune its limits and scope for your traffic and deployment.
    Limiter: &limiter.Config{
        Max:        100,
        Expiration: 1 * time.Minute,
    },
})
```

HTTP Server Parameter Breakdown:
- **Host & Port:** Defines where your application listens for incoming traffic (default is 127.0.0.1:3000).
- **Core:** Direct access to underlying fiber.Config settings for advanced performance tuning.
- **Cors:** Sets browser cross-origin sharing behavior. It is not authentication or authorization.
- **Recover:** Handles panics in the request pipeline; it does not replace application error handling or monitoring.
- **Requestid:** Adds a request identifier for correlation. It does not establish caller identity.
- **Compress:** Compresses HTTP responses to reduce payload size; it is a performance feature.
- **Helmet:** Adds configured HTTP response headers as one defense-in-depth measure. Review the policy for your application and clients.
- **Limiter:** Applies configured request limits. Tune the policy and use suitable edge or shared controls for the deployment; it is not, by itself, DDoS protection.

### 3. Redis Configuration (`redis.Run(...)`)
For caching and other Redis-backed operations, Gorest provides Redis integration with authentication, database selection, connection-pool settings, and TLS/mTLS configuration options. Confirm the effective TLS and certificate settings against the Redis server and deployment environment.

You can configure Redis by passing `redis.Config`:

```go
redis.Run(redis.Config{
    Disable: false, // false (default) initializes Redis when registered with app.Use; true skips it

    // Connection Target & Auth
    Host:     "127.0.0.1",
    Port:     "6379",
    Username: "", // Leave blank if using password-only or no auth (Redis 6.0+ ACL)
    Password: "", // Leave blank if no password is required
    Database: 0,  // Redis database index number (usually 0 to 15)

    // Security & TLS
    TLS:                     false, // Enable when required by the Redis endpoint; verify TLS in the target environment
    AllowInsecureProduction: false, // Explicit opt-in for an intentional, non-TLS production exception
    SSLCAPath:               "",    // Path to CA certificate file for TLS verification
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
- **Connection & Credentials** (Host, Port, Username, Password, Database): Configures the Redis endpoint and credentials, including ACL usernames and database indexes. Load secrets from a deployment secret store or environment configuration; do not commit them.
- **Security & TLS** (TLS, SSLCAPath, SSLCertPath, SSLKeyPath): Configures TLS and optional mutual TLS where supported by the endpoint. Verify certificate validation and the effective connection settings in the target environment.
- **Timeouts & Lifetimes** (Timeout, PoolTimeout, ConnMaxLifetime, ConnMaxIdleTime): Bounds connection and operation waits and controls connection reuse. Choose values for the service latency and workload; these settings do not replace monitoring or outage handling.
- **Pool & Buffer Optimization** (PoolSize, MinIdleConns, MaxActiveConns, MaxRetries, ReadBufferSize, WriteBufferSize): Fine-tunes concurrency throughput. Note on PoolSize: When deploying inside Docker or Kubernetes, ensure your CPU container limits are set properly so Go's runtime.GOMAXPROCS doesn't misread the host's total core count!

`redis.Config.Disable` controls whether Redis is registered and initialized by `app.Use(...)`. Its default is `false`, so Redis is enabled unless you explicitly set `Disable: true`. When enabled, GOREST verifies connectivity with `PING`; an initialization error is logged and aborts startup. When Redis is disabled or not registered, `app.Redis().QueryRedis()` returns an explicit unavailable-service error instead of a nil client. `app.Close()` closes an initialized Redis client during application shutdown.

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

In production-like environments, GOREST warns when Redis TLS is disabled and records whether the deployment explicitly opted in with `AllowInsecureProduction`. Interactive foreground startup prompts for approval before opening the unencrypted connection; the default is no. Headless deployments use the typed application or Redis config opt-in and never prompt. Enable the opt-in only for trusted local or isolated networks after documenting the risk and network controls.

For TLS implementation validation, a localhost certificate is sufficient to test certificate loading, encrypted handshakes, mutual TLS, invalid certificates, and expiry handling. A public or private DNS name is needed for production-representative hostname verification. Testing against `localhost` does not prove that a publicly deployed certificate chain and hostname are valid; use a certificate whose SAN matches the production hostname and is issued or trusted by an approved CA.

SQLite is local storage and does not use network transport. Production SQLite accepts empty, `localhost`, `127.0.0.1`, and `::1` host values only. Remote hosts are rejected rather than silently ignored. SQLite remains allowed in development/testing and for explicitly approved single-node production workloads, but it is not recommended for horizontally scaled or shared remote deployments.

TLS verification is enabled by default when TLS is configured; GOREST does not set `InsecureSkipVerify`. `SSLCAPath` adds a CA pool for server verification. Configure both `SSLCertPath` and `SSLKeyPath` together when mutual TLS is required. Connection timeout, retry, and pool options are passed to go-redis, but they do not guarantee recovery from prolonged Redis outages; validate reconnect behavior, resource limits, and application fallback policy against the Redis version and deployment used by your service.

### 4. Database Configuration (`database.Run(...)`)
Gorest provides modular database adapters supporting both SQL and NoSQL environments (**PostgreSQL, MySQL, SQL Server, Oracle, SQLite, MongoDB, and ScyllaDB**). 

`database.ConfigDefault` selects SQLite as the default lightweight database driver. `app.New()` does not automatically register or open a database; register one with `app.Use(...)` as shown below. To use another engine, change the driver and its connection settings:

`database.Run(config)` applies database defaults and returns a normalized `database.Config`; it does not establish a connection. Pass its result to `app.Use(...)` to register and initialize the database. `app.Use(...)` does not call `database.Run(...)` automatically. The `Disable` field defaults to `false`, so the database is enabled unless you explicitly set `Disable: true`. A partial config literal also has `Disable: false` by Go's zero-value rule; use `database.ConfigDefault` when you want the other configured defaults as well.

GOREST accepts custom `Environment` labels such as `staging`, `uat`, or `preview`; `development`, `testing`, and `production` are conventions, not an allowlist. The TLS warning policy treats every non-empty environment other than `development` and `testing` as production-like, so custom deployment environments do not silently bypass the production database TLS warning.

For compatibility and ease of local development, an omitted `TLS` value defaults to disabled for network database drivers (MongoDB defaults to `false`; SQLite does not use TLS). Gorest does not automatically enable TLS based on the database host: TLS modes and certificate or wallet requirements differ by driver, and automatically enabling one could break existing local or legacy deployments. This is not a secure production default. A remote database connection without TLS can expose credentials and data to interception or modification on an untrusted network. Configure a driver-supported TLS mode explicitly for production and validate the server certificate; do not treat a private IP, VPN, or firewall as a replacement for transport encryption.

For an enabled database, `app.Start()` logs a driver-aware warning when the environment is production-like (anything except `development` or `testing`) and TLS is disabled, optional, or lacks required certificate configuration. ScyllaDB is evaluated using the same production policy when `TLS` is not `"enable"`. In production-like environments this policy is enforced at startup: without explicit approval, TLS-disabled database and Redis connections cannot proceed. Set component-level `AllowInsecureProduction: true` or application-level `gorest.Config.AllowInsecureProduction: true` only as an explicit, auditable opt-in for a trusted local or isolated network without transport TLS. For an interactive foreground process, GOREST prompts the operator before opening an unencrypted connection; the default is no, and `y`/`yes` records approval for that process. Headless deployments never wait for terminal input; they must enable TLS or use the application-level or component-level typed opt-in. Custom labels such as `staging` and `uat` receive the same production-like startup policy.

Driver-specific notes: PostgreSQL supports `require`, `verify-ca`, and `verify-full` (prefer certificate and hostname validation with `verify-full`); SQL Server supports `mandatory` or `strict` and should use `TrustServerCertificate: false`; Oracle uses `enable` for TCPS and requires a wallet via `SSLCAPath`. For MySQL, GOREST requires a non-empty `SSLCAPath` only as a switch for passing `TLS` to go-sql-driver/mysql; it does not load that path as a CA file. With go-sql-driver/mysql v1.10.1, `TLS: "true"` enables normal certificate verification using the system roots and verifies the hostname derived from the connection address. Do not use the driver's `skip-verify` or `preferred` modes in production because they disable certificate verification. For a custom CA or client certificate, register a TLS profile with `mysql.RegisterTLSConfig`, configure its `RootCAs`, `ServerName`, and optional client certificate, then set `TLS` to that profile name and keep `SSLCAPath` non-empty. GOREST does not pass `SSLCertPath` or `SSLKeyPath` to the MySQL driver. SQLite has no network TLS.

### Operational endpoints and environment configuration

GOREST registers built-in operational endpoints in the `/gorest` route group by
default:

- `GET /gorest/live` — liveness probe; returns `200` with `{"status":"alive"}`
  when the application can serve the request.
- `GET /gorest/ready` — readiness probe; reports the status of the logger,
  server, database, Redis, and JWT filter. Each component is reported as
  `healthy`, `unhealthy`, or `not running`. The overall `status` is `ready`
  (`200`) or `not_ready` (`503`).
- `GET /gorest/metrics` — runtime metrics including goroutine count, heap
  allocation, heap system memory, and GC cycles. These are JSON runtime
  statistics, not a Prometheus exposition endpoint or a replacement for
  Prometheus/OpenTelemetry instrumentation.

Control endpoint exposure through `gorest.Config.DisableMonitoringEndpoints`.
It defaults to `false` (enabled); set it to `true` to prevent all three routes
from being registered:

```go
app := gorest.New(gorest.Config{
    DisableMonitoringEndpoints: true,
})
```

Disabling these routes does not disable the HTTP server, application services,
or their internal instrumentation. When enabled, the endpoints are served by
the application's HTTP server and have no built-in authentication specific to
the monitoring group. If their information should remain private, disable them
or restrict access at the network, proxy, or application layer.

The readiness endpoint reports all component statuses in its JSON response.
An unconfigured optional Redis or JWT filter is reported as `not running`;
an unavailable configured dependency or failed health check marks readiness
as `not_ready`. Liveness only confirms that the application can answer that
request; use an external probe to detect a process that cannot serve HTTP.

Database secrets can be loaded without placing them in source code using `database.ConfigFromEnv("GOREST_DB")`. Supported variables are `GOREST_DB_DRIVER`, `HOST`, `PORT`, `NAME`, `USERNAME`, `PASSWORD`, `TLS`, `SSL_CA`, `SSL_CERT`, and `SSL_KEY`. The password remains in the child-process environment only where required and is never logged by Gorest.

```go
cfg := database.ConfigFromEnv("GOREST_DB")
db, message, err := app.Database() // use only after app.Use(database.Run(cfg))
```
 ScyllaDB TLS is enabled by setting `TLS: "enable"`. GOREST loads `SSLCAPath` as a custom root pool and enables server certificate plus hostname verification. `SSLCertPath` and `SSLKeyPath` must be configured together for optional mTLS.

```go
app.Use(database.Run(database.Config{
    Disable: false, // false (default) initializes the database; true skips it
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
    TLS:                     "disable", // Local/development compatibility default only; explicitly configure production TLS
    AllowInsecureProduction: false,      // Explicit opt-in for an intentional, documented production exception
    SSLCertPath:             "",         // Path to client SSL certificate file (mutual TLS)
    SSLKeyPath:              "",        // Path to client SSL private key file
    SSLCAPath:               "",        // CA path (PostgreSQL/etc.), Oracle Wallet; for MySQL, non-empty enables TLS option but path isn't loaded
    TrustServerCertificate: false,     // MSSQL only: validate the server certificate; configure its CA as needed

    // Driver-Specific Addons
    LibDir:  "",        // Oracle Instant Client library directory path (godror/OCI only)
    Charset: "utf8mb4", // Character set (MySQL default: "utf8mb4")

    // Migrations & Seeders Automation
    MigrationDirectory: "migrations", // Relative to DatabaseDirectory; driver subdirectory is appended
    SeederDirectory:    "seeders",    // Relative to DatabaseDirectory; driver subdirectory is appended
    Migration:          true,         // Auto-run migrations on startup (Prod: run once in a deployment job)
    Seeder:             true,         // Auto-run seeders on startup (Prod: run once in a deployment job)
    QueryLogging:        false,        // Enable query execution logging/debugging (Slow, dev-only)
}))
```

Database Parameter Breakdown:
- **Driver & Target** (Driver, Host, Port, Name): Specifies the database engine and connection routing. Note that for Oracle, Name acts as the service name instead of the SID.
- **Credentials** (Username, Password): Supplies driver connection credentials. Load production secrets from a deployment secret store or environment configuration; do not commit them.
- **MongoDB authentication database** (`AuthSource`): Selects the database that stores the MongoDB user's credentials. It defaults to `admin`; set it when the user was created in another database.
- **Connection Pool Management** (MaxOpenConnections, MaxIdleConnections, ConnectionMaxLifetime, Timeout): Controls connection pool limits and lifetimes. `database.Run(...)` fills zero-valued pool fields with defaults (50 open, 5 idle, 10 minutes maximum lifetime); negative pool values are invalid. Tune these values for the database limits and application concurrency.
- **TLS configuration** (TLS, SSLCertPath, SSLKeyPath, SSLCAPath, TrustServerCertificate): Requests driver-specific encrypted connections and certificate behavior. Support and semantics vary by driver; verify the negotiated connection and certificate validation in the target environment. MongoDB uses `SSLCertPath` and `SSLKeyPath` together for a client certificate and private key when mTLS is required. For Oracle, `SSLCAPath` is used as the Oracle Wallet directory path (`TNS_ADMIN`) when TLS is set to `"enable"`.
- **Migrations & Seeders** (Migration, Seeder, directories, QueryLogging): Automates database bootstrapping and gives developers direct visibility into query generation during development.

### JWT Filter Configuration (`filter.Run(...)`)

JWT authentication is optional and disabled unless a filter configuration
with a non-empty signing key is registered. Start from `filter.ConfigDefault`
so the configured defaults are preserved, then register the configuration
with `app.Use(...)` before using authentication middleware:

```go
import (
    "os"
    "time"

    "github.com/elsyahtech/gorest"
    "github.com/elsyahtech/gorest/filter"
)

signingKey := os.Getenv("GOREST_JWT_SIGNATURE_KEY")

filterConfig := filter.ConfigDefault
filterConfig.SignatureKey = &signingKey
filterConfig.AccessTokenDuration = 15 * time.Minute
filterConfig.RefreshTokenDuration = 30 * 24 * time.Hour
filterConfig.CookieSecure = true

app := gorest.New(gorest.Config{AppName: "my-service"})
app.Use(filter.Run(filterConfig))
```

Provide `GOREST_JWT_SIGNATURE_KEY` through the deployment environment or a
secret manager. Use a high-entropy secret, do not commit it to source control,
and plan key rotation. Registering a filter config without a signature key
aborts startup. An unsupported signing method or token duration under one
second is logged and leaves the filter uninitialized; if route setup then
calls `app.Filter()` or creates authentication middleware from that
uninitialized filter, startup aborts. If JWT is not needed, do not register a
filter config.

| `filter.Config` field | Default and behavior |
|---|---|
| `SignatureKey` | No key by default; required to enable the filter. |
| `SigningMethod` | `HS256`; unsupported algorithms prevent filter initialization. |
| `Issuer` | Defaults to `gorest`, but GOREST sets the effective JWT issuer to the application's `gorest.Config.AppName`. |
| `AccessTokenDuration` | 15 minutes; minimum one second. |
| `RefreshTokenDuration` | 30 days; minimum one second. |
| `CookieSameSite` | `Lax`. |
| `CookieDomain` | Empty, producing host-only cookies. Set only when sharing cookies across subdomains is intended. |
| `CookiePath` | `/`. |
| `CookieSecure` | `false`; set to `true` when cookies are served over HTTPS. |
| `CookieHTTPOnly` | `true` with `filter.ConfigDefault`; prevents browser JavaScript from reading token cookies. |
| `CookieSessionOnly` | `false`; cookies expire based on their token duration. When true, browser-session cookies are used, but the JWT's own expiry still applies. |
| `CookieMaxAge` | Deprecated and ignored for token-cookie lifetime; expiry is derived from each token duration. |

Use `filter.ConfigDefault` when customizing the filter. A partial
`filter.Config{...}` literal gets zero values for boolean fields, so
`CookieHTTPOnly` and `CookieSecure` will both be false unless set explicitly.
With the defaults, GOREST issues cookies named `access` and `refresh`.
Authentication middleware accepts an access JWT from the `Authorization`
header using the `Bearer` scheme, or from the `access` cookie; if access
validation fails, it tries the `refresh` cookie. A valid refresh cookie can
renew access and continue the current protected request. The middleware
verifies the token signature, signing method, issuer, type, required claims,
and expiry. It then places a `filter.TokenDetail` in request locals under
`tokenJWT`. It does not authenticate user credentials, check account state, or
authorize application resources. For middleware, token creation, claims,
refresh behavior, and cookie security, see the
[JWT filter guide](./guide/filter.md).
