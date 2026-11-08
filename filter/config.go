package filter

import (
	golog "log"
	"time"
)

type Config struct {
	// CookieConfig holds the configuration for cookie settings
	CookieConfig CookieConfig

	// LoggerConfig holds the configuration for logging authentication events
	LoggerConfig LoggerConfig

	// SignatureKey secret key used to sign tokens
	SignatureKey *string

	// SigningMethod algorithm method used for signing tokens (e.g., HS256)
	SigningMethod string

	// Issuer identifies the principal that issued the token
	Issuer string

	// AccessTokenDuration lifespan duration for access tokens
	AccessTokenDuration time.Duration

	// RefreshTokenDuration lifespan duration for refresh tokens
	RefreshTokenDuration time.Duration

	// AllowedIPs is an optional allowlist of IP addresses or CIDR ranges for
	// routes protected by Authentication(). When non-empty, peers not in the
	// list are rejected. The connection peer IP is checked; forwarded headers
	// are not trusted.
	AllowedIPs []string

	// BlockedIPs is an optional denylist of IP addresses or CIDR ranges for
	// routes protected by Authentication(). A blocked peer is rejected even
	// when it also matches AllowedIPs. The connection peer IP is checked;
	// forwarded headers are not trusted.
	BlockedIPs []string
}

type CookieConfig struct {
	// SameSite cookie SameSite attribute policy
	CookieSameSite string

	// Domain cookie domain scope
	CookieDomain string

	// Path cookie path scope
	CookiePath string

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

	// DisableStoreTokenCookie disables storing access and refresh tokens in cookies.
	DisableStoreTokenCookie bool
}

type LoggerConfig struct {
	// DisableLogging disables logging of authentication events.
	DisableLogging bool

	// EnabledEvents selects which authentication events are logged.
	// When nil, the default event selection is used.
	EnabledEvents []AuthenticationLogEvent
}

type AuthenticationLogEvent string

const (
	AuthenticationEventMissingCredentials      AuthenticationLogEvent = "missing_credentials"
	AuthenticationEventAccessTokenInvalid      AuthenticationLogEvent = "access_token_invalid"
	AuthenticationEventAccessTokenExpired      AuthenticationLogEvent = "access_token_expired"
	AuthenticationEventRefreshTokenInvalid     AuthenticationLogEvent = "refresh_token_invalid"
	AuthenticationEventRefreshTokenExpired     AuthenticationLogEvent = "refresh_token_expired"
	AuthenticationEventRefreshFailed           AuthenticationLogEvent = "refresh_failed"
	AuthenticationEventAuthenticationSucceeded AuthenticationLogEvent = "authentication_succeeded"
	AuthenticationEventRefreshSucceeded        AuthenticationLogEvent = "refresh_succeeded"
)

var ConfigDefault = Config{
	CookieConfig: CookieConfig{
		CookieSameSite:          DefaultCookieSameSite,
		CookieDomain:            DefaultCookieDomain,
		CookiePath:              DefaultCookiePath,
		CookieMaxAge:            DefaultCookieMaxAge,
		CookieSecure:            DefaultCookieSecure,
		CookieHTTPOnly:          DefaultCookieHTTPOnly,
		CookieSessionOnly:       DefaultCookieSessionOnly,
		DisableStoreTokenCookie: DisableStoreTokenCookie,
	},
	LoggerConfig: LoggerConfig{
		DisableLogging: false,
		EnabledEvents:  []AuthenticationLogEvent{AuthenticationEventMissingCredentials},
	},
	SignatureKey:         nil,
	SigningMethod:        DefaultSigningMethod,
	AccessTokenDuration:  DefaultAccessTokenDuration,
	RefreshTokenDuration: DefaultRefreshTokenDuration,
	Issuer:               DefaultIssuer,
}

func configDefault(config ...Config) Config {
	if len(config) == 0 {
		cfg := ConfigDefault
		cfg.LoggerConfig.EnabledEvents = cloneAuthenticationLogEvents(ConfigDefault.LoggerConfig.EnabledEvents)

		return cfg
	}

	cfg := config[0]
	cfg.AllowedIPs = append([]string(nil), cfg.AllowedIPs...)
	cfg.BlockedIPs = append([]string(nil), cfg.BlockedIPs...)
	if cfg.LoggerConfig.EnabledEvents == nil {
		cfg.LoggerConfig.EnabledEvents = cloneAuthenticationLogEvents(ConfigDefault.LoggerConfig.EnabledEvents)
	} else {
		cfg.LoggerConfig.EnabledEvents = cloneAuthenticationLogEvents(cfg.LoggerConfig.EnabledEvents)
	}

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

	if cfg.CookieConfig.CookieSameSite == "" {
		cfg.CookieConfig.CookieSameSite = ConfigDefault.CookieConfig.CookieSameSite
	}

	if cfg.CookieConfig.CookieDomain == "" {
		cfg.CookieConfig.CookieDomain = ConfigDefault.CookieConfig.CookieDomain
	}

	if cfg.CookieConfig.CookiePath == "" {
		cfg.CookieConfig.CookiePath = ConfigDefault.CookieConfig.CookiePath
	}

	if cfg.CookieConfig.CookieMaxAge == 0 {
		cfg.CookieConfig.CookieMaxAge = ConfigDefault.CookieConfig.CookieMaxAge
	}

	if cfg.CookieConfig.CookieMaxAge < 0 {
		golog.Fatalf("CookieMaxAge must be a non-negative integer, got %d", cfg.CookieConfig.CookieMaxAge)
	}

	return cfg
}

func cloneAuthenticationLogEvents(events []AuthenticationLogEvent) []AuthenticationLogEvent {
	if events == nil {
		return nil
	}

	cloned := make([]AuthenticationLogEvent, len(events))
	copy(cloned, events)

	return cloned
}
