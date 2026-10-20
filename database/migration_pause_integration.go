//go:build integration

package database

import (
	"fmt"
	"os"
	"time"
)

// migrationIntegrationPause is an integration-build-only synchronization
// point used to coordinate independent migration processes in HTTP E2E tests.
func migrationIntegrationPause(stateKey string) error {
	if os.Getenv("GOREST_IT_MIGRATION_PAUSE_STATE") != stateKey {
		return nil
	}
	marker := os.Getenv("GOREST_IT_MIGRATION_PAUSE_MARKER")
	release := os.Getenv("GOREST_IT_MIGRATION_PAUSE_RELEASE")
	if marker == "" || release == "" {
		return fmt.Errorf("migration pause marker and release paths are required")
	}
	if err := os.WriteFile(marker, []byte(stateKey), 0o600); err != nil {
		return fmt.Errorf("write migration pause marker: %w", err)
	}
	for {
		if _, err := os.Stat(release); err == nil {
			return nil
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("check migration pause release: %w", err)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
