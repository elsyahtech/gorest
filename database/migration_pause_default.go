//go:build !integration

package database

func migrationIntegrationPause(string) error { return nil }
