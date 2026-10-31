//go:build integration && integrationcontainers

package httpapi_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/docker/go-connections/nat"
	gorest "github.com/elsyahtech/gorest"
	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/integration_test/internaltest"
	"github.com/testcontainers/testcontainers-go"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const migrationProcessHelperTest = "TestMigrationProcessHelper"

// TestMigrationProcessHelper is invoked as a separate OS process by each
// managed-driver integration test. It intentionally does nothing in the
// parent test process.
func TestMigrationProcessHelper(t *testing.T) {
	if os.Getenv("GOREST_IT_MIGRATION_PROCESS_MODE") != "run" {
		t.Skip("migration process helper")
	}
	driver := os.Getenv("GOREST_IT_MIGRATION_PROCESS_DRIVER")
	directory := os.Getenv("GOREST_IT_MIGRATION_PROCESS_DIRECTORY")
	if driver == "" || directory == "" {
		t.Fatal("migration process helper requires driver and directory")
	}
	t.Chdir(directory)
	app := internaltest.OpenAppWithConfig(t, driver, func(config *database.Config) {
		config.DatabaseDirectory = directory
		config.MigrationDirectory = "migrations"
		config.SeederDirectory = "seeders"
		config.Migration = true
		config.Seeder = false
	})
	if err := app.RunDatabaseStartupTasks(); err != nil {
		t.Fatalf("child migration runner: %v", err)
	}
}

func runMigrationProcessScenarios(t *testing.T, driver string, container testcontainers.Container) {
	t.Helper()
	if driver == database.MONGO {
		runMongoMigrationRollbackScenario(t, driver)
	}
	runIndependentMigrationRunners(t, driver)
	runKilledMigrationRunnerRecovery(t, driver)
	if supportsObservableLongSQL(driver) {
		runInterruptedSQLStatementRecovery(t, driver)
	}
	if container != nil {
		runDatabaseRestartMigrationRecovery(t, driver, container)
		t.Logf("%s migration recovery: concurrent runners, killed process recovery, and database restart recovery passed", driver)
		return
	}
	t.Logf("%s migration recovery: concurrent runners and killed process recovery passed; database restart recovery was skipped for the external endpoint", driver)
}

func runMongoMigrationRollbackScenario(t *testing.T, driver string) {
	t.Helper()
	fixture := internaltest.NewMigrationProcessFixture(t, driver)
	fixture.SetSecondMigrationContent(t, "{")
	app := openMigrationProcessApp(t, driver, fixture)
	defer fixture.Cleanup(t, app)
	if err := app.RunDatabaseStartupTasks(); err == nil {
		t.Fatal("MongoDB migration with malformed JSON unexpectedly succeeded")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, collectionName := range []string{fixture.FirstObject, fixture.SecondObject} {
		collection, _, _, err := app.Database().MongoCollection(collectionName)
		if err != nil {
			continue
		}
		count, err := collection.EstimatedDocumentCount(ctx)
		if err == nil && count != 0 {
			continue
		}
		nameExists, err := collectionNameExists(ctx, app.Database(), collectionName)
		if err != nil {
			t.Fatalf("check rolled back MongoDB collection %s: %v", collectionName, err)
		}
		if nameExists {
			t.Fatalf("MongoDB collection %s remained after migration compensation", collectionName)
		}
	}
	fixture.Recover(t, app)
}

func collectionNameExists(ctx context.Context, db *gorest.DBSession, collectionName string) (bool, error) {
	collection, _, _, err := db.MongoCollection(collectionName)
	if err != nil {
		return false, err
	}
	collections, err := collection.Database().ListCollectionNames(ctx, bson.M{"name": collectionName})
	if err != nil {
		return false, err
	}
	return len(collections) != 0, nil
}

func supportsObservableLongSQL(driver string) bool {
	return driver == database.POSTGRES || driver == database.MYSQL || driver == database.SQLSERVER
}

// runInterruptedSQLStatementRecovery kills the migration process only after
// the database itself reports that the deliberately slow migration statement
// is active. This tests the failure window inside ExecSQL, rather than the
// synchronization point between files.
func runInterruptedSQLStatementRecovery(t *testing.T, driver string) {
	t.Helper()
	fixture := internaltest.NewMigrationProcessFixture(t, driver)
	fixture.SetSecondMigrationContent(t, longRunningSQLStatement(driver))
	app := openMigrationProcessApp(t, driver, fixture)
	defer fixture.Cleanup(t, app)
	marker := filepath.Join(fixture.Directory, "runner-paused-before-slow-statement")
	release := filepath.Join(fixture.Directory, "runner-release-slow-statement")
	process, output, done := startMigrationProcess(t, driver, fixture.Directory, "migration:"+fixture.SecondName, marker, release)
	waitForMigrationMarker(t, marker, done, output)
	writeRelease(t, release)
	waitForSQLMigrationStatement(t, driver, app, done, output)
	if err := process.Process.Kill(); err != nil {
		t.Fatalf("kill %s migration process during an active SQL statement: %v", driver, err)
	}
	if err := <-done; err == nil {
		t.Fatalf("killed %s migration process unexpectedly exited successfully during SQL statement", driver)
	}
	fixture.RequireInterruptedState(t, app)
	if err := app.RunDatabaseStartupTasks(); err == nil || !strings.Contains(strings.ToLower(err.Error()), "lock") {
		t.Fatalf("%s startup after interrupted SQL statement returned %v, want retained-lock rejection", driver, err)
	}
	fixture.Recover(t, app)
	fixture.SetSecondMigrationContent(t, fixture.DefaultSecondMigrationContent())
	if err := app.RunDatabaseStartupTasks(); err != nil {
		t.Fatalf("rerun %s migration after interrupted SQL statement recovery: %v", driver, err)
	}
	fixture.RequireCompleted(t, app)
	t.Logf("%s migration process killed while its database reports the test statement active; running state was recovered and rerun passed", driver)
}

func longRunningSQLStatement(driver string) string {
	switch driver {
	case database.POSTGRES:
		return "SELECT pg_sleep(30);"
	case database.MYSQL:
		return "SELECT SLEEP(30);"
	case database.SQLSERVER:
		return "WAITFOR DELAY '00:00:30';"
	default:
		return ""
	}
}

func waitForSQLMigrationStatement(t *testing.T, driver string, app *gorest.App, done <-chan error, output *bytes.Buffer) {
	t.Helper()
	queries := map[string]string{
		database.POSTGRES:  "SELECT COUNT(*) FROM pg_stat_activity WHERE state = 'active' AND query ILIKE '%pg_sleep(30)%'",
		database.MYSQL:     "SELECT COUNT(*) FROM information_schema.processlist WHERE command <> 'Sleep' AND info LIKE '%SLEEP(30)%'",
		database.SQLSERVER: "SELECT COUNT(*) FROM sys.dm_exec_requests r CROSS APPLY sys.dm_exec_sql_text(r.sql_handle) AS q WHERE q.text LIKE '%WAITFOR DELAY%'",
	}
	deadline := time.NewTimer(15 * time.Second)
	tick := time.NewTicker(25 * time.Millisecond)
	defer deadline.Stop()
	defer tick.Stop()
	for {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		rows, message, code, err := app.Database().QuerySQL(ctx, queries[driver])
		active := int64(0)
		if err == nil {
			if !rows.Next() {
				err = rows.Err()
			} else {
				err = rows.Scan(&active)
			}
			closeErr := rows.Close()
			if err == nil {
				err = closeErr
			}
		}
		cancel()
		if err != nil {
			t.Fatalf("inspect active %s migration statement (HTTP %d, %s): %v", driver, code, message, err)
		}
		if active > 0 {
			return
		}
		select {
		case err := <-done:
			t.Fatalf("%s migration process exited before the database reported its statement active: %v\n%s", driver, err, output.String())
		case <-deadline.C:
			t.Fatalf("timed out waiting for active %s migration statement\n%s", driver, output.String())
		case <-tick.C:
		}
	}
}

// runDatabaseRestartMigrationRecovery force-stops the managed database
// container while a separate migration process holds the startup lock between
// files. It restarts the same container, verifies the persisted running state
// and lock, then exercises the documented operator-recovery path.
func runDatabaseRestartMigrationRecovery(t *testing.T, driver string, container testcontainers.Container) {
	t.Helper()
	fixture := internaltest.NewMigrationProcessFixture(t, driver)
	app := internaltest.OpenAppWithConfigWithoutCleanup(t, driver, fixture.Configure)
	marker := filepath.Join(fixture.Directory, "runner-paused-for-db-restart")
	release := filepath.Join(fixture.Directory, "runner-release-unused")
	process, output, done := startMigrationProcess(t, driver, fixture.Directory, "migration:"+fixture.SecondName, marker, release)
	waitForMigrationMarker(t, marker, done, output)

	if driver == database.SCYLLA {
		flushScyllaKeyspace(t, container)
	}

	// Close the observer's pool before stopping the server. The migration
	// subprocess remains alive and owns the database-backed startup lock.
	if _, _, err := app.Close(); err != nil {
		t.Fatalf("close %s observer before database restart: %v", driver, err)
	}
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 30*time.Second)
	zeroGrace := time.Duration(0)
	err := container.Stop(stopCtx, &zeroGrace)
	stopCancel()
	if err != nil {
		t.Fatalf("force-stop %s database container during migration: %v", driver, err)
	}
	startCtx, startCancel := context.WithTimeout(context.Background(), 10*time.Minute)
	err = container.Start(startCtx)
	startCancel()
	if err != nil {
		t.Fatalf("restart %s database container during migration recovery scenario: %v", driver, err)
	}
	refreshContainerEndpoint(t, driver, container)
	waitForContainerPort(t, driver, container)

	// The original runner is still paused outside the database. Kill it after
	// the server returns, then inspect durable migration state with a fresh app.
	if err := process.Process.Kill(); err != nil {
		t.Fatalf("kill %s migration process after database restart: %v", driver, err)
	}
	if err := <-done; err == nil {
		t.Fatalf("killed %s migration process unexpectedly exited successfully", driver)
	}
	restartedApp := openMigrationProcessApp(t, driver, fixture)
	defer fixture.Cleanup(t, restartedApp)
	fixture.WaitForDatabase(t, restartedApp)
	fixture.RequireInterruptedState(t, restartedApp)
	if err := restartedApp.RunDatabaseStartupTasks(); err == nil || !strings.Contains(strings.ToLower(err.Error()), "lock") {
		t.Fatalf("%s startup after database restart returned %v, want retained-lock rejection", driver, err)
	}
	fixture.Recover(t, restartedApp)
	if err := restartedApp.RunDatabaseStartupTasks(); err != nil {
		t.Fatalf("rerun %s migrations after database restart recovery: %v", driver, err)
	}
	fixture.RequireCompleted(t, restartedApp)
}

func refreshContainerEndpoint(t *testing.T, driver string, container testcontainers.Container) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	portByDriver := map[string]string{
		database.POSTGRES:  "5432/tcp",
		database.ORACLE:    "1521/tcp",
		database.SQLSERVER: "1433/tcp",
		database.MYSQL:     "3306/tcp",
		database.MONGO:     "27017/tcp",
		database.SCYLLA:    "9042/tcp",
	}
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("get %s host after container restart: %v", driver, err)
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}
	port, err := container.MappedPort(ctx, nat.Port(portByDriver[driver]))
	if err != nil {
		t.Fatalf("get %s mapped port after container restart: %v", driver, err)
	}
	prefix := "GOREST_IT_" + strings.ToUpper(driver)
	t.Setenv(prefix+"_HOST", host)
	t.Setenv(prefix+"_PORT", port.Port())
}

func waitForContainerPort(t *testing.T, driver string, container testcontainers.Container) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("get %s host while waiting for restart: %v", driver, err)
	}
	if host == "localhost" {
		host = "127.0.0.1"
	}
	portByDriver := map[string]string{
		database.POSTGRES:  "5432/tcp",
		database.ORACLE:    "1521/tcp",
		database.SQLSERVER: "1433/tcp",
		database.MYSQL:     "3306/tcp",
		database.MONGO:     "27017/tcp",
		database.SCYLLA:    "9042/tcp",
	}
	port, err := container.MappedPort(ctx, nat.Port(portByDriver[driver]))
	if err != nil {
		t.Fatalf("get %s mapped port while waiting for restart: %v", driver, err)
	}
	address := net.JoinHostPort(host, port.Port())
	deadline := time.Now().Add(10 * time.Minute)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, dialErr := net.DialTimeout("tcp", address, time.Second)
		if dialErr == nil {
			_ = conn.Close()
			if driver == database.ORACLE {
				waitForOracleService(t, container)
			}
			// Give the database time to finish crash recovery after opening its
			// listener before the fresh GOREST client initializes.
			time.Sleep(time.Second)
			return
		}
		lastErr = dialErr
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("%s listener did not return after container restart at %s: %v", driver, address, lastErr)
}

func waitForOracleService(t *testing.T, container testcontainers.Container) {
	t.Helper()

	deadline := time.Now().Add(10 * time.Minute)
	var lastErr error

	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		command := fmt.Sprintf(
			`printf '%%s\n' "set heading off feedback off" "select 'GOREST_ORACLE_READY' from dual;" "exit" | sqlplus -L -s %s/%s@//127.0.0.1:1521/%s`,
			oracleAppUser,
			oracleAppPassword,
			oracleService,
		)
		exitCode, output, err := container.Exec(ctx, []string{"bash", "-lc", command})
		if err == nil {
			var status []byte
			status, err = io.ReadAll(output)
			if err == nil && exitCode == 0 && strings.Contains(string(status), "GOREST_ORACLE_READY") {
				cancel()

				return
			}
			if err == nil {
				err = fmt.Errorf("application connection probe did not succeed (exit code %d): %s", exitCode, strings.TrimSpace(string(status)))
			}
		}
		cancel()
		lastErr = err
		time.Sleep(500 * time.Millisecond)
	}

	t.Fatalf("Oracle FREEPDB1 service did not register after container restart: %v", lastErr)
}

func runIndependentMigrationRunners(t *testing.T, driver string) {
	t.Helper()
	fixture := internaltest.NewMigrationProcessFixture(t, driver)
	app := openMigrationProcessApp(t, driver, fixture)
	defer fixture.Cleanup(t, app)
	marker := filepath.Join(fixture.Directory, "runner-a-paused")
	release := filepath.Join(fixture.Directory, "runner-a-release")
	_, firstOutput, firstDone := startMigrationProcess(t, driver, fixture.Directory, "migration:"+fixture.SecondName, marker, release)
	waitForMigrationMarker(t, marker, firstDone, firstOutput)

	second := migrationProcessCommand(t, driver, fixture.Directory, "", "", "")
	secondOutput, secondErr := second.CombinedOutput()
	if secondErr == nil || !strings.Contains(strings.ToLower(string(secondOutput)), "lock") {
		writeRelease(t, release)
		<-firstDone
		t.Fatalf("second independent %s migration runner result = %v, output=%s; want startup-lock rejection", driver, secondErr, secondOutput)
	}
	writeRelease(t, release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first independent %s migration runner failed: %v\n%s", driver, err, firstOutput.String())
	}
	fixture.RequireCompleted(t, app)
}

func runKilledMigrationRunnerRecovery(t *testing.T, driver string) {
	t.Helper()
	fixture := internaltest.NewMigrationProcessFixture(t, driver)
	app := openMigrationProcessApp(t, driver, fixture)
	defer fixture.Cleanup(t, app)
	marker := filepath.Join(fixture.Directory, "runner-paused-before-second-file")
	release := filepath.Join(fixture.Directory, "runner-release-unused")
	process, output, done := startMigrationProcess(t, driver, fixture.Directory, "migration:"+fixture.SecondName, marker, release)
	waitForMigrationMarker(t, marker, done, output)
	if err := process.Process.Kill(); err != nil {
		t.Fatalf("kill %s migration process while second file is running: %v", driver, err)
	}
	if err := <-done; err == nil {
		t.Fatalf("killed %s migration process unexpectedly exited successfully", driver)
	}

	fixture.RequireInterruptedState(t, app)
	if err := app.RunDatabaseStartupTasks(); err == nil || !strings.Contains(strings.ToLower(err.Error()), "lock") {
		t.Fatalf("%s startup after process crash returned %v, want stale-lock rejection", driver, err)
	}
	fixture.Recover(t, app)
	if err := app.RunDatabaseStartupTasks(); err != nil {
		t.Fatalf("rerun %s migrations after operator recovery: %v", driver, err)
	}
	fixture.RequireCompleted(t, app)
}

func startMigrationProcess(t *testing.T, driver, directory, pauseState, marker, release string) (*exec.Cmd, *bytes.Buffer, <-chan error) {
	t.Helper()
	cmd := migrationProcessCommand(t, driver, directory, pauseState, marker, release)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatalf("start independent %s migration process: %v", driver, err)
	}
	done := make(chan error, 1)
	finished := make(chan struct{})
	go func() {
		done <- cmd.Wait()
		close(finished)
	}()
	t.Cleanup(func() {
		select {
		case <-finished:
			return
		default:
		}
		_ = cmd.Process.Kill()
		<-finished
	})
	return cmd, &output, done
}

func migrationProcessCommand(t *testing.T, driver, directory, pauseState, marker, release string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=^"+migrationProcessHelperTest+"$")
	cmd.Env = append(os.Environ(),
		"GOREST_IT_MIGRATION_PROCESS_MODE=run",
		"GOREST_IT_MIGRATION_PROCESS_DRIVER="+driver,
		"GOREST_IT_MIGRATION_PROCESS_DIRECTORY="+directory,
		"GOREST_IT_MIGRATION_PAUSE_STATE="+pauseState,
		"GOREST_IT_MIGRATION_PAUSE_MARKER="+marker,
		"GOREST_IT_MIGRATION_PAUSE_RELEASE="+release,
	)
	return cmd
}

func waitForMigrationMarker(t *testing.T, marker string, done <-chan error, output *bytes.Buffer) {
	t.Helper()
	deadline := time.NewTimer(2 * time.Minute)
	tick := time.NewTicker(10 * time.Millisecond)
	defer deadline.Stop()
	defer tick.Stop()
	for {
		if _, err := os.Stat(marker); err == nil {
			return
		}
		select {
		case err := <-done:
			t.Fatalf("migration subprocess exited before pause marker: %v\n%s", err, output.String())
		case <-deadline.C:
			t.Fatalf("timed out waiting for migration pause marker %s\n%s", marker, output.String())
		case <-tick.C:
		}
	}
}

func writeRelease(t *testing.T, release string) {
	t.Helper()
	if err := os.WriteFile(release, []byte("continue"), 0o600); err != nil {
		t.Fatalf("release paused migration subprocess: %v", err)
	}
}

func openMigrationProcessApp(t *testing.T, driver string, fixture *internaltest.MigrationProcessFixture) *gorest.App {
	t.Helper()
	app := internaltest.OpenAppWithConfig(t, driver, fixture.Configure)
	return app
}
