package gorest

import (
	"errors"
	"testing"
	"time"

	"github.com/elsyahtech/gorest/database"
)

func TestRunMigrationStartupStopsSeederAfterMigrationFailure(t *testing.T) {
	migrationErr := errors.New("migration failed")
	seederCalled := false

	err := runMigrationStartup(
		func() error { return migrationErr },
		func() error {
			seederCalled = true
			return nil
		},
	)

	if !errors.Is(err, migrationErr) {
		t.Fatalf("startup error = %v, want migration error", err)
	}

	if seederCalled {
		t.Fatal("seeder ran after migration failed")
	}
}

func TestRunMigrationStartupRunsMigrationBeforeSeeder(t *testing.T) {
	var calls []string
	err := runMigrationStartup(
		func() error {
			calls = append(calls, "migration")
			return nil
		},
		func() error {
			calls = append(calls, "seeder")
			return nil
		},
	)
	if err != nil {
		t.Fatalf("runMigrationStartup() error = %v", err)
	}

	if len(calls) != 2 || calls[0] != "migration" || calls[1] != "seeder" {
		t.Fatalf("startup task order = %v, want [migration seeder]", calls)
	}
}

func TestRunMigrationStartupReturnsSeederError(t *testing.T) {
	seederErr := errors.New("seeder failed")
	err := runMigrationStartup(
		func() error { return nil },
		func() error { return seederErr },
	)
	if !errors.Is(err, seederErr) {
		t.Fatalf("startup error = %v, want seeder error", err)
	}
}

func TestLoadTimezoneDoesNotChangeProcessLocalTimezone(t *testing.T) {
	original := time.Local
	app := &App{config: Config{Timezone: "Asia/Jakarta"}}

	app.loadTimezone()

	if time.Local != original {
		t.Fatal("loading an app timezone changed process-global time.Local")
	}
}

func TestRunDatabaseStartupTasksValidatesApplicationAndDatabaseState(t *testing.T) {
	if err := (*App)(nil).RunDatabaseStartupTasks(); err == nil {
		t.Fatal("RunDatabaseStartupTasks() accepted a nil application")
	}

	app := &App{config: Config{}}
	if err := app.RunDatabaseStartupTasks(); err == nil {
		t.Fatal("RunDatabaseStartupTasks() accepted an uninitialized database")
	}

	app = &App{
		config:   Config{database: &database.Config{}},
		database: &database.Database{},
	}
	if err := app.RunDatabaseStartupTasks(); err != nil {
		t.Fatalf("RunDatabaseStartupTasks() with database tasks disabled: %v", err)
	}
}

func TestRunMigrationReportsMissingLoggerAndDatabase(t *testing.T) {
	app := &App{config: Config{database: &database.Config{Migration: true}}}
	if err := app.runMigration(); err == nil {
		t.Fatal("runMigration() accepted a missing logger")
	}

	app.log = newLifecycleTestLogger(t)
	if err := app.runMigration(); err == nil {
		t.Fatal("runMigration() accepted a missing database")
	}
}
