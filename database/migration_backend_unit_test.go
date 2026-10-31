package database

import (
	"context"
	"strings"
	"testing"
)

func TestMigrationLockRejectsMissingInputsAndUnsupportedDriver(t *testing.T) {
	tests := []struct {
		name     string
		ctx      context.Context
		database *Database
		config   *Config
	}{
		{name: "nil context", database: &Database{}, config: &Config{Driver: SQLITE}},
		{name: "nil database", ctx: context.Background(), config: &Config{Driver: SQLITE}},
		{name: "nil config", ctx: context.Background(), database: &Database{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			release, message, err := acquireMigrationLock(test.ctx, test.database, test.config)
			if err == nil || release != nil || message == "" {
				t.Fatalf("acquireMigrationLock() = (release nil: %t, %q, %v), want an initialization error",
					release == nil, message, err)
			}
		})
	}

	_, message, err := acquireMigrationLock(
		context.Background(),
		&Database{},
		&Config{Driver: "unsupported"},
	)
	if err == nil || message == "" || !strings.Contains(err.Error(), "unsupported driver") {
		t.Fatalf("unsupported migration lock = (%q, %v)", message, err)
	}
	if !strings.Contains(migrationLockHeldMessage(), "already holds") {
		t.Fatalf("migrationLockHeldMessage() = %q", migrationLockHeldMessage())
	}
}

func TestNoSQLMigrationLockAndStateLoadReportUnavailableServices(t *testing.T) {
	ctx := context.Background()
	config := &Config{Driver: MONGO}

	if _, message, err := mongoCaseAcquireMigrationLock(ctx, &Database{}, config, "database_startup", "owner"); err == nil || message == "" {
		t.Fatalf("mongoCaseAcquireMigrationLock() = (%q, %v), want unavailable-service error", message, err)
	}
	if _, _, err := scyllaCaseAcquireMigrationLock(ctx, &Database{}, &Config{Driver: SCYLLA}, "database_startup", "owner"); err == nil {
		t.Fatal("scyllaCaseAcquireMigrationLock() accepted an unavailable service")
	}

	if _, err := mongoCaseLoadMigrationStates(ctx, &Database{}, config, map[string]migrationState{}); err == nil {
		t.Fatal("mongoCaseLoadMigrationStates() accepted an unavailable service")
	}
	if _, err := scyllaCaseLoadMigrationStates(ctx, &Database{}, &Config{Driver: SCYLLA}, map[string]migrationState{}); err == nil {
		t.Fatal("scyllaCaseLoadMigrationStates() accepted an unavailable service")
	}
}
