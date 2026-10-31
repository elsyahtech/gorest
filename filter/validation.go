package filter

import (
	"errors"
	"fmt"
	"math"
	"net/http"
	"time"

	"github.com/elsyahtech/gorest/server"
	"github.com/golang-jwt/jwt/v5"
)

func (filter *Filter) validateAccessToken(ctx server.Context, accessToken string) (jwt.MapClaims, string, int, error) {
	const responseMessage = "A valid access token is required"
	if accessToken == "" {
		return nil, responseMessage, http.StatusUnauthorized, errors.New("missing access token")
	}

	claims, status, err := filter.validateToken(accessToken, "access")
	if err != nil {
		return nil, responseMessage, status, fmt.Errorf("validate access token: %w", err)
	}

	claims, message, status, err := checkTokenExpired(claims)
	if err != nil {
		if errors.Is(err, errTokenExpired) && ctx.Cookies("access") == accessToken {
			filter.deleteTokenCookie(ctx, "access", accessToken)
		}

		return nil, message, status, fmt.Errorf("validate access token expiration: %w", err)
	}

	return claims, "", http.StatusOK, nil
}

func (filter *Filter) validateToken(tokenString, expectedType string) (jwt.MapClaims, int, error) {
	if filter == nil || filter.Config == nil || filter.Config.SignatureKey == nil || *filter.Config.SignatureKey == "" {
		return nil, http.StatusInternalServerError, errors.New("filter signing key is not configured")
	}

	if filter.JWT == nil || filter.JWT.Method == nil {
		return nil, http.StatusInternalServerError, errors.New("filter signing method is not configured")
	}

	parsed, err := jwt.Parse(tokenString, func(_ *jwt.Token) (any, error) {
		return []byte(*filter.Config.SignatureKey), nil
	}, jwt.WithValidMethods([]string{filter.JWT.Method.Alg()}))
	if err != nil {
		return nil, http.StatusUnauthorized, fmt.Errorf("parse token: %w", err)
	}

	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok || !parsed.Valid {
		return nil, http.StatusUnauthorized, errors.New("invalid token")
	}

	if err := validateTokenClaims(claims, expectedType, filter.Config.Issuer); err != nil {
		return nil, http.StatusUnauthorized, err
	}

	return claims, http.StatusOK, nil
}

func validateTokenClaims(claims jwt.MapClaims, expectedType, expectedIssuer string) error {
	if tokenType, ok := claims["token_type"].(string); !ok || tokenType != expectedType {
		return fmt.Errorf("token type must be %q", expectedType)
	}

	if issuer, ok := claims["issuer"].(string); !ok || issuer != expectedIssuer {
		return errors.New("token issuer does not match")
	}

	if tokenID, ok := claims["token_id"].(string); !ok || tokenID == "" {
		return errors.New("token is missing a valid token_id")
	}

	if _, err := unixTimestampClaim(claims, "issue_at"); err != nil {
		return err
	}

	return nil
}

func (filter *Filter) validateRefreshToken(ctx server.Context, refreshToken string) (jwt.MapClaims, string, int, error) {
	const responseMessage = "A valid refresh token is required"
	if refreshToken == "" {
		return nil, responseMessage, http.StatusUnauthorized, errors.New("missing refresh token")
	}

	claims, status, err := filter.validateToken(refreshToken, "refresh")
	if err != nil {
		return nil, responseMessage, status, fmt.Errorf("validate refresh token: %w", err)
	}

	claims, message, status, err := checkTokenExpired(claims)
	if err != nil {
		if errors.Is(err, errTokenExpired) {
			filter.deleteTokenCookie(ctx, "refresh", refreshToken)
		}

		return nil, message, status, fmt.Errorf("validate refresh token expiration: %w", err)
	}

	tokenID, ok := claims["token_id"].(string)
	if !ok {
		return nil, responseMessage, http.StatusUnauthorized, errors.New("token is missing a valid token_id")
	}

	key := []byte(*filter.Config.SignatureKey)

	if message, status, err := filter.execGenerateToken(ctx, tokenID, "access", key); err != nil {
		return nil, message, status, fmt.Errorf("generate new access token: %w", err)
	}

	return claims, "", http.StatusOK, nil
}

func (filter *Filter) deleteTokenCookie(ctx server.Context, name, value string) {
	deleteCookie(ctx, name, &Cookie{
		Expires:     time.Now().Add(-time.Second),
		Name:        name,
		Value:       value,
		Path:        filter.Config.CookiePath,
		Domain:      filter.Config.CookieDomain,
		SameSite:    filter.Config.CookieSameSite,
		Secure:      filter.Config.CookieSecure,
		HTTPOnly:    filter.Config.CookieHTTPOnly,
		SessionOnly: filter.Config.CookieSessionOnly,
	})
}

var errTokenExpired = errors.New("token has expired")

func checkTokenExpired(claims jwt.MapClaims) (jwt.MapClaims, string, int, error) {
	expiresAt, err := unixTimestampClaim(claims, "expires_at")
	if err != nil {
		const message = "The token is missing a valid expires_at claim"
		return nil, message, http.StatusUnauthorized, err
	}

	if time.Now().Unix() >= expiresAt {
		const message = "The token has expired; please authenticate again"
		return nil, message, http.StatusUnauthorized, fmt.Errorf("%w at %d", errTokenExpired, expiresAt)
	}

	return claims, "", http.StatusOK, nil
}

func unixTimestampClaim(claims jwt.MapClaims, name string) (int64, error) {
	value, ok := claims[name].(float64)
	if value < -0x1p63 || value >= 0x1p63 || !ok || math.IsNaN(value) ||
		math.IsInf(value, 0) || value != math.Trunc(value) {
		return 0, fmt.Errorf("missing or invalid %s claim", name)
	}

	return int64(value), nil
}
