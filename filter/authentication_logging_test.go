package filter

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestAuthenticationLogsConfiguredSuccessEvents(t *testing.T) {
	tests := []struct {
		name       string
		event      AuthenticationLogEvent
		useRefresh bool
		wantEvent  AuthenticationLogEvent
	}{
		{
			name:      "access token authentication",
			event:     AuthenticationEventAuthenticationSucceeded,
			wantEvent: AuthenticationEventAuthenticationSucceeded,
		},
		{
			name:       "refresh token authentication",
			event:      AuthenticationEventRefreshSucceeded,
			useRefresh: true,
			wantEvent:  AuthenticationEventRefreshSucceeded,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := ConfigDefault
			config.SignatureKey = authenticationStringPointer("authentication-test-key")
			config.LoggerConfig.EnabledEvents = []AuthenticationLogEvent{test.event}
			tokenFilter := newTestFilter(t, config)
			core, recorded := observer.New(zap.DebugLevel)
			tokenFilter.Logger = zap.New(core)

			request := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if test.useRefresh {
				request.AddCookie(&http.Cookie{
					Name:  "refresh",
					Value: createAuthenticationTestToken(t, tokenFilter, "refresh", time.Hour),
				})
			} else {
				request.AddCookie(&http.Cookie{
					Name:  "access",
					Value: createAuthenticationTestToken(t, tokenFilter, "access", time.Minute),
				})
			}

			response, err := newAuthenticationTestApp(tokenFilter).Test(request)
			if err != nil {
				t.Fatalf("send request: %v", err)
			}
			defer response.Body.Close()

			if response.StatusCode != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
			}

			entries := recorded.All()
			if len(entries) != 1 {
				t.Fatalf("logged entries = %d, want 1", len(entries))
			}
			if got := entries[0].ContextMap()["authentication_event"]; got != string(test.wantEvent) {
				t.Errorf("authentication_event = %v, want %q", got, test.wantEvent)
			}
			if entries[0].Level != zap.InfoLevel {
				t.Errorf("log level = %s, want %s", entries[0].Level, zap.InfoLevel)
			}
		})
	}
}

func TestAuthenticationLogsConfiguredFailureEvents(t *testing.T) {
	tests := []struct {
		name       string
		event      AuthenticationLogEvent
		tokenName  string
		tokenValue func(*testing.T, *Filter) string
	}{
		{
			name:  "missing credentials",
			event: AuthenticationEventMissingCredentials,
		},
		{
			name:       "invalid access token",
			event:      AuthenticationEventAccessTokenInvalid,
			tokenName:  "access",
			tokenValue: func(*testing.T, *Filter) string { return "invalid-token" },
		},
		{
			name:      "expired access token",
			event:     AuthenticationEventAccessTokenExpired,
			tokenName: "access",
			tokenValue: func(t *testing.T, tokenFilter *Filter) string {
				return createAuthenticationTestToken(t, tokenFilter, "access", -time.Second)
			},
		},
		{
			name:       "invalid refresh token",
			event:      AuthenticationEventRefreshTokenInvalid,
			tokenName:  "refresh",
			tokenValue: func(*testing.T, *Filter) string { return "invalid-token" },
		},
		{
			name:      "expired refresh token",
			event:     AuthenticationEventRefreshTokenExpired,
			tokenName: "refresh",
			tokenValue: func(t *testing.T, tokenFilter *Filter) string {
				return createAuthenticationTestToken(t, tokenFilter, "refresh", -time.Second)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			config := ConfigDefault
			config.SignatureKey = authenticationStringPointer("authentication-test-key")
			config.LoggerConfig.EnabledEvents = []AuthenticationLogEvent{test.event}
			tokenFilter := newTestFilter(t, config)
			core, recorded := observer.New(zap.WarnLevel)
			tokenFilter.Logger = zap.New(core)

			request := httptest.NewRequest(http.MethodGet, "/protected", nil)
			if test.tokenName != "" {
				request.AddCookie(&http.Cookie{
					Name:  test.tokenName,
					Value: test.tokenValue(t, tokenFilter),
				})
			}

			response, err := newAuthenticationTestApp(tokenFilter).Test(request)
			if err != nil {
				t.Fatalf("send request: %v", err)
			}
			defer response.Body.Close()

			if response.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusUnauthorized)
			}

			entries := recorded.All()
			if len(entries) != 1 {
				t.Fatalf("logged entries = %d, want 1", len(entries))
			}
			if got := entries[0].ContextMap()["authentication_event"]; got != string(test.event) {
				t.Errorf("authentication_event = %v, want %q", got, test.event)
			}
		})
	}
}

func TestAuthenticationEventAllowlistAndGlobalDisable(t *testing.T) {
	for _, test := range []struct {
		name           string
		disableLogging bool
		enabledEvents  []AuthenticationLogEvent
		wantEntries    int
	}{
		{
			name:          "event not selected",
			enabledEvents: []AuthenticationLogEvent{AuthenticationEventAccessTokenInvalid},
		},
		{
			name:           "globally disabled",
			disableLogging: true,
			enabledEvents:  []AuthenticationLogEvent{AuthenticationEventMissingCredentials},
		},
		{
			name:          "selected event",
			enabledEvents: []AuthenticationLogEvent{AuthenticationEventMissingCredentials},
			wantEntries:   1,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			config := ConfigDefault
			config.SignatureKey = authenticationStringPointer("authentication-test-key")
			config.LoggerConfig.DisableLogging = test.disableLogging
			config.LoggerConfig.EnabledEvents = test.enabledEvents
			tokenFilter := newTestFilter(t, config)
			core, recorded := observer.New(zap.WarnLevel)
			tokenFilter.Logger = zap.New(core)

			response, err := newAuthenticationTestApp(tokenFilter).Test(
				httptest.NewRequest(http.MethodGet, "/protected", nil),
			)
			if err != nil {
				t.Fatalf("send request: %v", err)
			}
			defer response.Body.Close()

			if response.StatusCode != http.StatusUnauthorized {
				t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusUnauthorized)
			}
			if got := recorded.Len(); got != test.wantEntries {
				t.Errorf("logged entries = %d, want %d", got, test.wantEntries)
			}
		})
	}
}

func TestAuthenticationDoesNotLogRecoveredAccessTokenFailure(t *testing.T) {
	config := ConfigDefault
	config.SignatureKey = authenticationStringPointer("authentication-test-key")
	config.LoggerConfig.EnabledEvents = []AuthenticationLogEvent{AuthenticationEventAccessTokenExpired}
	tokenFilter := newTestFilter(t, config)
	expiredAccess := createAuthenticationTestToken(t, tokenFilter, "access", -time.Second)
	refresh := createAuthenticationTestToken(t, tokenFilter, "refresh", time.Hour)
	core, recorded := observer.New(zap.WarnLevel)
	tokenFilter.Logger = zap.New(core)

	request := httptest.NewRequest(http.MethodGet, "/protected", nil)
	request.AddCookie(&http.Cookie{Name: "access", Value: expiredAccess})
	request.AddCookie(&http.Cookie{Name: "refresh", Value: refresh})
	response, err := newAuthenticationTestApp(tokenFilter).Test(request)
	if err != nil {
		t.Fatalf("send request: %v", err)
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", response.StatusCode, http.StatusOK)
	}
	if got := recorded.Len(); got != 0 {
		t.Errorf("logged entries = %d, want 0 for successful refresh fallback", got)
	}
}

func TestAuthenticationLogsRefreshFailureWithoutRawError(t *testing.T) {
	config := ConfigDefault
	config.SignatureKey = authenticationStringPointer("authentication-test-key")
	config.LoggerConfig.EnabledEvents = []AuthenticationLogEvent{AuthenticationEventRefreshFailed}
	tokenFilter := newTestFilter(t, config)
	core, recorded := observer.New(zap.WarnLevel)
	tokenFilter.Logger = zap.New(core)

	tokenFilter.logAuthenticationFailures(
		"",
		"do-not-log-this-token",
		nil,
		refreshFailedError{err: errors.New("private signing detail")},
		http.StatusInternalServerError,
	)

	entries := recorded.All()
	if len(entries) != 1 {
		t.Fatalf("logged entries = %d, want 1", len(entries))
	}
	fields := entries[0].ContextMap()
	if got := fields["authentication_event"]; got != string(AuthenticationEventRefreshFailed) {
		t.Errorf("authentication_event = %v, want %q", got, AuthenticationEventRefreshFailed)
	}
	if _, ok := fields["error"]; ok {
		t.Error("log entry unexpectedly includes raw error details")
	}
	if _, ok := fields["token"]; ok {
		t.Error("log entry unexpectedly includes a token")
	}
}

func TestRefreshFailedErrorPreservesResponseErrorMessage(t *testing.T) {
	original := errors.New("generate new access token: token signing failed")
	failure := refreshFailedError{err: original}

	if got := failure.Error(); got != original.Error() {
		t.Errorf("refresh error = %q, want unchanged response error %q", got, original.Error())
	}
	if !errors.Is(failure, errRefreshFailed) {
		t.Error("refresh failure does not match errRefreshFailed")
	}
	if !errors.Is(failure, original) {
		t.Error("refresh failure does not unwrap the original error")
	}
}
