package filter

import (
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
	if got := Run(); got != ConfigDefault {
		t.Errorf("Run() = %#v, want %#v", got, ConfigDefault)
	}
}
