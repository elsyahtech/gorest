# GOREST Filter (Middleware Authentication)

GOREST uses JSON Web Tokens (JWTs) for access and refresh tokens. Its optional
Filter issues these tokens, stores them in `HttpOnly` cookies, and provides
middleware to authenticate requests. The filter is configured only when a
non-empty `SignatureKey` is configured. Without a key, the filter remains
disabled by default; applications that enable JWT must provide a valid
configuration.

```go
key := os.Getenv("GOREST_JWT_SIGNATURE_KEY")
filterConfig := filter.ConfigDefault
filterConfig.SignatureKey = &key
filterConfig.AccessTokenDuration = 15 * time.Minute
filterConfig.RefreshTokenDuration = 30 * 24 * time.Hour
filterConfig.CookieSecure = true

app := gorest.New()
app.Use(filterConfig)
```

## Required startup validation

JWT is optional: creating an application without registering a filter
configuration leaves JWT disabled. Passing a filter configuration to
`app.Use(...)` explicitly requests the JWT filter. GOREST normalizes the
configuration with `filter.Run(...)`; a nil, empty, or whitespace-only
`SignatureKey` aborts startup immediately. If the key is present but the
signing method or token durations are invalid, filter initialization logs the
error and leaves JWT disabled. `app.Use(...)` does not return an error because
its existing API is chainable.

When JWT initialization fails this way, the application can continue if it
does not use JWT. If route setup then accesses `app.Filter()` or creates
authentication middleware from an uninitialized filter, GOREST aborts startup
so the protected routes cannot silently run without authentication.

Accessing `app.Filter()` while JWT is disabled, or creating authentication
middleware from a `filter.Filter` that is not running, also aborts startup. This
makes route setup fail immediately instead of registering middleware that only
returns an internal server error when a request arrives. A disabled filter is
reported as “JWT filter is not running”; configure it with `filter.Run(...)`
and register the result using `app.Use(...)`. Route registrars
passed to `app.RegisterServices(...)` execute synchronously during registration;
call that method before `app.Start()` so configuration and middleware errors
are found before the HTTP listener opens. Configure JWT before registering
routes that use it:

```go
key := os.Getenv("GOREST_JWT_SIGNATURE_KEY")
config := filter.ConfigDefault
config.SignatureKey = &key
app.Use(config)

jwtFilter := app.Filter()
group := app.Group("/orm-queries")
group.Get("/users/select-all", jwtFilter.Authentication(), hndlr.GetUsersWithAllColumns)
```

GOREST can validate that a key is present, but cannot determine whether a
non-empty key is the intended secret. A key mismatch is detected when a token
signed with a different key is validated. Load the same high-entropy secret
from a secret manager or environment-specific secret store wherever tokens are
issued and verified; do not commit it to source control.

Use a high-entropy secret from a secret manager or environment-specific secret
store. Do not commit it to source control. `SigningMethod` defaults to `HS256`;
unsupported method names prevent the filter from running.

An HTTP handler can issue both tokens with:

```go
func issueTokens(ctx *gorest.Context) error {
	message, status, err := app.Filter().CreateToken(*ctx, "user-id")
	if err != nil {
		return ctx.Status(status).SendString(message)
	}
	return ctx.Status(status).SendString(message)
}
```

The generated cookies are named `access` and `refresh`. Each cookie's
`Max-Age` is derived from that token's configured duration, rounded up to the
nearest whole second. The JWT's own expiration remains authoritative.
`CookieMaxAge` remains for source compatibility but is deprecated and ignored
for these token cookies. Set `CookieSessionOnly` to create browser-session
cookies; the JWT's own expiration still applies. An empty `CookieDomain`
creates host-only cookies; set a domain only when sharing cookies across
subdomains is intended.

`filter.GetClientIP()` uses the first valid IP address in `X-Forwarded-For`,
which supports deployments behind reverse proxies and services such as
Cloudflare; if the header is absent or invalid, it falls back to Fiber's
connection IP. Because forwarded headers can be supplied by clients, configure
the public edge to overwrite or sanitize `X-Forwarded-For` and prevent direct
untrusted access to the application when using this value for security
decisions.

## Authenticating requests

`app.Filter().Authentication()` returns middleware that validates the access
token from the `Authorization` header when sent as a bearer token, or from the
`access` cookie. If access validation fails, it tries the `refresh` cookie. A
valid refresh token causes GOREST to issue a new access cookie and lets the
current request continue.
Consequently, a valid refresh cookie also authenticates requests protected by
this middleware; use a separate, restricted refresh endpoint if your
application requires refresh tokens to be used only for token renewal.

The middleware verifies the configured signing algorithm, signature, issuer,
token type, required identity and time claims, and GOREST's `expires_at`.
On success, it stores a `filter.TokenDetail` value in request locals under
`tokenJWT`. This verifies the signed token but does not look up the account,
check whether it is disabled, or authorize access to application resources.
Applications must perform those checks in their own handler or service layer.

## Authentication failure logging

Authentication event logging is controlled by `filter.Config.LoggerConfig`.
It does not change the middleware's HTTP responses or authentication result.
`DisableLogging: true` disables every authentication event. Otherwise,
`EnabledEvents` acts as an allowlist; by default only `missing_credentials`
is enabled to preserve the existing default behavior. Success events are
opt-in because they can generate a log entry for every authenticated request.
Set an explicit empty slice to disable all authentication event logs while
keeping logging enabled for future selected categories:

```go
config := filter.ConfigDefault
config.SignatureKey = &key
config.LoggerConfig.EnabledEvents = []filter.AuthenticationLogEvent{
    filter.AuthenticationEventMissingCredentials,
    filter.AuthenticationEventAccessTokenExpired,
    filter.AuthenticationEventRefreshTokenExpired,
}
app.Use(filter.Run(config))
```

Available event constants are:

| Event | Meaning |
|---|---|
| `AuthenticationEventMissingCredentials` | Neither access nor refresh token was provided. |
| `AuthenticationEventAccessTokenInvalid` | A supplied access token failed validation and authentication was ultimately rejected. |
| `AuthenticationEventAccessTokenExpired` | A supplied access token expired and authentication was ultimately rejected. |
| `AuthenticationEventRefreshTokenInvalid` | A supplied refresh token failed validation. |
| `AuthenticationEventRefreshTokenExpired` | A supplied refresh token expired. |
| `AuthenticationEventRefreshFailed` | A valid refresh token could not be used to issue a replacement access token. |
| `AuthenticationEventAuthenticationSucceeded` | A valid access token authenticated the request. |
| `AuthenticationEventRefreshSucceeded` | A refresh token issued a replacement access token and authenticated the current request. |

An access-token failure that is recovered by a valid refresh token is not
logged as an authentication failure. If both supplied tokens fail, the
selected access and refresh failure events can both be recorded. If both
success events are enabled, a request authenticated through refresh emits both
`authentication_succeeded` and `refresh_succeeded`; otherwise each event can
be selected independently. Failure events are logged at `WARN`, while success
events are logged at `INFO`. Events use the `authentication_event` structured
log field. Token values, authorization headers, and raw validation errors are
not included in these event logs.

The filter only records events from its own JWT authentication and refresh
flow. Authorization decisions (for example, an authenticated identity being
denied access to a resource), credential login, MFA, account lockout, logout,
token revocation, and credential changes belong to the application or identity
provider that implements those workflows. Record those events at the point
where the application makes the decision.

## GOREST token claims

GOREST currently uses these claim names:

| Claim | Meaning |
|---|---|
| `issue_at` | Unix timestamp in seconds when the token was issued |
| `token_id` | Application-provided identity or token identifier |
| `issuer` | Issuer configured by GOREST from the application name |
| `token_type` | `access` or `refresh` |
| `expires_at` | Unix timestamp in seconds when the token expires |

These are GOREST's standardized claim names and meanings. When validating
tokens outside GOREST's authentication middleware, configure the validator to
verify the signature, allowed algorithm, issuer, token type, required claims,
and expiration using GOREST's `expires_at` claim before trusting the claims.

## Cookie transport warning

`CookieSecure` defaults to `false` so applications that intentionally operate
over HTTP, including trusted intranet deployments, are not prevented from
starting. In production-like environments GOREST logs a warning when the
configured JWT cookies do not have the `Secure` attribute. This is a warning,
not an enforcement: cookies may be sent over unencrypted HTTP. Set
`CookieSecure: true` when clients access the application over HTTPS, and verify
TLS at the application or trusted reverse-proxy boundary.

When starting from `filter.ConfigDefault`, `HttpOnly` defaults to `true` and
`SameSite` defaults to `Lax`. A partial `filter.Config` literal uses Go's zero
values for boolean fields; set `CookieHTTPOnly` explicitly or start from
`filter.ConfigDefault`. Review cookie domain, path, SameSite policy, CSRF
defenses, and the deployment's transport security for the application.
