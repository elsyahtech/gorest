//go:build integration

package filter_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	gorest "github.com/elsyahtech/gorest"
	jwtfilter "github.com/elsyahtech/gorest/filter"
	"github.com/elsyahtech/gorest/server"
	"github.com/golang-jwt/jwt/v5"
)

func TestCreateTokenSetsSignedAccessAndRefreshCookies(t *testing.T) {
	const signingKey = "integration-test-signing-key-with-at-least-32-bytes"
	config := jwtfilter.ConfigDefault
	config.SignatureKey = stringPointer(signingKey)
	config.AccessTokenDuration = 2 * time.Minute
	config.RefreshTokenDuration = 2 * time.Hour
	config.CookieSecure = true

	app := gorest.New(gorest.Config{AppName: "filter-integration"})
	app.Use(config)
	t.Cleanup(func() {
		if _, _, err := app.Close(); err != nil {
			t.Errorf("close GOREST app: %v", err)
		}
	})

	httpServer, _, err := server.New(nil)
	if err != nil {
		t.Fatalf("initialize HTTP server: %v", err)
	}
	httpServer.Post("/tokens", func(ctx *server.Context) error {
		message, status, err := app.Filter().CreateToken(*ctx, "user-123")
		if err != nil {
			return ctx.Status(status).SendString(message)
		}
		return ctx.Status(status).SendString(message)
	})

	response, err := httpServer.SRV.Test(httptest.NewRequest(http.MethodPost, "/tokens", nil))
	if err != nil {
		t.Fatalf("send token request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("token request status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	cookies := response.Cookies()
	if len(cookies) != 2 {
		t.Fatalf("response cookies = %d, want 2", len(cookies))
	}

	want := map[string]time.Duration{
		"access":  config.AccessTokenDuration,
		"refresh": config.RefreshTokenDuration,
	}
	for _, cookie := range cookies {
		duration, ok := want[cookie.Name]
		if !ok {
			t.Errorf("unexpected token cookie %q", cookie.Name)
			continue
		}
		delete(want, cookie.Name)

		if cookie.Value == "" {
			t.Errorf("%s cookie value is empty", cookie.Name)
			continue
		}
		if cookie.MaxAge < int(duration/time.Second)-1 || cookie.MaxAge > int(duration/time.Second)+1 {
			t.Errorf("%s cookie MaxAge = %d, want approximately %d seconds",
				cookie.Name, cookie.MaxAge, int(duration/time.Second))
		}
		if !cookie.Secure {
			t.Errorf("%s cookie Secure = false, want true", cookie.Name)
		}
		if !cookie.HttpOnly {
			t.Errorf("%s cookie HttpOnly = false, want true", cookie.Name)
		}

		parsed, err := jwt.Parse(cookie.Value, func(token *jwt.Token) (any, error) {
			if token.Method.Alg() != jwtfilter.DefaultSigningMethod {
				t.Errorf("token algorithm = %q, want %q", token.Method.Alg(), jwtfilter.DefaultSigningMethod)
			}
			return []byte(signingKey), nil
		})
		if err != nil {
			t.Fatalf("verify %s token signature: %v", cookie.Name, err)
		}
		claims, ok := parsed.Claims.(jwt.MapClaims)
		if !ok || !parsed.Valid {
			t.Fatalf("%s token claims are invalid: %#v", cookie.Name, parsed.Claims)
		}
		if claims["token_type"] != cookie.Name {
			t.Errorf("%s token_type = %v", cookie.Name, claims["token_type"])
		}
		if claims["token_id"] != "user-123" {
			t.Errorf("%s token_id = %v, want user-123", cookie.Name, claims["token_id"])
		}
		if claims["issuer"] != "filter-integration" {
			t.Errorf("%s issuer = %v, want filter-integration", cookie.Name, claims["issuer"])
		}
		issuedAt, issuedOK := claims["issue_at"].(float64)
		expiresAt, expiresOK := claims["expires_at"].(float64)
		if !issuedOK || !expiresOK || expiresAt-issuedAt != duration.Seconds() {
			t.Errorf("%s custom time claims issue_at=%v expires_at=%v, want duration %s",
				cookie.Name, claims["issue_at"], claims["expires_at"], duration)
		}
	}
	if len(want) != 0 {
		t.Errorf("missing token cookies: %v", want)
	}
}

func TestAuthenticationRejectsInvalidAccessTokensOverHTTP(t *testing.T) {
	tests := []struct {
		name       string
		signingKey string
		issuer     string
		tokenType  string
		expiresIn  time.Duration
	}{
		{
			name:       "invalid signature",
			signingKey: "different-signing-key-with-at-least-32-bytes",
			issuer:     "filter-auth-integration",
			tokenType:  "access",
			expiresIn:  time.Hour,
		},
		{
			name:      "expired access token",
			issuer:    "filter-auth-integration",
			tokenType: "access",
			expiresIn: -time.Minute,
		},
		{
			name:      "issuer mismatch",
			issuer:    "unexpected-issuer",
			tokenType: "access",
			expiresIn: time.Hour,
		},
		{
			name:      "token type mismatch",
			issuer:    "filter-auth-integration",
			tokenType: "refresh",
			expiresIn: time.Hour,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			suite := newAuthenticationHTTPSuite(t)
			signingKey := test.signingKey
			if signingKey == "" {
				signingKey = suite.signingKey
			}
			token := createSignedTestToken(t, signingKey, test.issuer, test.tokenType, test.expiresIn)

			response := suite.getProtected(t, &http.Cookie{Name: "access", Value: token})
			defer response.Body.Close()
			if response.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusUnauthorized)
			}
		})
	}
}

func TestAuthenticationRejectsExpiredRefreshTokenOverHTTP(t *testing.T) {
	suite := newAuthenticationHTTPSuite(t)
	expiredRefresh := createSignedTestToken(
		t,
		suite.signingKey,
		"filter-auth-integration",
		"refresh",
		-time.Minute,
	)

	response := suite.getProtected(t, &http.Cookie{Name: "refresh", Value: expiredRefresh})
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusUnauthorized)
	}
}

func TestAuthenticationRenewsAccessAndContinuesRequestWithValidRefreshOverHTTP(t *testing.T) {
	suite := newAuthenticationHTTPSuite(t)
	expiredAccess := createSignedTestToken(
		t,
		suite.signingKey,
		"filter-auth-integration",
		"access",
		-time.Minute,
	)
	validRefresh := createSignedTestToken(
		t,
		suite.signingKey,
		"filter-auth-integration",
		"refresh",
		time.Hour,
	)

	response := suite.getProtected(
		t,
		&http.Cookie{Name: "access", Value: expiredAccess},
		&http.Cookie{Name: "refresh", Value: validRefresh},
	)
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read protected response: %v", err)
	}
	if got := string(body); got != "refresh" {
		t.Fatalf("handler received token type %q, want refresh", got)
	}

	var renewedAccess *http.Cookie
	for _, cookie := range response.Cookies() {
		if cookie.Name == "access" && cookie.Value != "" {
			renewedAccess = cookie
			break
		}
	}
	if renewedAccess == nil {
		t.Fatal("response did not set a renewed access cookie")
	}

	parsed, err := jwt.Parse(renewedAccess.Value, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != jwtfilter.DefaultSigningMethod {
			return nil, fmt.Errorf("unexpected signing algorithm %q", token.Method.Alg())
		}
		return []byte(suite.signingKey), nil
	})
	if err != nil {
		t.Fatalf("verify renewed access token: %v", err)
	}
	claims, ok := parsed.Claims.(jwt.MapClaims)
	if !ok || !parsed.Valid {
		t.Fatalf("renewed access token claims are invalid: %#v", parsed.Claims)
	}
	if claims["token_type"] != "access" {
		t.Errorf("renewed token_type = %v, want access", claims["token_type"])
	}
	if claims["token_id"] != "user-123" {
		t.Errorf("renewed token_id = %v, want user-123", claims["token_id"])
	}
	if claims["issuer"] != "filter-auth-integration" {
		t.Errorf("renewed issuer = %v, want filter-auth-integration", claims["issuer"])
	}
	issuedAt, issuedOK := claims["issue_at"].(float64)
	expiresAt, expiresOK := claims["expires_at"].(float64)
	if !issuedOK || !expiresOK || expiresAt-issuedAt != (2*time.Minute).Seconds() {
		t.Errorf("renewed access lifetime issue_at=%v expires_at=%v, want %s",
			claims["issue_at"], claims["expires_at"], 2*time.Minute)
	}
}

type authenticationHTTPSuite struct {
	signingKey string
	httpServer *server.Server
}

func newAuthenticationHTTPSuite(t *testing.T) *authenticationHTTPSuite {
	t.Helper()

	const signingKey = "authentication-integration-key-with-at-least-32-bytes"
	config := jwtfilter.ConfigDefault
	config.SignatureKey = stringPointer(signingKey)
	config.AccessTokenDuration = 2 * time.Minute
	config.RefreshTokenDuration = 2 * time.Hour
	config.Issuer = "filter-auth-integration"

	app := gorest.New(gorest.Config{AppName: config.Issuer})
	app.Use(config)
	t.Cleanup(func() {
		if _, _, err := app.Close(); err != nil {
			t.Errorf("close GOREST app: %v", err)
		}
	})

	httpServer, _, err := server.New(nil)
	if err != nil {
		t.Fatalf("initialize HTTP server: %v", err)
	}
	httpServer.Add(
		[]string{http.MethodGet},
		"/protected",
		app.Filter().Authentication(),
		func(ctx *server.Context) error {
			detail, ok := ctx.Locals("tokenJWT").(jwtfilter.TokenDetail)
			if !ok {
				return ctx.SendStatus(http.StatusInternalServerError)
			}
			return ctx.SendString(detail.TokenType)
		},
	)

	return &authenticationHTTPSuite{signingKey: signingKey, httpServer: httpServer}
}

func (suite *authenticationHTTPSuite) getProtected(t *testing.T, cookies ...*http.Cookie) *http.Response {
	t.Helper()

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	for _, cookie := range cookies {
		request.AddCookie(cookie)
	}
	response, err := suite.httpServer.SRV.Test(request)
	if err != nil {
		t.Fatalf("send protected HTTP request: %v", err)
	}
	return response
}

func createSignedTestToken(t *testing.T, signingKey, issuer, tokenType string, expiresIn time.Duration) string {
	t.Helper()

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"issue_at":   now.Unix(),
		"token_id":   "user-123",
		"issuer":     issuer,
		"token_type": tokenType,
		"expires_at": now.Add(expiresIn).Unix(),
	})
	signed, err := token.SignedString([]byte(signingKey))
	if err != nil {
		t.Fatalf("sign %s test token: %v", tokenType, err)
	}
	return signed
}

func stringPointer(value string) *string {
	return &value
}
