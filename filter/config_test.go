package filter

import (
	"reflect"
	"testing"
	"time"
)

func TestRunAppliesDefaultsWithoutDiscardingSignatureKey(t *testing.T) {
	key := "test-signature-key"
	config := Config{
		SignatureKey: &key,
	}

	got := Run(config)
	if got.SignatureKey != &key {
		t.Fatal("Run() did not preserve the configured signature key")
	}
	if got.SigningMethod != DefaultSigningMethod {
		t.Errorf("SigningMethod = %q, want %q", got.SigningMethod, DefaultSigningMethod)
	}
	if got.AccessTokenDuration != 15*time.Minute {
		t.Errorf("AccessTokenDuration = %s, want %s", got.AccessTokenDuration, 15*time.Minute)
	}
	if got.RefreshTokenDuration != 30*24*time.Hour {
		t.Errorf("RefreshTokenDuration = %s, want %s", got.RefreshTokenDuration, 30*24*time.Hour)
	}
	if got.CookiePath != DefaultCookiePath {
		t.Errorf("CookiePath = %q, want %q", got.CookiePath, DefaultCookiePath)
	}
}

func TestRunWithoutConfigReturnsDefaults(t *testing.T) {
	if got := Run(); !reflect.DeepEqual(got, ConfigDefault) {
		t.Errorf("Run() = %#v, want %#v", got, ConfigDefault)
	}
}

func TestRunDefaultsAuthenticationLogEventsAndPreservesEmptySelection(t *testing.T) {
	defaults := Run()
	if len(defaults.LoggerConfig.EnabledEvents) != 1 ||
		defaults.LoggerConfig.EnabledEvents[0] != AuthenticationEventMissingCredentials {
		t.Fatalf("default authentication events = %v, want [%s]",
			defaults.LoggerConfig.EnabledEvents, AuthenticationEventMissingCredentials)
	}

	emptySelection := Run(Config{
		LoggerConfig: LoggerConfig{EnabledEvents: []AuthenticationLogEvent{}},
	})
	if emptySelection.LoggerConfig.EnabledEvents == nil || len(emptySelection.LoggerConfig.EnabledEvents) != 0 {
		t.Errorf("explicit empty authentication event selection = %v, want non-nil empty slice",
			emptySelection.LoggerConfig.EnabledEvents)
	}

	partialConfig := Run(Config{})
	if len(partialConfig.LoggerConfig.EnabledEvents) != 1 ||
		partialConfig.LoggerConfig.EnabledEvents[0] != AuthenticationEventMissingCredentials {
		t.Errorf("partial config authentication events = %v, want default [%s]",
			partialConfig.LoggerConfig.EnabledEvents, AuthenticationEventMissingCredentials)
	}
}
