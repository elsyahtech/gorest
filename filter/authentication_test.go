package filter

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/elsyahtech/gorest/server"
	"github.com/gofiber/fiber/v3"
)

func TestAuthenticationAcceptsValidAccessToken(t *testing.T) {
	tokenFilter := newTestFilter(t, Config{SignatureKey: authenticationStringPointer("authentication-test-key")})
	accessToken := createAuthenticationTestToken(t, tokenFilter, "access", time.Minute)
	app := newAuthenticationTestApp(tokenFilter)

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.AddCookie(&http.Cookie{Name: "access", Value: accessToken})
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
}

func TestAuthenticationFallsBackToRefreshToken(t *testing.T) {
	tokenFilter := newTestFilter(t, Config{
		SignatureKey:         authenticationStringPointer("authentication-test-key"),
		AccessTokenDuration:  2 * time.Minute,
		RefreshTokenDuration: 24 * time.Hour,
	})
	expiredAccess := createAuthenticationTestToken(t, tokenFilter, "access", -time.Second)
	refresh := createAuthenticationTestToken(t, tokenFilter, "refresh", time.Hour)
	app := newAuthenticationTestApp(tokenFilter)

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.AddCookie(&http.Cookie{Name: "access", Value: expiredAccess})
	request.AddCookie(&http.Cookie{Name: "refresh", Value: refresh})
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	var renewedAccess *http.Cookie
	for _, cookie := range response.Cookies() {
		if cookie.Name == "access" && cookie.Value != "" {
			renewedAccess = cookie
		}
	}
	if renewedAccess == nil {
		t.Fatal("response did not set a renewed access cookie")
	}
	if renewedAccess.MaxAge < 119 || renewedAccess.MaxAge > 121 {
		t.Errorf("renewed access MaxAge = %d, want approximately 120 seconds", renewedAccess.MaxAge)
	}
}

func TestAuthenticationRejectsExpiredAccessWithoutRefresh(t *testing.T) {
	tokenFilter := newTestFilter(t, Config{SignatureKey: authenticationStringPointer("authentication-test-key")})
	expiredAccess := createAuthenticationTestToken(t, tokenFilter, "access", -time.Second)
	app := newAuthenticationTestApp(tokenFilter)

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.AddCookie(&http.Cookie{Name: "access", Value: expiredAccess})
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusUnauthorized)
	}
}

func TestAuthenticationRejectsRefreshTokenAsAccessToken(t *testing.T) {
	tokenFilter := newTestFilter(t, Config{SignatureKey: authenticationStringPointer("authentication-test-key")})
	refreshToken := createAuthenticationTestToken(t, tokenFilter, "refresh", time.Hour)
	app := newAuthenticationTestApp(tokenFilter)

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.AddCookie(&http.Cookie{Name: "access", Value: refreshToken})
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusUnauthorized)
	}
}

func TestGetClientIPDoesNotTrustForwardedHeaderByDefault(t *testing.T) {
	app := fiber.New()
	app.Get("/ip", func(ctx fiber.Ctx) error {
		return ctx.SendString(GetClientIP(server.Context{Ctx: ctx}))
	})

	request := httptest.NewRequest(http.MethodGet, "/ip", nil)
	request.Header.Set("X-Forwarded-For", "203.0.113.42")
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer response.Body.Close()

	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read client IP: %v", err)
	}
	if string(body) == "203.0.113.42" {
		t.Fatal("GetClientIP trusted X-Forwarded-For without proxy configuration")
	}
}

func newAuthenticationTestApp(tokenFilter *Filter) *fiber.App {
	app := fiber.New()
	app.Use(func(ctx fiber.Ctx) error {
		serverContext := &server.Context{Ctx: ctx}
		return tokenFilter.Authentication()(serverContext)
	})
	app.Get("/protected", func(ctx fiber.Ctx) error {
		detail, ok := ctx.Locals("tokenJWT").(TokenDetail)
		if !ok {
			return ctx.SendStatus(http.StatusInternalServerError)
		}
		if detail.TokenID != "user-123" || detail.TokenType == "" || detail.ExpiresAt == 0 {
			return ctx.SendStatus(http.StatusInternalServerError)
		}
		return ctx.SendStatus(http.StatusOK)
	})
	return app
}

func createAuthenticationTestToken(t *testing.T, tokenFilter *Filter, tokenType string, duration time.Duration) string {
	t.Helper()

	token, _, err := tokenFilter.generateToken(
		tokenFilter.JWT.Method,
		"user-123",
		tokenType,
		duration,
		[]byte(*tokenFilter.Config.SignatureKey),
	)
	if err != nil {
		t.Fatalf("generate %s token: %v", tokenType, err)
	}
	return token
}

func authenticationStringPointer(value string) *string {
	return &value
}
