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

func TestAuthenticationRejectsMissingTokensWhenLoggingDisabled(t *testing.T) {
	config := ConfigDefault
	config.SignatureKey = authenticationStringPointer("authentication-test-key")
	config.LoggerConfig.DisableLogging = true
	tokenFilter := newTestFilter(t, config)
	app := newAuthenticationTestApp(tokenFilter)

	response, err := app.Test(httptest.NewRequest(http.MethodGet, "/protected", nil))
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusUnauthorized)
	}
}

func TestAuthenticationIPRestrictions(t *testing.T) {
	tests := []struct {
		name       string
		allowedIPs []string
		blockedIPs []string
		wantStatus int
	}{
		{
			name:       "allowlist matching peer",
			allowedIPs: []string{"0.0.0.0/0", "::/0"},
			wantStatus: http.StatusOK,
		},
		{
			name:       "allowlist excludes peer",
			allowedIPs: []string{"192.0.2.0/24"},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "blocklist takes precedence",
			allowedIPs: []string{"0.0.0.0/0", "::/0"},
			blockedIPs: []string{"0.0.0.0/0", "::/0"},
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "empty lists allow all peers",
			wantStatus: http.StatusOK,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := Config{
				SignatureKey: authenticationStringPointer("authentication-test-key"),
				AllowedIPs:   test.allowedIPs,
				BlockedIPs:   test.blockedIPs,
			}
			tokenFilter := newTestFilter(t, config)
			accessToken := createAuthenticationTestToken(t, tokenFilter, "access", time.Minute)
			app := newAuthenticationTestApp(tokenFilter)

			request := httptest.NewRequest(http.MethodGet, "/protected", nil)
			request.Header.Set("Authorization", "Bearer "+accessToken)
			response, err := app.Test(request)
			if err != nil {
				t.Fatalf("send request: %v", err)
			}
			defer response.Body.Close()

			if response.StatusCode != test.wantStatus {
				t.Fatalf("status = %d, want %d", response.StatusCode, test.wantStatus)
			}
		})
	}
}

func TestAuthenticationIPRestrictionsIgnoreForwardedHeaders(t *testing.T) {
	tokenFilter := newTestFilter(t, Config{
		SignatureKey: authenticationStringPointer("authentication-test-key"),
		AllowedIPs:   []string{"192.0.2.42"},
	})
	app := newAuthenticationTestApp(tokenFilter)
	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("X-Forwarded-For", "192.0.2.42")
	request.Header.Set("Authorization", "Bearer "+createAuthenticationTestToken(t, tokenFilter, "access", time.Minute))

	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("status = %d, want %d; forwarded header must not override the connection peer IP",
			response.StatusCode, http.StatusForbidden)
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

func TestAuthenticationRejectsRefreshTokenInBearerHeader(t *testing.T) {
	tokenFilter := newTestFilter(t, Config{SignatureKey: authenticationStringPointer("authentication-test-key")})
	refreshToken := createAuthenticationTestToken(t, tokenFilter, "refresh", time.Hour)
	app := newAuthenticationTestApp(tokenFilter)

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "Bearer "+refreshToken)
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusUnauthorized)
	}
}

func TestAuthenticationAcceptsBearerAccessToken(t *testing.T) {
	tokenFilter := newTestFilter(t, Config{SignatureKey: authenticationStringPointer("authentication-test-key")})
	accessToken := createAuthenticationTestToken(t, tokenFilter, "access", time.Minute)
	app := newAuthenticationTestApp(tokenFilter)

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.Header.Set("Authorization", "bEaReR "+accessToken)
	response, err := app.Test(request)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
}

func TestGetClientIPUsesForwardedHeaderAndFallsBackForInvalidHeader(t *testing.T) {
	app := fiber.New()
	app.Get("/ip", func(ctx fiber.Ctx) error {
		return ctx.SendString(GetClientIP(server.Context{Ctx: ctx}))
	})
	app.Get("/direct-ip", func(ctx fiber.Ctx) error {
		return ctx.SendString(ctx.IP())
	})

	for _, test := range []struct {
		name        string
		forwarded   string
		want        string
		wantForward bool
	}{
		{
			name:        "first address in forwarding chain",
			forwarded:   " 203.0.113.42 , 198.51.100.10",
			want:        "203.0.113.42",
			wantForward: true,
		},
		{
			name:      "invalid forwarding address falls back to connection IP",
			forwarded: "not-an-ip, 198.51.100.10",
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodGet, "/ip", nil)
			request.Header.Set("X-Forwarded-For", test.forwarded)
			response, err := app.Test(request)
			if err != nil {
				t.Fatalf("send request: %v", err)
			}
			defer response.Body.Close()

			body, err := io.ReadAll(response.Body)
			if err != nil {
				t.Fatalf("read client IP: %v", err)
			}
			if test.wantForward {
				if got := string(body); got != test.want {
					t.Fatalf("GetClientIP() = %q, want first forwarded IP %q", got, test.want)
				}
				return
			}

			directRequest := httptest.NewRequest(http.MethodGet, "/direct-ip", nil)
			directResponse, err := app.Test(directRequest)
			if err != nil {
				t.Fatalf("send direct IP request: %v", err)
			}
			defer directResponse.Body.Close()
			directIP, err := io.ReadAll(directResponse.Body)
			if err != nil {
				t.Fatalf("read direct connection IP: %v", err)
			}
			if got := string(body); got != string(directIP) {
				t.Fatalf("GetClientIP() = %q, want connection IP %q", got, directIP)
			}
		})
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
	return token.Token
}

func authenticationStringPointer(value string) *string {
	return &value
}
