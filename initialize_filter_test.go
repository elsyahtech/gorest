package gorest

import (
	"testing"

	"github.com/elsyahtech/gorest/filter"
	logimpl "github.com/elsyahtech/gorest/log"
)

func newFilterInitTestApp(t *testing.T) *App {
	t.Helper()

	logger, _, err := logimpl.New(&logimpl.Config{LogDir: t.TempDir()}, "filter-init-test")
	if err != nil {
		t.Fatalf("initialize test logger: %v", err)
	}
	t.Cleanup(func() {
		if _, _, err := logger.Close(); err != nil {
			t.Errorf("close test logger: %v", err)
		}
	})

	return &App{
		config:  Config{AppName: "filter-init-test"},
		log:     logger,
		modules: make([]string, 0),
	}
}

func TestFilterInitInitializesFilterWithNormalizedConfig(t *testing.T) {
	app := newFilterInitTestApp(t)
	key := "test-signature-key"
	config := &filter.Config{SignatureKey: &key}

	app.filterInit(config)

	if app.filter == nil {
		t.Fatal("filterInit() did not initialize the filter")
	}
	if app.filter.JWT == nil || app.filter.JWT.Method == nil {
		t.Fatal("filterInit() did not initialize the JWT signing method")
	}
	if got := app.filter.JWT.Method.Alg(); got != filter.DefaultSigningMethod {
		t.Errorf("JWT algorithm = %q, want %q", got, filter.DefaultSigningMethod)
	}
	if app.filter.Config == nil {
		t.Fatal("filterInit() did not attach the normalized configuration")
	}
	if app.filter.Config.SignatureKey == nil || *app.filter.Config.SignatureKey != key {
		t.Error("filterInit() did not preserve the configured signature key")
	}
	if app.filter.Config.Issuer != app.config.AppName {
		t.Errorf("filter issuer = %q, want app name %q", app.filter.Config.Issuer, app.config.AppName)
	}
	if config.Issuer != "" {
		t.Errorf("filterInit() mutated the caller's config issuer to %q", config.Issuer)
	}
	if app.config.filter != app.filter.Config {
		t.Error("app config and initialized filter do not reference the same normalized config")
	}
	if len(app.modules) != 1 || app.modules[0] != "Filter" {
		t.Errorf("registered modules = %v, want [Filter]", app.modules)
	}
}

func TestFilterInitLeavesFilterDisabledWithoutSignatureKey(t *testing.T) {
	app := newFilterInitTestApp(t)
	app.filterInit(&filter.Config{})

	if app.filter != nil {
		t.Error("filterInit() initialized a filter without a signature key")
	}
	if app.config.filter != nil {
		t.Error("filterInit() retained filter configuration without a signature key")
	}
	if len(app.modules) != 0 {
		t.Errorf("registered modules = %v, want no modules", app.modules)
	}
}

func TestFilterInitWithNilConfigDisablesFilter(t *testing.T) {
	app := newFilterInitTestApp(t)
	app.filterInit(nil)

	if app.filter != nil {
		t.Error("filterInit(nil) left the filter initialized")
	}
	if app.config.filter != nil {
		t.Error("filterInit(nil) retained filter configuration")
	}
}

func TestFilterInitRejectsUnsupportedSigningMethod(t *testing.T) {
	app := newFilterInitTestApp(t)
	key := "test-signature-key"
	app.filterInit(&filter.Config{
		SignatureKey:  &key,
		SigningMethod: "unsupported",
	})

	if app.filter != nil {
		t.Error("filterInit() initialized a filter with an unsupported signing method")
	}
	if app.config.filter != nil {
		t.Error("filterInit() retained configuration for an unsupported signing method")
	}
	if len(app.modules) != 0 {
		t.Errorf("registered modules = %v, want no modules", app.modules)
	}
}
