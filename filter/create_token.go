package filter

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/elsyahtech/gorest/server"
	"github.com/golang-jwt/jwt/v5"
)

// CreateToken generates signed access and refresh tokens. It returns both token
// details and, unless CookieConfig.DisableStoreTokenCookie is true, stores the
// tokens in response cookies.
func (filter *Filter) CreateToken(ctx server.Context, tokenID string) (*TokenResponse, string, int, error) {
	if filter == nil || filter.JWT == nil || filter.Config == nil {
		return nil, filterConfigurationMessage, http.StatusNotImplemented,
			errors.New("filter configuration is error: config is nil")
	}

	if filter.Config.SignatureKey == nil || *filter.Config.SignatureKey == "" {
		return nil, filterConfigurationMessage, http.StatusBadRequest,
			errors.New("filter signature key is empty")
	}

	if tokenID == "" {
		return nil, "Ensure the token ID is provided and not empty.", http.StatusBadRequest,
			errors.New("token ID is empty")
	}

	if filter.JWT.Method == nil {
		return nil, "", http.StatusInternalServerError,
			errors.New("JWT signing method is not configured")
	}

	if filter.Config.AccessTokenDuration < time.Second || filter.Config.RefreshTokenDuration < time.Second {
		return nil, "Access and refresh token durations must be at least one second.", http.StatusBadRequest,
			errors.New("JWT token duration must be at least one second")
	}

	key := []byte(*filter.Config.SignatureKey)

	const (
		access  = "access"
		refresh = "refresh"
	)

	accessToken, message, httpCode, err := filter.execGenerateToken(ctx, tokenID, access, key)
	if err != nil {
		return nil, message, httpCode, fmt.Errorf("failed to generate access token: %w", err)
	}

	refreshToken, message, httpCode, err := filter.execGenerateToken(ctx, tokenID, refresh, key)
	if err != nil {
		return nil, message, httpCode, fmt.Errorf("failed to generate refresh token: %w", err)
	}

	return &TokenResponse{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
	}, "Tokens generated successfully", http.StatusOK, nil
}

func (filter *Filter) execGenerateToken(ctx server.Context, tokenID, tokenType string, key []byte) (*TokenDetail, string, int, error) {
	duration := filter.Config.AccessTokenDuration
	if tokenType == "refresh" {
		duration = filter.Config.RefreshTokenDuration
	} else if tokenType != "access" {
		return nil, "Ensure the token type is access or refresh.", http.StatusBadRequest,
			fmt.Errorf("unsupported token type %q", tokenType)
	}

	tokenGenerated, expiresAt, err := filter.generateToken(filter.JWT.Method, tokenID, tokenType, duration, key)
	if err != nil {
		return nil, "", http.StatusInternalServerError,
			fmt.Errorf("failed to sign %s token: %w", tokenType, err)
	}

	if filter.Config.CookieConfig.DisableStoreTokenCookie {
		return tokenGenerated, "", http.StatusOK, nil
	}

	message, httpCode, err := filter.createCookie(ctx, tokenType, tokenGenerated.Token, expiresAt)
	if err != nil {
		return nil, message, httpCode, fmt.Errorf("%w", err)
	}

	return tokenGenerated, "", http.StatusOK, nil
}

func (filter *Filter) generateToken(
	signingMethod jwt.SigningMethod,
	tokenID string,
	tokenType string,
	duration time.Duration,
	key []byte,
) (*TokenDetail, time.Time, error) {
	if signingMethod == nil {
		return nil, time.Time{}, errors.New("JWT signing method is not configured")
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
		return nil, time.Time{}, fmt.Errorf("%w", err)
	}

	return &TokenDetail{
		IssueAt:   now,
		TokenID:   tokenID,
		Token:     tokenGenerated,
		Issuer:    filter.Config.Issuer,
		TokenType: tokenType,
		ExpiresAt: expiresAt.Unix(),
	}, expiresAt, nil
}
