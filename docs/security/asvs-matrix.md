# OWASP ASVS Mapping for GOREST

This document maps selected OWASP Application Security Verification Standard
(ASVS) controls to GOREST's framework features and documents which controls
remain the responsibility of an application or its deployment.

**Reference standard:** OWASP ASVS 5.0.0. Control identifiers and summaries
below refer to the official
[ASVS 5.0.0 release](https://github.com/OWASP/ASVS/tree/2b47a9dd16760ee484a6672d197f1f7e25d9c632/5.0)
and its
[English requirements CSV](https://github.com/OWASP/ASVS/blob/2b47a9dd16760ee484a6672d197f1f7e25d9c632/5.0/docs_en/OWASP_Application_Security_Verification_Standard_5.0.0_en.csv).
The ASVS remains the normative source for each requirement.

## Scope and interpretation

GOREST is a Go application framework, not a complete application or deployment.
This mapping focuses on the framework's HTTP server, ORM and native database
APIs, Redis integration, configuration, helper functions, and logger. It does
not assess application handlers, identity providers, database/server policies,
reverse proxies, cloud infrastructure, or production operations.

Level 2 is a reasonable proposed target for production applications built with
GOREST, but this document is a **scoped starting matrix**, not a complete
requirement-by-requirement assessment. It does not claim that GOREST or any
application using it is ASVS-compliant, certified, or secure. A team adopting
ASVS must assess all applicable requirements against its own application and
deployment.

### Status meanings

- **Partial:** GOREST provides a related control, but the implementation,
  configuration, integration evidence, or application behavior is not enough
  to verify the full requirement.
- **Application/deployment:** The control must be implemented or configured by
  the application team or its operators; GOREST cannot establish it on their
  behalf.
- **Not provided:** GOREST currently has no built-in feature for this control.
- **Out of scope:** GOREST does not provide the feature area; reassess if an
  application adds it.

## Control matrix

| ASVS controls | Area | GOREST mapping and evidence | Initial status | Follow-up verification / owner |
|---|---|---|---|---|
| V1.2.1–V1.2.3 | Contextual output encoding | GOREST supports HTTP and JSON responses, but applications define handler output, templates, URLs, and any embedded content. | Application/deployment | Test every output context, including HTML, JavaScript, URL, and response headers. Application owner. |
| V1.2.4 | SQL and NoSQL injection prevention | ORM and native database APIs support bound values. Native SQL, raw-query callbacks, dynamic identifiers, and application-built query text are not sanitized by GOREST; the querying guide warns callers to bind values and allowlist identifiers. | Partial | Add injection-focused tests for supported drivers and review application query construction, especially raw query escape hatches. |
| V2.2.1–V2.2.2 | Input validation | ORM/query validation protects query construction in supported cases; GOREST does not provide universal request DTO, business-rule, or handler validation. | Partial | Validate untrusted request data at a trusted application layer and test invalid, oversized, and boundary values. |
| V2.3.1, V2.4.1 | Business-flow sequencing and anti-automation | GOREST provides configurable request limiting and database transaction APIs. It cannot know application workflow state, user quotas, or whether a request is authorized. | Application/deployment | Test sequencing, abuse limits, credential-stuffing controls, and shared/distributed rate limiting at the application and edge. |
| V3.4.1–V3.4.6 | Browser security headers | GOREST installs configurable Helmet middleware by default; server tests assert `X-Content-Type-Options: nosniff`. The tests do not establish a production HSTS, CSP, framing, or referrer policy. | Partial | Verify actual response headers for all routes in the deployed HTTPS topology; set CSP/HSTS and other policies to match the application. |
| V3.5.1–V3.5.3 | Cross-origin requests and safe methods | GOREST installs CORS middleware, but CORS is not authentication, authorization, or a general CSRF defense. Route methods are chosen by application code. | Application/deployment | Configure a fixed trusted-origin allowlist, test sensitive routes against cross-origin requests, and ensure state changes do not use safe methods. |
| V4.1.1, V4.2.1 | API response type and HTTP message boundaries | The Fiber server provides HTTP parsing and response handling. GOREST sets a default 4 MiB body limit and finite read/write/idle timeouts when the core config is omitted; callers can replace those defaults. | Partial | Verify content types and request parsing limits on real endpoints and through proxies. Document any intentional overrides. |
| V5.1–V5.4 | File upload, storage, and download | GOREST does not provide a file upload or download subsystem. | Out of scope | If the application accepts, processes, stores, or serves files, assess all applicable V5 requirements in the application. |
| V6.1–V6.8 | Authentication | GOREST provides access/refresh JWT issuance and optional token-authentication middleware, but does not authenticate user credentials, integrate an identity provider, or check account state. Password-hashing helpers do not constitute a credential-authentication system. | Partial | Integrate an identity solution and implement credential policy, MFA, recovery, throttling, account lifecycle, and account-state checks in the application. |
| V7.1–V7.6 | Session management | GOREST can issue access/refresh JWTs and its optional middleware validates signatures, configured algorithms, issuer, token type, required claims, and GOREST expiry; a valid refresh cookie can renew access and continue the request. It does not rotate/revoke refresh sessions or provide a complete session lifecycle. Redis is an integration component, not a session security policy. | Partial | If sessions are used, implement and test session rotation/revocation, account-state checks, cookie flags, CSRF defenses, and the intended refresh-token scope in the application. |
| V8.1.1, V8.2.1–V8.2.3, V8.3.1 | Function-, object-, and field-level authorization | Route handlers and data access are application-defined; GOREST has no built-in user or resource authorization model. | Application/deployment | Test authorization at a trusted service layer for every function, record, and field, including IDOR and mass-assignment cases. |
| V9.1.1–V9.1.3, V9.2.1–V9.2.3 | Self-contained token security | GOREST issues signed access/refresh JWTs and its optional middleware validates signatures, configured algorithms, issuer, token type, required identity/time claims, and expiry using GOREST's standardized `issue_at` and `expires_at` claims. It does not provide token revocation or account-state checks. | Partial | Test validation and renewal behavior; implement revocation and account-state checks as needed. Configure any external validator to enforce GOREST's claim semantics. |
| V10.1–V10.7 | OAuth and OpenID Connect | GOREST does not implement OAuth/OIDC clients, resource servers, or authorization servers. | Not provided | Assess the selected identity provider and each OAuth/OIDC flow in the application. |
| V11.4.1–V11.4.4, V11.5.1 | Hashing and random values | `helper.GenerateBcrypt` and `helper.GenerateArgon` provide password-hashing helpers. MD5 and SHA-512 helpers are documented as unsuitable for passwords/secrets; MD5 is deprecated. GOREST does not define an application's token or key lifecycle. | Partial | Use only purpose-appropriate password hashing; review parameters and migration policy. Do not use fast hashes for passwords, tokens, or secrets. Verify randomness for any application-generated security token. |
| V12.1.1, V12.3.1–V12.3.4 | Transport security | The HTTP server does not itself establish a production TLS deployment; TLS may be terminated by a trusted proxy. Database TLS options are driver-specific and commonly disabled by default. ScyllaDB supports TLS through `TLS: "enable"`, verified CA certificates, and optional mTLS; Redis TLS is optional and defaults off. GOREST warns about database TLS settings in production, but cannot verify network topology or certificates end to end. | Partial | Require HTTPS at the serving edge; configure and test certificate verification for every remote database and Redis connection; document Scylla transport protection separately. |
| V13.2.1–V13.2.5, V13.3.1–V13.3.2 | Backend communication and secrets | Config structs accept credentials and TLS settings but do not provide a secrets vault. GOREST cannot enforce least-privilege database/Redis accounts, network allowlists, or credential rotation. | Application/deployment | Load secrets from an approved secret manager, use least-privilege service accounts, rotate credentials, and restrict backend network access. |
| V13.4.1–V13.4.5 | Production configuration and information leakage | GOREST applies defaults and documents that debug and middleware settings need production review. Deployment packaging, source-control metadata exposure, directory listing, TRACE, and endpoint exposure are not controlled by the framework alone. | Application/deployment | Disable debug features and unnecessary routes in production; verify proxy/web-server settings and deployment artifacts. |
| V14.2.1–V14.2.4 | Sensitive-data protection | The GOREST logger heuristically redacts common credentials and personal-data patterns. Redaction is not comprehensive, and GOREST does not control application responses, caches, database encryption at rest, retention, or external telemetry. | Partial | Minimize sensitive data; do not place secrets in URLs or logs; set retention and access policies; verify encryption and cache behavior in the deployment. |
| V15.2.1–V15.2.3 | Dependencies and resource exhaustion | The framework uses Go modules and provides configurable request limits, body limits, timeouts, database pools, and Redis pools. These controls can be overridden and do not establish a dependency-remediation process or deployment-wide DoS protection. | Partial | Maintain dependency vulnerability review/update policy; test limits under representative load; apply edge protections and resource quotas. |
| V15.3.1–V15.3.7 | Data exposure, mass assignment, and request handling | ORM selection and update-column APIs let application code constrain fields, but handler-to-model binding and response field selection remain application decisions. Proxy-derived client identity also depends on trusted proxy configuration. | Application/deployment | Allowlist writable fields per operation, return only required fields, test duplicate/ambiguous parameters, and trust forwarded headers only from known proxies. |
| V16.2.1–V16.2.5, V16.3.1–V16.3.4, V16.4.1–V16.4.3 | Security logging and log protection | GOREST provides structured rotating logs and heuristic redaction. It does not produce application-specific authentication/authorization audit events or ensure tamper resistance, secure remote transport, alerting, retention, or synchronized clocks. | Partial | Define auditable security events, protect and forward logs, synchronize time, and test that sensitive values and log-injection payloads are handled safely. |
| V16.5.1–V16.5.3 | Error handling and fail-safe behavior | GOREST provides structured errors and panic-recovery middleware, but handlers and deployment components still determine whether internal details leak or failures fail open. | Partial | Test public error responses, panic paths, database/network failure paths, and transaction rollback behavior. |
| V17.1–V17.3 | WebRTC | GOREST does not provide WebRTC signaling, TURN, or media features. | Out of scope | Reassess if the application adds WebRTC functionality. |

## Additional implementation notes

The following notes supplement the existing control matrix with framework
behavior and features that are useful when assessing the controls above. They
do not replace or change the matrix's existing mappings, statuses, or
follow-up actions.

- **Response handling (V1, V4):** The optional `view` package can wrap JSON
  responses. It is not a general contextual encoder for HTML, JavaScript,
  URLs, or headers; handlers remain responsible for safe output in each
  context. `helper.BodyRequest` parses request bodies but does not replace
  endpoint-specific validation.
- **Database and ORM APIs (V1, V2, V8, V15):** GOREST has adapters for
  PostgreSQL, MySQL, SQL Server, Oracle, SQLite, MongoDB, and ScyllaDB, with
  native APIs and ORM operations whose coverage varies by driver. SQL
  transaction wrappers are available for relational drivers; MongoDB and
  ScyllaDB have driver-specific transaction behavior. ORM column selection
  and explicit update-column APIs can help constrain data access, but do not
  provide authorization or make application-selected fields safe by default.
  Bind values and allowlist dynamic identifiers and operators in native
  queries.
- **JWT and password helpers (V6, V7, V9, V11):** The filter validates JWT
  signatures, allowed signing method, issuer, token type, required claims,
  and expiry. A valid refresh cookie can renew access and also continue the
  current protected request. `filter.ConfigDefault` sets `HttpOnly` and
  `SameSite=Lax`; `Secure` defaults off, and partial config literals may
  zero boolean cookie flags. `helper.GenerateBcrypt` and
  `helper.GenerateArgon` provide password-hash helpers; Argon2id uses a
  per-hash cryptographically random salt. MD5 and SHA-512 are fast, unkeyed
  digests, not password hashing or authentication mechanisms. GOREST does
  not provide identity, account lifecycle, token revocation, key rotation, or
  a credential store.
- **HTTP middleware and transport (V2, V3, V4, V12, V15):** The default
  server installs CORS, panic recovery, request ID, compression, Helmet, and
  rate-limiting middleware. These settings are configurable and must be
  verified for the deployed application. CORS is not CSRF protection.
  GOREST also has configurable request body limits and finite server
  timeouts when its default core configuration is used. HTTP TLS is a
  deployment concern; database and Redis TLS are driver/configuration
  specific. Production-like startup checks warn or require explicit approval
  for insecure backend transport, but do not prove the network path is
  encrypted.
- **Migrations and startup (V13, V16):** Configured migrations run before
  seeders and before the HTTP listener starts. A startup-task failure aborts
  startup. GOREST uses transactions where supported and may execute matching
  `.down` compensation files, but compensation is bounded and best-effort;
  it cannot guarantee atomic rollback across every driver or outage. Review
  migration/seeder files and use appropriately restricted database
  privileges.
- **Operational endpoints (V13, V14):** By default, `/gorest/live`,
  `/gorest/ready`, `/gorest/metrics`, and `/gorest/storage` are registered.
  Readiness reports component states; metrics expose runtime values such as
  goroutine count, heap allocation, and GC cycles; storage reports
  driver-specific database usage estimates. `gorest.Config.DisableMonitoringEndpoints:
  true` prevents registration, but does not add authentication or network
  restrictions. Restrict access at the network/proxy layer if this
  information must remain private.
- **Logging (V14, V16):** GOREST uses structured rotating logs and heuristic
  redaction for recognized sensitive assignments, tokens, cookies, email
  addresses, and phone-like values. Redaction can miss secrets or produce
  false positives. The framework does not provide an application-specific
  audit trail, tamper-proof remote storage, alerting, or retention policy.

## Suggested verification sequence

1. Confirm the target application scope, data classification, deployment
   topology, and intended ASVS level.
2. Review high-impact application responsibilities first: authentication,
   authorization, session/token handling, request validation, and database
   query construction.
3. Verify production transport and server policy: HTTPS, CORS, security
   headers, trusted proxies, backend TLS, credentials, and request limits.
4. Test sensitive-data handling, logging, public error responses, dependencies,
   and failure behavior.
5. Extend this matrix with evidence links, test names, owners, and dated
   dispositions for every applicable ASVS requirement before making any
   compliance statement.
