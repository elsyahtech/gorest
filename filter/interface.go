package filter

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type Filter struct {
	JWT    *jwt.Token
	Config *Config
}

type Cookie struct {
	// Expires absolute point in time when the cookie expires
	Expires time.Time

	// Name name or key of the cookie
	Name string

	// Value value stored inside the cookie
	Value string

	// Path URL path scope where the cookie is valid
	Path string

	// Domain domain scope where the cookie is valid
	Domain string

	// SameSite cookie SameSite attribute policy (e.g., Lax, Strict, None)
	SameSite string

	// MaxAge maximum age of the cookie in seconds
	MaxAge int

	// Secure restricts cookie transmission to secure (HTTPS) connections only
	Secure bool

	// HTTPOnly prevents client-side scripts from accessing the cookie
	HTTPOnly bool

	// SessionOnly specifies whether the cookie should expire when the browser session ends
	SessionOnly bool
}

type TokenDetail struct {
	// IssueAt represents the timestamp when the token was generated/issued.
	IssueAt time.Time

	// TokenID is the unique identifier for the token (often used for revocation or tracking).
	TokenID string

	// Token is the raw JWT signature string (Access Token or Refresh Token).
	Token string

	// Issuer represents the entity that issued the token (e.g., application name or service).
	Issuer string

	// TokenType specifies the classification of the token (e.g., "access" or "refresh").
	TokenType string

	// ExpiresAt represents the expiration time of the token expressed as a Unix timestamp (seconds).
	ExpiresAt int64
}
