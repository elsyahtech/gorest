# GOREST Filter (Middleware Authentication)

GOREST uses JSON Web Tokens (JWTs) for access and refresh tokens. Its optional
Filter issues these tokens, stores them in `HttpOnly` cookies, and provides
middleware to authenticate requests. The filter is initialized only when a
non-empty `SignatureKey` is configured. Without a key, the filter remains
disabled.

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

Use a high-entropy secret from a secret manager or environment-specific secret
store. Do not commit it to source control. `SigningMethod` defaults to `HS256`;
unsupported method names prevent filter initialization.

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

`filter.GetClientIP()` follows Fiber's configured client-IP and trusted-proxy
settings. Do not trust forwarded headers unless the server is configured to
accept them only from known proxies.

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
