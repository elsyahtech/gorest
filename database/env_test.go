package database

import "testing"

func TestConfigFromEnvUsesPasswordEnvironmentWithoutLogging(t *testing.T) {
	t.Setenv("GOREST_DB_HOST", "srv.elsyah.com")
	t.Setenv("GOREST_DB_PORT", "5432")
	t.Setenv("GOREST_DB_NAME", "gorest")
	t.Setenv("GOREST_DB_USERNAME", "gorest")
	t.Setenv("GOREST_DB_PASSWORD", "secret")
	t.Setenv("GOREST_DB_AUTH_SOURCE", "gorest")
	t.Setenv("GOREST_DB_TLS", "verify-full")
	config := ConfigFromEnv("GOREST_DB")
	if config.Host != "srv.elsyah.com" || config.Port != "5432" || config.Name != "gorest" {
		t.Fatalf("ConfigFromEnv() target = %s:%s/%s", config.Host, config.Port, config.Name)
	}
	if config.Password != "secret" || config.AuthSource != "gorest" || config.TLS != "verify-full" {
		t.Fatal("ConfigFromEnv() did not load credential or TLS settings")
	}
}
