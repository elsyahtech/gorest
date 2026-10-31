package filter

import "time"

const (
	// DefaultSigningMethod is the default algorithm method used for signing tokens.
	DefaultSigningMethod = "HS256"

	// DefaultAccessTokenDuration is the default lifespan duration for access tokens.
	DefaultAccessTokenDuration = 15 * time.Minute

	// DefaultRefreshTokenDuration is the default lifespan duration for refresh tokens.
	DefaultRefreshTokenDuration = 30 * 24 * time.Hour

	// DefaultIssuer is the default identifier for the principal that issued the token.
	DefaultIssuer = "gorest"

	// DefaultCookieSameSite is the default SameSite attribute policy for cookies.
	DefaultCookieSameSite = "Lax"

	// DefaultCookieDomain is the default domain scope for cookies.
	DefaultCookieDomain = ""

	// DefaultCookiePath is the default path scope for cookies.
	DefaultCookiePath = "/"

	// DefaultCookieMaxAge is deprecated. Token cookies use the corresponding token duration.
	// It is retained for source compatibility.
	DefaultCookieMaxAge = 0

	// DefaultCookieSecure restricts cookie transmission to secure (HTTPS) connections only.
	DefaultCookieSecure = false

	// DefaultCookieHTTPOnly prevents client-side script access to the cookie.
	DefaultCookieHTTPOnly = true

	// DefaultCookieSessionOnly specifies whether the cookie expires when the browser session ends.
	DefaultCookieSessionOnly = false
)

const filterConfigurationMessage = "Ensure the filter is configured with a valid signature key."

const (
	errorKey   = "Error"
	messageKey = "Message"
)
