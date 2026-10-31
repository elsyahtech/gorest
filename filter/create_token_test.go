package filter

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/elsyahtech/gorest/server"
	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
)

func newTestFilter(t *testing.T, config Config) *Filter {
	t.Helper()

	config = Run(config)
	tokenTemplate, message, err := New(&config)
	if err != nil {
		t.Fatalf("initialize JWT: %s: %v", message, err)
	}

	return &Filter{JWT: tokenTemplate, Config: &config}
}

func TestCreateTokenSetsCookiesMatchingTokenLifetimes(t *testing.T) {
	key := "test-signing-key"
	config := Config{
		SignatureKey:         &key,
		AccessTokenDuration:  5 * time.Minute,
		RefreshTokenDuration: 24 * time.Hour,
		CookieMaxAge:         60,
	}
	tokenFilter := newTestFilter(t, config)
	app := fiber.New()
	app.Post("/token", func(ctx fiber.Ctx) error {
		message, status, err := tokenFilter.CreateToken(server.Context{Ctx: ctx}, "user-123")
		if err != nil {
			return ctx.Status(status).SendString(message)
		}
		return ctx.Status(status).SendString(message)
	})

	response, err := app.Test(httptest.NewRequest(http.MethodPost, "/token", nil))
	if err != nil {
		t.Fatalf("send token request: %v", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}

	cookies := response.Cookies()
	if len(cookies) != 2 {
		t.Fatalf("response cookies = %d, want 2", len(cookies))
	}
	wantDurations := map[string]time.Duration{
		"access":  config.AccessTokenDuration,
		"refresh": config.RefreshTokenDuration,
	}
	for _, cookie := range cookies {
		duration, ok := wantDurations[cookie.Name]
		if !ok {
			t.Errorf("unexpected cookie %q", cookie.Name)
			continue
		}
		if cookie.MaxAge < int(duration/time.Second)-1 || cookie.MaxAge > int(duration/time.Second)+1 {
			t.Errorf("%s cookie MaxAge = %d, want approximately %d", cookie.Name, cookie.MaxAge, int(duration/time.Second))
		}
		if cookie.Domain != "" {
			t.Errorf("%s cookie Domain = %q, want host-only cookie", cookie.Name, cookie.Domain)
		}
		parsed, _, err := new(jwt.Parser).ParseUnverified(cookie.Value, jwt.MapClaims{})
		if err != nil {
			t.Fatalf("parse %s token claims: %v", cookie.Name, err)
		}
		claims, ok := parsed.Claims.(jwt.MapClaims)
		if !ok {
			t.Fatalf("%s claims have type %T, want jwt.MapClaims", cookie.Name, parsed.Claims)
		}
		if claims["token_type"] != cookie.Name {
			t.Errorf("%s token_type = %v", cookie.Name, claims["token_type"])
		}
		issuedAt, issuedOK := claims["issue_at"].(float64)
		expiresAt, expiresOK := claims["expires_at"].(float64)
		if !issuedOK || !expiresOK || expiresAt-issuedAt != duration.Seconds() {
			t.Errorf("%s custom time claims issue_at=%v expires_at=%v, want delta %v seconds",
				cookie.Name, claims["issue_at"], claims["expires_at"], duration.Seconds())
		}
	}
}

func TestGenerateTokenConcurrentCallsUseIndependentClaims(t *testing.T) {
	key := "test-signing-key"
	tokenFilter := newTestFilter(t, Config{SignatureKey: &key})
	const count = 32

	results := make(chan error, count)
	var wait sync.WaitGroup
	for index := range count {
		wait.Add(1)
		go func() {
			defer wait.Done()

			tokenID := fmt.Sprintf("user-%d", index)
			token, _, err := tokenFilter.generateToken(
				tokenFilter.JWT.Method,
				tokenID,
				"access",
				tokenFilter.Config.AccessTokenDuration,
				[]byte(key),
			)
			if err != nil {
				results <- fmt.Errorf("generate token for %s: %w", tokenID, err)
				return
			}
			parsed, _, err := new(jwt.Parser).ParseUnverified(token, jwt.MapClaims{})
			if err != nil {
				results <- fmt.Errorf("parse token for %s: %w", tokenID, err)
				return
			}
			claims, ok := parsed.Claims.(jwt.MapClaims)
			if !ok || claims["token_id"] != tokenID {
				results <- fmt.Errorf("token for %s contains claims %#v", tokenID, parsed.Claims)
			}
		}()
	}
	wait.Wait()
	close(results)

	for err := range results {
		if err != nil {
			t.Error(err)
		}
	}
}

func TestNewRejectsTokenDurationsBelowClaimPrecision(t *testing.T) {
	key := "test-signing-key"
	config := Run(Config{
		SignatureKey:         &key,
		AccessTokenDuration:  time.Second,
		RefreshTokenDuration: time.Second,
	})
	config.AccessTokenDuration = time.Millisecond

	if _, _, err := New(&config); err == nil {
		t.Fatal("New() error = nil, want sub-second token duration rejection")
	}
}
