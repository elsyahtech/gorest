package filter

import "time"

type Config struct {
	// LoggerConfig holds the configuration for logging authentication events
	LoggerConfig LoggerConfig

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
	LoggerConfig: LoggerConfig{
		DisableLogging: false,
		EnabledEvents:  []AuthenticationLogEvent{AuthenticationEventMissingCredentials},
	},
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
		cfg := ConfigDefault
		cfg.LoggerConfig.EnabledEvents = cloneAuthenticationLogEvents(ConfigDefault.LoggerConfig.EnabledEvents)

		return cfg
	}

	cfg := config[0]
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

func cloneAuthenticationLogEvents(events []AuthenticationLogEvent) []AuthenticationLogEvent {
	if events == nil {
		return nil
	}

	cloned := make([]AuthenticationLogEvent, len(events))
	copy(cloned, events)

	return cloned
}
