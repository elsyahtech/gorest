package redis

import "testing"

func TestConfigDefaultFillsMissingValuesAndPreservesOverrides(t *testing.T) {
	defaults := configDefault()
	if defaults.Host != ConfigDefault.Host || defaults.Port != ConfigDefault.Port ||
		defaults.Timeout != ConfigDefault.Timeout || defaults.PoolTimeout != ConfigDefault.PoolTimeout {
		t.Fatalf("configDefault() = %#v, want configured defaults", defaults)
	}

	got := configDefault(Config{
		Host:     "redis.internal",
		Port:     "6380",
		Password: "test-password",
		Database: 4,
	})
	if got.Host != "redis.internal" || got.Port != "6380" ||
		got.Password != "test-password" || got.Database != 4 {
		t.Fatalf("configDefault() discarded caller overrides: %#v", got)
	}
}
