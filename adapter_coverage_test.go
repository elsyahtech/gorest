package gorest

import (
	"testing"

	"github.com/elsyahtech/gorest/filter"
	gorestredis "github.com/elsyahtech/gorest/redis"
)

func TestGetRedisConfigHandlesNilAndConfiguredApplications(t *testing.T) {
	if got := (*App)(nil).GetRedisConfig(); got != (gorestredis.Config{}) {
		t.Fatalf("nil app Redis config = %#v, want zero value", got)
	}

	app := &App{}
	if got := app.GetRedisConfig(); got != (gorestredis.Config{}) {
		t.Fatalf("missing Redis config = %#v, want zero value", got)
	}

	config := gorestredis.Config{Host: "cache.internal", Port: "6380"}
	app.config.redis = &config
	if got := app.GetRedisConfig(); got != config {
		t.Fatalf("configured Redis config = %#v, want %#v", got, config)
	}
}

func TestFilterHandlesNilAndReturnsConfiguredFilter(t *testing.T) {
	if got := (*App)(nil).Filter(); got != nil {
		t.Fatalf("nil app Filter() = %v, want nil", got)
	}

	expected := &filter.Filter{}
	app := &App{filter: expected}
	if got := app.Filter(); got != expected {
		t.Fatalf("Filter() = %p, want %p", got, expected)
	}
}
