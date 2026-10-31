package filter

import "time"

type Config struct {
	// SignatureKey secret key used to sign tokens
	SignatureKey *string

	// SigningMethod algorithm method used for signing tokens (e.g., HS256)
	SigningMethod string

	// Issuer identifies the principal that issued the token
	Issuer string

	// SameSite cookie SameSite attribute policy
	CookieSameSite string

	// Domain cookie domain scope
	CookieDomain string

	// Path cookie path scope
	CookiePath string

	// AccessTokenDuration lifespan duration for access tokens
	AccessTokenDuration time.Duration

	// RefreshTokenDuration lifespan duration for refresh tokens
	RefreshTokenDuration time.Duration

	// CookieMaxAge is deprecated. Access and refresh token cookie lifetimes are
	// derived from their respective token durations.
	//
	// Deprecated: use AccessTokenDuration and RefreshTokenDuration.
	CookieMaxAge int

	// Secure restricts cookie transmission to secure (HTTPS) connections only
	CookieSecure bool

	// HTTPOnly prevents client-side script access to the cookie
	CookieHTTPOnly bool

	// SessionOnly specifies whether the cookie expires when the browser session ends
	CookieSessionOnly bool
}

var ConfigDefault = Config{
	SignatureKey:         nil,
	SigningMethod:        DefaultSigningMethod,
	AccessTokenDuration:  DefaultAccessTokenDuration,
	RefreshTokenDuration: DefaultRefreshTokenDuration,
	Issuer:               DefaultIssuer,
	CookieSameSite:       DefaultCookieSameSite,
	CookieDomain:         DefaultCookieDomain,
	CookiePath:           DefaultCookiePath,
	CookieMaxAge:         DefaultCookieMaxAge,
	CookieSecure:         DefaultCookieSecure,
	CookieHTTPOnly:       DefaultCookieHTTPOnly,
	CookieSessionOnly:    DefaultCookieSessionOnly,
}

func configDefault(config ...Config) Config {
	if len(config) == 0 {
		return ConfigDefault
	}

	cfg := config[0]

	if cfg.SigningMethod == "" {
		cfg.SigningMethod = ConfigDefault.SigningMethod
	}

	if cfg.AccessTokenDuration == 0 {
		cfg.AccessTokenDuration = ConfigDefault.AccessTokenDuration
	}

	if cfg.RefreshTokenDuration == 0 {
		cfg.RefreshTokenDuration = ConfigDefault.RefreshTokenDuration
	}

	if cfg.Issuer == "" {
		cfg.Issuer = ConfigDefault.Issuer
	}

	if cfg.CookieSameSite == "" {
		cfg.CookieSameSite = ConfigDefault.CookieSameSite
	}

	if cfg.CookieDomain == "" {
		cfg.CookieDomain = ConfigDefault.CookieDomain
	}

	if cfg.CookiePath == "" {
		cfg.CookiePath = ConfigDefault.CookiePath
	}

	if cfg.CookieMaxAge == 0 {
		cfg.CookieMaxAge = ConfigDefault.CookieMaxAge
	}

	return cfg
}
