package filter

import (
	"errors"
	"net/http"
	"time"

	"github.com/elsyahtech/gorest/server"
	"github.com/gofiber/fiber/v3"
)

func (filter *Filter) Authentication() server.Handler {
	return func(ctx *server.Context) error {
		if filter == nil || filter.Config == nil || filter.Config.SignatureKey == nil || *filter.Config.SignatureKey == "" {
			return ctx.Status(http.StatusInternalServerError).JSON(fiber.Map{
				errorKey:   "authentication filter is not configured",
				messageKey: "The server cannot validate authentication tokens",
			})
		}

		accessToken, refreshToken := extractTokens(ctx)
		claims, _, _, accessErr := filter.validateAccessToken(*ctx, accessToken)
		tokenString := accessToken

		if accessErr != nil {
			refreshClaims, message, status, err := filter.validateRefreshToken(*ctx, refreshToken)
			if err != nil {
				if status < http.StatusBadRequest || status > 599 {
					status = http.StatusUnauthorized
				}

				return ctx.Status(status).JSON(fiber.Map{
					errorKey:   err.Error(),
					messageKey: message,
				})
			}

			claims = refreshClaims
			tokenString = refreshToken
		}

		detail, err := tokenDetailFromClaims(claims, tokenString)
		if err != nil {
			return ctx.Status(http.StatusUnauthorized).JSON(fiber.Map{
				errorKey:   err.Error(),
				messageKey: "The token contains invalid claims",
			})
		}

		ctx.Locals("tokenJWT", detail)

		return ctx.Next()
	}
}

func tokenDetailFromClaims(claims map[string]any, tokenString string) (TokenDetail, error) {
	issuedAt, err := unixTimestampClaim(claims, "issue_at")
	if err != nil {
		return TokenDetail{}, err
	}

	expiresAt, err := unixTimestampClaim(claims, "expires_at")
	if err != nil {
		return TokenDetail{}, err
	}

	tokenID, tokenIDOK := claims["token_id"].(string)
	if !tokenIDOK || tokenID == "" {
		return TokenDetail{}, errors.New("token is missing a valid token_id")
	}

	issuer, issuerOK := claims["issuer"].(string)
	if !issuerOK {
		return TokenDetail{}, errors.New("token is missing a valid issuer")
	}

	tokenType, tokenTypeOK := claims["token_type"].(string)
	if !tokenTypeOK {
		return TokenDetail{}, errors.New("token is missing a valid token_type")
	}

	return TokenDetail{
		IssueAt:   time.Unix(issuedAt, 0),
		TokenID:   tokenID,
		Token:     tokenString,
		Issuer:    issuer,
		TokenType: tokenType,
		ExpiresAt: expiresAt,
	}, nil
}

// GetClientIP returns the client IP according to Fiber's proxy configuration.
func GetClientIP(ctx server.Context) string {
	return ctx.IP()
}
