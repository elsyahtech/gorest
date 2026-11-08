# GOREST Runtime Hardening Report

This guide explains GOREST's optional runtime configuration report, how to
enable and access it, what its results mean, and which deployment controls
still need to be reviewed by the application team.

## 1. Purpose

The hardening report helps an application team review the effective GOREST
configuration held by a running process. It inventories exported framework
configuration fields, applies a conservative subset of documented checks,
explains unknowns, and returns JSON or HTML.

The report is an advisory configuration assessment, not a runtime monitor,
penetration test, compliance certification, or guarantee that an application
is production-ready. Green means only that a specific check passed for the
evidence examined.

The report endpoint is separate from `monitoring`. It does not collect
performance metrics, perform source-code analysis, or certify that the
application is production-ready. It does not connect to databases or Redis,
modify files, or change the request behavior of application routes.

## 2. Endpoint and access control

Enable the endpoint explicitly and provide a narrow peer-IP allowlist:

```go
app.Use(hardening.Config{
    Enable:              true,
    HardeningAllowedIPs: []string{"127.0.0.1/32", "::1/128"},
})
```

The route is `GET /gorest/hardening`. JSON is returned by default; request
`/gorest/hardening?format=html` or send `Accept: text/html` for HTML. Unsupported
formats receive HTTP 400. Reports are generated for each request and are not
written to disk. Responses use `Cache-Control: no-store`.

The endpoint is disabled by default (`Enable: false`). When enabled, an empty
allowlist denies every peer; requests outside the configured IP/CIDR ranges
receive HTTP 403. The check uses the application's connection peer IP and does
not trust `X-Forwarded-For`. Behind a reverse proxy, allowlist the proxy peer
addresses and restrict direct access to the origin. Do not use a public or
overly broad range unless intentional.

The returned inventory contains operational metadata such as configured
hosts, paths, usernames, and middleware settings. Keep the endpoint private
and treat reports as sensitive. Passwords, signing keys, private keys, and
token values are represented only as configured/not-configured metadata.

## 3. Evidence and limits

The endpoint assesses the effective GOREST configuration available in the
running process. It does not reread startup input from source or environment,
and does not prove that the current configuration matches a separately
deployed binary or infrastructure declaration. Function callbacks and opaque
third-party settings may be inventoried as unknown. Report values describe
the current normalized configuration in this process, not necessarily every
value supplied in the application's original configuration literal.

The endpoint inventories available exported GOREST configuration fields and
supported nested framework configuration values. An unconfigured optional
component appears as not configured/unknown. It applies the runtime checks
listed in the current coverage summary below. Source analysis, source secret
scanning, all checks in the reference matrix, and external infrastructure
assessment are outside its scope. Unimplemented or unobservable controls
must not be interpreted as passing.

## 4. Configuration inventory

### Application configuration

| Configuration | Fields included in the report | Main assessment areas |
|---|---|---|
| `gorest.Config` | `AppName`, `Version`, `Environment`, `Timezone`, `Debug`, `AllowInsecureProduction`, `CheckForUpdates` | Production-like environment recognition; debug disabled; insecure-production exception explicitly declared; update-check policy documented. Application name, version and timezone are informational/operational, not inherently security pass/fail fields. |

### HTTP server configuration

| Configuration | Fields included in the report | Main assessment areas |
|---|---|---|
| `server.Config` | `Host`, `Port`, `Core`, `Cors`, `Recover`, `Requestid`, `Compress`, `Helmet`, `Limiter`, `Listener` | Listener exposure and TLS termination assumptions; finite request timeouts and body limits; CORS policy; security headers; limiter configuration; panic recovery; route-printing/debug behavior. |
| `server.ListenerConfig` | `Network`, `TLSCertKeyFile`, `TLSCertClientFile`, `TLSCertFile`, `UnixSocketPath`, `PreforkRecoverInterval`, `ShutdownTimeout`, `PreforkShutdownGracePeriod`, `PreforkRecoverThreshold`, `UnixSocketFileMode`, `TLSMinVersion`, `EnablePrefork`, `EnablePrintRoutes` | HTTPS certificate/key pairing and file readability; TLS minimum version; public bind exposure; Unix socket permissions; graceful shutdown/recovery bounds; route disclosure via startup output. |
| `server.Config.Core` (Fiber) | At minimum: read/write/idle timeouts, body limit, proxy trust and forwarded-header-related settings, error handling, case/strict routing, and any fields that affect request parsing or security | Flag unbounded or weak request limits where verifiable; correctly interpret proxy-related settings; mark unreviewed Fiber fields as unknown rather than silently ignoring them. |
| `Cors` | Allowed origins, methods, headers, exposed headers, credentials, max age, and origin callback/function configuration where present | Wildcard or overly broad origins, credentials combined with wildcard, unsafe callback values, and permissive methods/headers. Function-valued settings are normally unknown to static analysis. |
| `Helmet` | All configured security-header policy fields supported by the installed Fiber version | Missing or weakened headers relative to an explicit profile. Avoid treating one universal CSP/HSTS value as correct for every application. |
| `Limiter` | Max requests, expiration/window, key generation, storage, skip rules, headers, and related middleware options | Disabled or ineffective limiting, in-memory-only deployment assumptions, and custom callbacks that cannot be evaluated statically. Thresholds must be workload-specific. |
| `Recover`, `Requestid`, `Compress` | Their exported settings and callbacks where present | Recovery enabled and panic response behavior; request-ID header behavior; compression settings and known risks such as compression of secrets alongside attacker-controlled content. Do not mark these as universally safe solely because middleware is present. |

### Database configuration

| Configuration | Fields included in the report | Main assessment areas |
|---|---|---|
| `database.Config` | `Driver`, `Host`, `Username`, `Password`, `AuthSource`, `Name`, `Port`, `LibDir`, `TLS`, `SSLCertPath`, `SSLKeyPath`, `SSLCAPath`, `Charset`, `DatabaseDirectory`, `MigrationDirectory`, `SeederDirectory`, `Timeout`, `ConnectionMaxLifetime`, `AllowInsecureProduction`, `TrustServerCertificate`, `QueryLogging`, `Migration`, `Seeder`, `Disable`, `MaxOpenConnections`, `MaxIdleConnections` | Driver and environment consistency; TLS mode and server verification implications; credential presence without revealing values; certificate/key pair and file metadata where allowed; timeouts/pool bounds; production migration/seeder/query-log policy; SQLite topology suitability; least-privilege reminder. |
| Driver-specific database settings | Driver-specific meanings and applicability of TLS, certificates/wallet, host, port, `LibDir`, `Charset`, and pool settings | Apply rules by driver. For example, Oracle wallet requirements differ from PostgreSQL TLS modes; SQL Server certificate trust must not be conflated with TLS enablement; SQLite has no network TLS. Irrelevant fields should be `not_applicable`, not red. |

### Redis configuration

| Configuration | Fields included in the report | Main assessment areas |
|---|---|---|
| `redis.Config` | `Host`, `Port`, `Username`, `Password`, `SSLCAPath`, `SSLCertPath`, `SSLKeyPath`, `Timeout`, `PoolTimeout`, `ConnMaxLifetime`, `ConnMaxIdleTime`, `TLS`, `AllowInsecureProduction`, `Disable`, `Database`, `MaxRetries`, `PoolSize`, `MinIdleConns`, `MaxActiveConns`, `ReadBufferSize`, `WriteBufferSize` | TLS and certificate validation, authentication configuration without exposing values, explicit insecure-production approval, bounded connection/operation waits, pool sizing and retry tradeoffs, and whether Redis is enabled/required. No universal pool-size recommendation should be labeled as a security failure. |

### Filter/JWT configuration

| Configuration | Fields included in the report | Main assessment areas |
|---|---|---|
| `filter.Config` | `LoggerConfig`, redacted `SignatureKey`, `SigningMethod`, `Issuer`, `CookieConfig` fields, `AllowedIPs`, `BlockedIPs`, `AccessTokenDuration`, `RefreshTokenDuration` | Signing-key presence without revealing its value; algorithm; secure/HttpOnly cookie settings; IP restrictions. Token lifetime policy and authentication/authorization coverage require application-specific review. |
| `filter.LoggerConfig` | `DisableLogging`, `EnabledEvents` | Whether useful authentication events are logged and whether the selected event set could expose sensitive data or create excess volume. Never assume that logging alone establishes audit coverage. |

### Logging and monitoring configuration

| Configuration | Fields included in the report | Main assessment areas |
|---|---|---|
| `log.Config` | `LogDir`, `LogFile`, `Caller`, `MaxFileSize`, `MaxBackups`, `MaxAge` | Retention and rotation bounds; path/permission checks only when an explicit local filesystem path is available and the user enables filesystem checks; avoid declaring local log files suitable for every container/runtime. |
| `monitoring.Config` | `MonitoringAllowedIPs`, `DisableMonitoringEndpoints`, `EnableHTTPMetricsMonitoring` | Whether operational endpoints are disabled or access-restricted; invalid CIDR/IP values; route-level metrics cardinality and memory tradeoff. The hardening endpoint uses its own `HardeningAllowedIPs`; it does not protect legacy diagnostic endpoints. |
| `hardening.Config` | `Enable`, `HardeningAllowedIPs` | Whether the report endpoint is registered and whether its peer-IP allowlist is configured. The endpoint remains disabled by default. |

### Other and evolving fields

| Field class | Report behavior |
|---|---|
| Third-party nested config fields | Exported values are inventoried. Function callbacks and opaque values are represented as configured/opaque or unknown because their behavior cannot be safely evaluated. |
| New GOREST config fields | Exported configuration fields are discovered at runtime. New fields appear in the inventory but do not automatically receive an assessment rule. |
| Fields without security/performance significance | They appear in the inventory, but do not automatically receive a pass/fail finding. |

## 5. Security and operational review matrix

Severity describes the potential consequence and confidence of an individual
rule, not the security of the whole application. Severity and status are
separate: a high-severity control can remain unknown when the report lacks
deployment evidence. The matrix below is a client review guide; it is broader
than the checks currently automated by the endpoint.

| ID | Area | Review question | Evidence to consider | Typical interpretation | Severity if confirmed |
|---|---|---|---|---|---|
| APP-001 | Environment | Production-like target explicitly selected for the report; custom labels follow the same policy | Snapshot or source literal | Unknown if dynamic; informational if explicitly set | Informational |
| APP-002 | Debug | Debug mode is enabled in a production-like profile | Effective config or recognizable literal | Red when true; unknown when dynamic | High |
| APP-003 | Insecure exceptions | Global or component `AllowInsecureProduction` is enabled | Effective config/source literal | Yellow with the affected component and exception; never interpret as TLS enabled | High |
| HTTP-001 | Listener exposure | Listener binds to wildcard/public interface or a non-loopback host | Effective host/network/listener config | Yellow or red according to profile and explicit exposure intent; unknown behind a proxy | Medium/High |
| HTTP-002 | TLS | TLS is terminated in GOREST or by a declared trusted proxy; listener cert/key pairing is coherent | Listener config and explicit deployment metadata | Red for known plaintext public listener without declared TLS termination; unknown if topology is not supplied | High |
| HTTP-003 | TLS protocol | Minimum TLS version below profile baseline | `TLSMinVersion` | Red if below configured baseline; not applicable if TLS is terminated elsewhere and not controlled here | High |
| HTTP-004 | Request limits | Read/write/idle timeout or body limit is unlimited/too permissive | Core config | Red/yellow only against configurable baseline; custom Core must be inspected as supplied | Medium/High |
| HTTP-005 | CORS | Wildcard origins, permissive origin callback, or credentials combined with broad origin policy | Cors config | Red/yellow for recognizable unsafe combination; callback usually unknown | High |
| HTTP-006 | Security headers | Relevant headers disabled or materially weakened | Helmet config | Yellow by default; severity depends on application/browser use and deployment | Medium |
| HTTP-007 | Rate limiting | Limiter disabled, bypassed, or only local memory in a horizontally scaled deployment | Limiter config and deployment profile | Yellow/unknown unless scale and endpoint sensitivity are declared | Medium |
| HTTP-008 | Route disclosure | Route printing enabled in production-like profile | Listener config | Yellow or red per policy | Low/Medium |
| DB-001 | Database TLS | Network DB uses disabled/optional TLS without explicit approved exception | Driver, TLS, environment, approval fields | Red for known unencrypted production-like connection; unknown if configuration dynamic | High |
| DB-002 | Certificate verification | Configuration selects trust/skip verification behavior or lacks required CA/wallet inputs | Driver-specific TLS settings | Red when verification is explicitly bypassed; unknown when driver profile cannot be inspected | High |
| DB-003 | Client certificate | mTLS key/certificate pair is incomplete or unreadable when required | Certificate paths and optional filesystem metadata | Red for mismatched pair; unknown for external secret mounts not available to scanner | High |
| DB-004 | Credentials | Empty credentials, obvious defaults, or secrets embedded in source/repository | Source scan and secret-presence metadata | Red for known committed secret/default credential; unknown if supplied externally | Critical/High |
| DB-005 | Driver suitability | SQLite used for declared multi-replica/shared production workload | Driver and deployment profile | Red/yellow only when scale/topology is explicitly known | High |
| DB-006 | Pool/timeouts | Pool unbounded/ill-suited, invalid bounds, or missing per-operation context policy | Effective config and profile | Yellow; workload and driver limits prevent universal pass/fail | Medium |
| DB-007 | Schema changes | Migration/seeder/query logging enabled in production-like profile | Effective config | Yellow/high depending on policy; distinguish migration jobs from app replicas | Medium/High |
| DB-008 | Database privileges | Least-privilege role is declared/verified | Optional operator attestation or external evidence | Unknown by default; config alone cannot inspect database grants | High |
| REDIS-001 | Redis TLS | Redis network connection lacks TLS in production-like profile without explicit exception | Redis config/environment | Red for known unencrypted connection; unknown for external termination/topology | High |
| REDIS-002 | Redis credentials | Credentials embedded in source or absent where policy requires auth | Static scan/config metadata | Red for known secret exposure; unknown for ACL policy | High |
| REDIS-003 | Redis timeout/pool | Operation/pool waits or resource bounds are missing or inconsistent with the declared profile | Effective config | Yellow; tune to workload and service limits | Medium |
| JWT-001 | Signing key | Missing, too short/known/default, or committed signing key | Filter config and source scan | Red for known missing/weak/committed key; never include key in report | Critical/High |
| JWT-002 | Algorithm | Signing method is outside an explicit approved algorithm policy | Filter config | Red for disallowed algorithm; no generic recommendation without library support policy | High |
| JWT-003 | Token duration | Access/refresh lifetime exceeds an explicit policy | Filter config/profile | Yellow/red only against declared policy | Medium/High |
| JWT-004 | Cookies | Secure/HttpOnly/SameSite settings conflict with a declared browser/HTTPS deployment | Filter config and profile | Red/yellow based on explicit browser-cookie use; otherwise not applicable/unknown | High |
| LOG-001 | Logging paths | Log path or permissions appear unsafe or logs may be retained indefinitely | Log config; optional filesystem metadata | Yellow/unknown without deployment filesystem context | Medium |
| LOG-002 | Sensitive logging | Known config enables query/auth logging that may contain sensitive values | Config and known framework behavior | Yellow/high by policy; scanner must not infer that heuristic redaction is complete | Medium/High |
| MON-001 | Operational endpoints | Monitoring/diagnostic endpoints enabled without an access policy | Monitoring config and route inventory | Red/yellow depending on endpoint and whether peer allowlist is configured | High |
| MON-002 | Allowlist | Invalid, empty, overly broad, or proxy-mismatched IP/CIDR rules | Monitoring config and declared proxy topology | Red for invalid/known unrestricted exposure; unknown when proxy peer address is not declared | High |
| PERF-001 | Resource bounds | Connection pools, request body limits, timeouts, or metrics cardinality lack a declared bound | Config and optional deployment profile | Yellow/unknown, not a generic production failure | Medium |
| CODE-001 | Native query escape hatches | Usage of raw/native SQL, CQL, MongoDB, or dynamic identifiers exists | Static AST/pattern scan | Yellow review item; not a vulnerability finding without evidence of unsafe input construction | Informational/Medium |
| CODE-002 | Secret scanning | Candidate credentials/keys appear in tracked source or config files | Local repository scan | Red for high-confidence secret matches; redact matched value and restrict report access | Critical/High |
| SCOPE-001 | Application controls | Authentication, authorization, input/business validation, CSRF, backup, network policy, and cloud IAM | Usually not available to framework scanner | Unknown/out of scope with owner/action note | Not scored as pass |

Treat this matrix as a review checklist, not a result generated in full by the
endpoint. A configuration difference is not automatically a vulnerability;
consider application behavior, deployment topology, and compensating controls.

The endpoint currently implements an initial subset: production debug and
insecure-transport flags; listener host, TLS certificate/key pairing and TLS
minimum version; positive HTTP read/write timeouts; database TLS exceptions,
production migrations/seeders/query logging and SQL Server certificate trust;
Redis TLS; browser-cookie `Secure`/`HttpOnly`; and monitoring endpoint exposure.
Other matrix items are not assessed by the endpoint and must not be inferred
to pass.

## 6. Status, severity, and confidence

Each check has a status and, when applicable, a severity. The report's
severity totals count only findings with `fail` or `review` status; `pass`,
`unknown`, and `not_applicable` are counted separately by status.

| Status | Indicator | Meaning |
|---|---|---|
| `fail` / action required | 🔴 | The report found evidence that a defined check is not met. This is not automatically proof of exploitability. |
| Review / warning | 🟡 | A configuration choice or risk needs owner review, contextual tuning, or compensating control evidence. |
| `pass` for this check | 🟢 | The specific check passed using the evidence recorded in the report. It is not a general security approval. |
| `unknown` / not assessed | ⚪ | Values are dynamic, evidence is missing, the field is opaque, or the control is outside endpoint coverage. |
| Not applicable | — | The check does not apply to the selected driver, feature, or declared architecture. It should not count as a pass. |

Severity levels are Critical, High, Medium, Low, and Informational.
Confidence is reported separately as high, medium, or low. Summary severity
counts include only findings with `fail` or `review` status; `pass`, `unknown`,
and `not_applicable` are counted separately by status. The endpoint does not
enforce CI thresholds.

## 7. Request and response formats

`GET /gorest/hardening` accepts `format=json` or `format=html` through the
`format` query parameter. JSON is the default. HTML can also be selected with
the `Accept: text/html` request header. Unsupported format values return
HTTP 400.

## 8. Output contract

Both response formats contain:

- Report name/version, schema version, timestamp, environment profile, and
  evidence mode.
- Per-field inventory and per-check results. Findings include `field_path`,
  `status`, `severity`, `confidence`, `evidence_source`, redacted
  `observed_value`, rationale, and recommendation.
- Summary counts by status and severity, plus unknown/not-applicable counts.
- A clear scope statement and limitations.

JSON is the canonical response format. HTML is a review view and escapes
configuration-derived content. Neither format includes password, signing-key,
private-key, or token values. Reports still contain sensitive metadata, such
as endpoints, paths, and usernames, and should be access-controlled.

Example response:

```json
{
  "schema_version": "1.0",
  "scanner_version": "1.0.0",
  "profile": "production",
  "evidence_mode": "effective_runtime_config",
  "summary": {
    "critical": 0,
    "high": 1,
    "medium": 2,
    "low": 0,
    "informational": 0,
    "pass": 8,
    "review": 1,
    "fail": 1,
    "unknown": 5,
    "not_applicable": 3
  },
  "findings": [
    {
      "id": "DB-001",
      "status": "fail",
      "severity": "high",
      "confidence": "medium",
      "field_path": "database.Config.TLS",
      "evidence_source": "effective_runtime_config",
      "observed_value": "disable",
      "message": "Network database TLS is disabled for the production profile.",
      "recommendation": "Configure a verified driver-supported TLS mode."
    }
  ]
}
```

## 9. HTTP status and CI use

The endpoint returns HTTP 200 when a report is generated, HTTP 400 for an
unsupported format, and HTTP 403 when the peer is not allowlisted. It does not
write report files, enforce a failure threshold, or provide process exit codes.
Clients that use the endpoint in CI must implement report retrieval, storage,
and policy evaluation themselves. Do not treat HTTP 200 or an all-green subset
of checks as production certification.

## 10. Explicit non-goals and limits

The endpoint does not:

- Execute source analysis, start another application process, run migrations,
  or connect to databases/Redis.
- Scan infrastructure such as cloud IAM, firewall rules, Kubernetes policies,
  reverse proxies, database grants, backups, DNS, or secret-manager access
  policy.
- Claim that application authentication, authorization, business rules,
  parameterized-query usage, or endpoint-level access control is correct from
  configuration alone.
- Call a green report “secure,” “compliant,” or “ready to deploy.”
- Replace Go vulnerability/dependency checks, SAST, secret-management
  controls, penetration testing, integration testing, or operational review.
- Send source, configuration, or findings to an external service.

It is not a static scanner, secret scanner, vulnerability scanner, or
compliance certification. A green check applies only to its stated
configuration evidence and rule.

## 11. Recommended client workflow

1. Restrict access to the report endpoint and retrieve the report from an
   approved operator network or deployment environment.
2. Review every `fail` and `review` finding, then validate the recommendation
   against the application's architecture and policy.
3. Review `unknown` items separately; absence of evidence is not a pass.
4. Compare the inventory with the actual deployment configuration and verify
   external controls such as TLS termination, firewalls, proxy trust, database
   grants, secret-manager policy, and backups.
5. Store reports as sensitive deployment artifacts, and apply an application-
   owned CI policy if reports are consumed automatically.

## 12. Related guidance

- [Configuration guide](../docs/configuration.md)
- [Production validation guide](../docs/production-validation.md)
- [OWASP ASVS mapping](../docs/security/asvs-matrix.md)
- [Database querying guide](../docs/guide/querying.md)
- [Operational endpoints](../docs/configuration.md#operational-endpoints-and-environment-configuration)
