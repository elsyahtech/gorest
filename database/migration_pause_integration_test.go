//go:build integration

package database

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMigrationIntegrationPause(t *testing.T) {
	originalState := os.Getenv("GOREST_IT_MIGRATION_PAUSE_STATE")
	originalMarker := os.Getenv("GOREST_IT_MIGRATION_PAUSE_MARKER")
	originalRelease := os.Getenv("GOREST_IT_MIGRATION_PAUSE_RELEASE")
	t.Cleanup(func() {
		_ = os.Setenv("GOREST_IT_MIGRATION_PAUSE_STATE", originalState)
		_ = os.Setenv("GOREST_IT_MIGRATION_PAUSE_MARKER", originalMarker)
		_ = os.Setenv("GOREST_IT_MIGRATION_PAUSE_RELEASE", originalRelease)
	})

	if err := migrationIntegrationPause("inactive"); err != nil {
		t.Fatalf("inactive pause returned error: %v", err)
	}

	directory := t.TempDir()
	marker := filepath.Join(directory, "marker")
	release := filepath.Join(directory, "release")
	_ = os.Setenv("GOREST_IT_MIGRATION_PAUSE_STATE", "active")
	_ = os.Setenv("GOREST_IT_MIGRATION_PAUSE_MARKER", marker)
	_ = os.Setenv("GOREST_IT_MIGRATION_PAUSE_RELEASE", release)

	done := make(chan error, 1)
	go func() { done <- migrationIntegrationPause("active") }()

	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, err := os.Stat(marker); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("migration pause marker was not created")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := os.WriteFile(release, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("migrationIntegrationPause() error = %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("migrationIntegrationPause() did not observe release")
	}
}
