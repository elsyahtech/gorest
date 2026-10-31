package filter

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/elsyahtech/gorest/server"
	"github.com/golang-jwt/jwt/v5"
)

// CreateToken generates a new JWT token with the provided claims and signs it using the specified signature key.
func (filter *Filter) CreateToken(ctx server.Context, tokenID string) (string, int, error) {
	if filter == nil || filter.JWT == nil || filter.Config == nil {
		return filterConfigurationMessage, http.StatusNotImplemented,
			errors.New("filter configuration is error: config is nil")
	}

	if filter.Config.SignatureKey == nil || *filter.Config.SignatureKey == "" {
		return filterConfigurationMessage, http.StatusBadRequest,
			errors.New("filter signature key is empty")
	}

	if tokenID == "" {
		return "Ensure the token ID is provided and not empty.", http.StatusBadRequest,
			errors.New("token ID is empty")
	}

	if filter.JWT.Method == nil {
		return "", http.StatusInternalServerError,
			errors.New("JWT signing method is not configured")
	}

	if filter.Config.AccessTokenDuration < time.Second || filter.Config.RefreshTokenDuration < time.Second {
		return "Access and refresh token durations must be at least one second.", http.StatusBadRequest,
			errors.New("JWT token duration must be at least one second")
	}

	key := []byte(*filter.Config.SignatureKey)

	const (
		access  = "access"
		refresh = "refresh"
	)

	message, httpCode, err := filter.execGenerateToken(ctx, tokenID, access, key)
	if err != nil {
		return message, httpCode, fmt.Errorf("failed to generate access token: %w", err)
	}

	message, httpCode, err = filter.execGenerateToken(ctx, tokenID, refresh, key)
	if err != nil {
		return message, httpCode, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return "Token saved successfully", http.StatusOK, nil
}

func (filter *Filter) execGenerateToken(ctx server.Context, tokenID, tokenType string, key []byte) (string, int, error) {
	duration := filter.Config.AccessTokenDuration
	if tokenType == "refresh" {
		duration = filter.Config.RefreshTokenDuration
	} else if tokenType != "access" {
		return "Ensure the token type is access or refresh.", http.StatusBadRequest,
			fmt.Errorf("unsupported token type %q", tokenType)
	}

	tokenGenerated, expiresAt, err := filter.generateToken(filter.JWT.Method, tokenID, tokenType, duration, key)
	if err != nil {
		return "", http.StatusInternalServerError,
			fmt.Errorf("failed to sign %s token: %w", tokenType, err)
	}

	message, httpCode, err := filter.createCookie(ctx, tokenType, tokenGenerated, expiresAt)
	if err != nil {
		return message, httpCode, fmt.Errorf("%w", err)
	}

	return "", http.StatusOK, nil
}

func (filter *Filter) generateToken(
	signingMethod jwt.SigningMethod,
	tokenID string,
	tokenType string,
	duration time.Duration,
	key []byte,
) (string, time.Time, error) {
	if signingMethod == nil {
		return "", time.Time{}, errors.New("JWT signing method is not configured")
	}

	now := time.Now()
	expiresAt := now.Add(duration)

	token := jwt.NewWithClaims(signingMethod, jwt.MapClaims{
		"issue_at":   now.Unix(),
		"token_id":   tokenID,
		"issuer":     filter.Config.Issuer,
		"token_type": tokenType,
		"expires_at": expiresAt.Unix(),
	})

	tokenGenerated, err := token.SignedString(key)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("%w", err)
	}

	return tokenGenerated, expiresAt, nil
}
