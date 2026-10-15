package gorest

import (
	"errors"
	"testing"
	"time"
)

func TestStartServerAfterDatabaseTasksStopsOnMigrationError(t *testing.T) {
	migrationErr := errors.New("migration failed")
	seederCalled := false
	serverStarted := false

	startServer, err := prepareStartup(
		func() error { return migrationErr },
		func() error {
			seederCalled = true
			return nil
		},
		func() { serverStarted = true },
	)

	if !errors.Is(err, migrationErr) {
		t.Fatalf("startup error = %v, want migration error", err)
	}

	if startServer != nil {
		t.Fatal("server start callback returned after migration failed")
	}

	if seederCalled {
		t.Fatal("seeder ran after migration failed")
	}

	if serverStarted {
		t.Fatal("server started after migration failed")
	}
}

func TestStartServerAfterDatabaseTasksStopsOnSeederError(t *testing.T) {
	seederErr := errors.New("seeder failed")
	serverStarted := make(chan struct{}, 1)

	startServer, err := prepareStartup(
		func() error { return nil },
		func() error { return seederErr },
		func() { serverStarted <- struct{}{} },
	)

	if !errors.Is(err, seederErr) {
		t.Fatalf("startup error = %v, want seeder error", err)
	}

	if startServer != nil {
		t.Fatal("server start callback returned after seeder failed")
	}

	select {
	case <-serverStarted:
		t.Fatal("server started after seeder failed")
	default:
	}
}

func TestStartServerAfterDatabaseTasksStartsServerOnSuccess(t *testing.T) {
	serverStarted := make(chan struct{}, 1)

	startServer, err := prepareStartup(
		func() error { return nil },
		func() error { return nil },
		func() { serverStarted <- struct{}{} },
	)

	if err != nil {
		t.Fatalf("startup returned unexpected error: %v", err)
	}

	if startServer == nil {
		t.Fatal("server start callback was not returned")
	}

	startServer()
	select {
	case <-serverStarted:
	case <-time.After(time.Second):
		t.Fatal("server did not start after successful database tasks")
	}
}

func TestRunDatabaseStartupTasksStopsSeederAfterMigrationFailure(t *testing.T) {
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
