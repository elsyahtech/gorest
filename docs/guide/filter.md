# GOREST Filter (Middleware Authentication)

GOREST uses JSON Web Tokens (JWTs) for access and refresh tokens. The access
token is the normal credential for protected requests. The refresh token lets a
client continue a session by obtaining a replacement access token after the
access token expires. Its optional Filter issues both tokens, stores them in
`HttpOnly` cookies by default, and provides middleware to authenticate
requests. The filter is configured only when a
non-empty `SignatureKey` is configured. Without a key, the filter remains
disabled by default; applications that enable JWT must provide a valid
configuration.

```go
key := os.Getenv("GOREST_JWT_SIGNATURE_KEY")
filterConfig := filter.ConfigDefault
filterConfig.SignatureKey = &key
filterConfig.AccessTokenDuration = 15 * time.Minute
filterConfig.RefreshTokenDuration = 30 * 24 * time.Hour
filterConfig.CookieConfig.CookieSecure = true

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
	tokenResponse, message, status, err := app.Filter().CreateToken(*ctx, "user-id")
	if err != nil {
		return ctx.Status(status).SendString(message)
	}
	return ctx.Status(status).JSON(tokenResponse)
}
```

`CreateToken()` returns both generated token details in `TokenResponse`.
By default, it also sets cookies named `access` and `refresh`. Set
`CookieConfig.DisableStoreTokenCookie` to `true` when the application wants to
transport or store the returned tokens itself; in that mode GOREST does not
set token cookies. The application is then responsible for protecting tokens
in its response and client-side storage. This option controls token storage
when tokens are issued; it does not make `Authentication()` return a renewed
token to the application.

Each cookie's `Max-Age` is derived from that token's configured duration,
rounded up to the nearest whole second. The JWT's own expiration remains
authoritative. `CookieConfig.CookieMaxAge` remains for source compatibility
but is deprecated and ignored for these token cookies. Set
`CookieConfig.CookieSessionOnly` to create browser-session cookies; the JWT's
own expiration still applies. An empty `CookieConfig.CookieDomain` creates
host-only cookies; set a domain only when sharing cookies across subdomains is
intended.

### Optional IP restrictions

`filter.Config.AllowedIPs` and `filter.Config.BlockedIPs` optionally restrict
requests that pass through `Authentication()`:

```go
filterConfig.AllowedIPs = []string{"203.0.113.8", "198.51.100.0/24"}
filterConfig.BlockedIPs = []string{"198.51.100.42"}
```

Entries accept individual IPv4/IPv6 addresses or CIDR ranges. An empty
allowlist allows any peer not in the blocklist. When the allowlist is
non-empty, peers outside it are denied. A blocklist match always denies,
including when that peer also matches the allowlist. Denied requests receive
HTTP 403. Invalid IP/CIDR entries prevent the filter from initializing.

These settings are an optional IP access control for routes where the
application installs `Authentication()`; they are not a host/network firewall
and do not protect unguarded routes or other services. GOREST checks the
connection peer IP and does not trust `X-Forwarded-For` for this decision. If
the application is behind a reverse proxy, the peer is normally the proxy.
For example, when browser API requests pass through Cloudflare to the GOREST
origin, GOREST normally sees a Cloudflare edge address, not the browser's
public IP. Allowlisting Cloudflare addresses therefore permits requests
arriving through those edges; it does not distinguish or allowlist individual
visitors. Blocklisting an edge address may deny many unrelated visitors.
Cookies sent by the browser do not change the network peer IP used by this
check. The same applies when a frontend server proxies requests: GOREST sees
that server or its immediate proxy.

Use the trusted edge's firewall/WAF rules to restrict original visitor IPs.
Alternatively, a trusted-proxy-aware application design can use a forwarded
client-IP header only when direct untrusted access to the origin is prevented
and the trusted proxy overwrites that header. Do not treat
`filter.GetClientIP()` as safe for access control by itself.

`filter.GetClientIP()` uses the first valid IP address in `X-Forwarded-For`,
which may be useful for application-level logging or display behind a proxy;
if the header is absent or invalid, it falls back to Fiber's connection IP.
Because forwarded headers can be supplied by clients, configure the public
edge to overwrite or sanitize `X-Forwarded-For` before relying on this helper
for any security-sensitive decision.

## Authenticating requests

`app.Filter().Authentication()` returns middleware that validates an access
token from the `Authorization` header using the `Bearer` scheme, or from the
`access` cookie when no Bearer token is supplied. A Bearer token is always
validated as an **access token**; putting a refresh token in the `Authorization`
header does not make it a refresh credential. Refresh tokens are read only
from the `refresh` cookie.

The middleware uses the refresh cookie only when the access token is otherwise
valid but has expired. A malformed token, invalid signature, wrong issuer or
token type, or missing/invalid required claim is rejected immediately; a
refresh cookie does not recover an invalid access token. When the access token
has expired, a valid refresh token causes GOREST to issue a replacement access
token and, by default, set it in the `access` cookie. The middleware then
continues the **same request** using the validated refresh token's identity;
the client does not need to repeat the request with the new access token.
This fallback is intended to keep cookie-based sessions seamless and avoid an
extra round trip. It means a valid refresh cookie can authorize the current
protected request when paired with an expired access token.

On the fallback path, the `filter.TokenDetail` stored in request locals under
`tokenJWT` describes the refresh token that authenticated the request
(`TokenType` is `"refresh"`); it does not describe the newly issued access
token. Applications that inspect `tokenJWT` should account for this, and must
still perform their own account-state and resource-authorization checks.

`CookieConfig.DisableStoreTokenCookie` disables writing both initial and
renewed token cookies. With this option enabled, a valid refresh cookie can
still authorize the current request, but `Authentication()` does not expose
the replacement access token to the handler or return it in the response.
Applications that manage token transport themselves need to implement their
own renewal flow; the middleware fallback alone cannot deliver the replacement
token to that transport. For integrations using Bearer authentication without
a refresh cookie, an invalid or expired access token is rejected and the
integration must obtain another access token through its own credential flow.

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
| `AuthenticationEventAuthenticationSucceeded` | A valid access token, or a valid refresh token on the fallback path, authenticated the request. |
| `AuthenticationEventRefreshSucceeded` | A refresh token issued a replacement access token and authenticated the current request. |

An expired access token that is recovered by a valid refresh token is not
logged as an authentication failure. A refresh failure event is recorded only
when the access token is expired and refresh validation is attempted. If both
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

`CookieConfig.CookieSecure` defaults to `false` so applications that
intentionally operate over HTTP, including trusted intranet deployments, are
not prevented from starting. In production-like environments GOREST logs a
warning when the configured JWT cookies do not have the `Secure` attribute.
This is a warning, not an enforcement: cookies may be sent over unencrypted
HTTP. Set `CookieConfig.CookieSecure: true` when clients access the application
over HTTPS, and verify TLS at the application or trusted reverse-proxy
boundary.

When starting from `filter.ConfigDefault`, `CookieConfig.CookieHTTPOnly`
defaults to `true` and `CookieConfig.CookieSameSite` defaults to `Lax`. A
partial `filter.Config` literal uses Go's zero values for boolean fields; set
`CookieConfig.CookieHTTPOnly` explicitly or start from
`filter.ConfigDefault`. Review cookie domain, path, SameSite policy, CSRF
defenses, and the deployment's transport security for the application.
