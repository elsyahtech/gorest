package database

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"go.uber.org/zap"
)

func TestNormalizeDatabaseDriver(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{input: " MYSQL ", want: MYSQL},
		{input: "MariaDB", want: MYSQL},
		{input: "POSTGRES", want: POSTGRES},
		{input: "SQLSERVER", want: SQLSERVER},
		{input: "SQLite", want: SQLITE},
		{input: "Oracle", want: ORACLE},
		{input: "Mongo", want: MONGO},
		{input: "Scylla", want: SCYLLA},
		{input: "CustomDriver", want: "customdriver"},
	}

	for _, test := range tests {
		t.Run(test.input, func(t *testing.T) {
			if got := NormalizeDatabaseDriver(test.input); got != test.want {
				t.Fatalf("NormalizeDatabaseDriver(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestStringToIntHandlesEmptyInvalidAndValidValues(t *testing.T) {
	tests := []struct {
		input string
		want  int
	}{
		{input: "", want: 0},
		{input: "not-a-number", want: 0},
		{input: "1433", want: 1433},
		{input: "-1", want: -1},
	}
	for _, test := range tests {
		if got := stringToInt(test.input); got != test.want {
			t.Errorf("stringToInt(%q) = %d, want %d", test.input, got, test.want)
		}
	}
}

func TestSplitStatementsTrimsAndDropsEmptyStatements(t *testing.T) {
	got := splitStatements("  CREATE TABLE a (id int); ;\nINSERT INTO a VALUES (1);  ")
	want := []string{"CREATE TABLE a (id int)", "INSERT INTO a VALUES (1)"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("splitStatements() = %#v, want %#v", got, want)
	}
	if got := splitStatements(" ; \n ;"); len(got) != 0 {
		t.Fatalf("empty statements = %#v, want none", got)
	}
}

func TestEnsureDirCreatesNestedDirectoryWithExpectedPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "migrations")
	message, err := ensureDir(path, typeMigration)
	if err != nil {
		t.Fatalf("ensureDir() error = %v, message %q", err, message)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat created directory: %v", err)
	}
	if !info.IsDir() {
		t.Fatal("ensureDir() created a non-directory")
	}
	if got := info.Mode().Perm(); got != 0o700 {
		t.Fatalf("directory permissions = %#o, want 0700", got)
	}

	if message, err := ensureDir(path, typeMigration); err != nil || message != "" {
		t.Fatalf("ensureDir() on existing directory = %q, %v; want empty message and nil error", message, err)
	}
}

func TestDecodeCompensationJSONRequiresOneKnownValue(t *testing.T) {
	type document struct {
		Name string `json:"name"`
	}
	tests := []struct {
		name    string
		content string
		wantErr bool
		want    document
	}{
		{name: "valid", content: `{"name":"rollback"}`, want: document{Name: "rollback"}},
		{name: "malformed", content: `{"name":`, wantErr: true},
		{name: "unknown field", content: `{"name":"rollback","extra":true}`, wantErr: true},
		{name: "multiple values", content: `{"name":"first"} {"name":"second"}`, wantErr: true},
		{name: "trailing garbage", content: `{"name":"rollback"} garbage`, wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var got document
			err := decodeCompensationJSON(test.content, &got)
			if (err != nil) != test.wantErr {
				t.Fatalf("decodeCompensationJSON() error = %v, wantErr %t", err, test.wantErr)
			}
			if err == nil && got != test.want {
				t.Fatalf("decoded document = %#v, want %#v", got, test.want)
			}
		})
	}
}

func TestFilterPendingMigrationFiles(t *testing.T) {
	files := []File{{Name: "001_init.sql"}, {Name: "002_users.sql"}, {Name: "003_orders.sql"}}
	got := filterPendingMigrationExec(files, []string{"002_users.sql"})
	want := []File{files[0], files[2]}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("pending files = %#v, want %#v", got, want)
	}
	if got := filterPendingMigrationExec(files, []string{"001_init.sql", "002_users.sql", "003_orders.sql"}); len(got) != 0 {
		t.Fatalf("all-executed files = %#v, want none", got)
	}
}

func TestMigrationLockExistsDetection(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{name: "nil", want: false},
		{name: "Oracle object exists", err: errors.New("ORA-00955: name is already used by an existing object"), want: true},
		{name: "SQL Server object exists", err: errors.New("There is already an object named 'gorest_migration_lock'"), want: true},
		{name: "generic already exists", err: errors.New("table already exists"), want: true},
		{name: "Postgres lock table exists", err: errors.New(`relation "gorest_migration_lock" already exists`), want: true},
		{name: "unrelated error", err: errors.New("permission denied"), want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := isMigrationLockTableExistsError(test.err); got != test.want {
				t.Fatalf("isMigrationLockTableExistsError() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestValidateInterruptedExecutionScopesByTypeAndStatus(t *testing.T) {
	tests := []struct {
		name     string
		states   map[string]migrationState
		execType executionType
		wantErr  bool
	}{
		{
			name:     "no records",
			states:   map[string]migrationState{},
			execType: typeMigration,
		},
		{
			name:     "applied migration",
			states:   map[string]migrationState{"migration:001_init.sql": {Status: "applied"}},
			execType: typeMigration,
		},
		{
			name:     "interrupted migration",
			states:   map[string]migrationState{"migration:001_init.sql": {Status: "running"}},
			execType: typeMigration,
			wantErr:  true,
		},
		{
			name:     "interrupted seeder does not block migrations",
			states:   map[string]migrationState{"seeder:001_seed.sql": {Status: "running"}},
			execType: typeMigration,
		},
		{
			name:     "interrupted seeder",
			states:   map[string]migrationState{"seeder:001_seed.sql": {Status: "failed"}},
			execType: typeSeeder,
			wantErr:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message, err := validateInterruptedExecution(test.states, test.execType)
			if (err != nil) != test.wantErr {
				t.Fatalf("validateInterruptedExecution() error = %v, wantErr %t", err, test.wantErr)
			}
			if test.wantErr && message == "" {
				t.Fatal("interrupted execution returned an empty recovery message")
			}
			if !test.wantErr && message != "" {
				t.Fatalf("successful validation returned message %q", message)
			}
		})
	}
}

func TestMigrationRollbackReliabilityAndFailureUnwrap(t *testing.T) {
	for _, driver := range []string{POSTGRES, SQLSERVER, SQLITE} {
		if !migrationRollbackIsReliable(driver) {
			t.Errorf("migrationRollbackIsReliable(%q) = false, want true", driver)
		}
	}
	for _, driver := range []string{MYSQL, ORACLE, MONGO, SCYLLA, "unknown"} {
		if migrationRollbackIsReliable(driver) {
			t.Errorf("migrationRollbackIsReliable(%q) = true, want false", driver)
		}
	}

	cause := errors.New("statement failed")
	failure := &sqlMigrationFailure{cause: cause, rolledBack: true}
	if failure.Error() != cause.Error() {
		t.Fatalf("failure error = %q, want %q", failure.Error(), cause.Error())
	}
	if !errors.Is(failure, cause) {
		t.Fatal("sqlMigrationFailure does not unwrap its cause")
	}
}

func TestSampleFileSelectsMigrationOrSeederDirectory(t *testing.T) {
	if got, want := sampleFile(POSTGRES, typeMigration), samplesBaseURL+"/migrations/postgres"; got != want {
		t.Fatalf("migration sample URL = %q, want %q", got, want)
	}
	if got, want := sampleFile(MONGO, typeSeeder), samplesBaseURL+"/seeders/mongo"; got != want {
		t.Fatalf("seeder sample URL = %q, want %q", got, want)
	}
}

func TestMigrationAndSeederValidateConfigurationAndHonorDisabledFlags(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()
	db := &Database{}

	if message, err := db.Migration(ctx, nil, logger); err == nil || message == "" {
		t.Fatalf("Migration(nil config) = %q, %v; want message and error", message, err)
	}
	if message, err := db.Migration(ctx, &Config{}, nil); err == nil || message == "" {
		t.Fatalf("Migration(nil logger) = %q, %v; want message and error", message, err)
	}
	if message, err := db.Seeder(ctx, nil, logger); err == nil || message == "" {
		t.Fatalf("Seeder(nil config) = %q, %v; want message and error", message, err)
	}
	if message, err := db.Seeder(ctx, &Config{}, nil); err == nil || message == "" {
		t.Fatalf("Seeder(nil logger) = %q, %v; want message and error", message, err)
	}

	if message, err := db.Migration(ctx, &Config{Migration: false}, logger); err != nil || message != "" {
		t.Fatalf("disabled Migration() = %q, %v; want empty message and nil error", message, err)
	}
	if message, err := db.Seeder(ctx, &Config{Seeder: false}, logger); err != nil || message != "" {
		t.Fatalf("disabled Seeder() = %q, %v; want empty message and nil error", message, err)
	}
}

func TestNewRejectsNilAndUnsupportedConfiguration(t *testing.T) {
	if db, message, err := New(context.Background(), nil, "UTC"); err == nil || db != nil || message == "" {
		t.Fatalf("New(nil) = %#v, %q, %v; want nil database with message and error", db, message, err)
	}
	if db, message, err := New(context.Background(), &Config{Driver: "unknown"}, "UTC"); err == nil || db != nil || message == "" {
		t.Fatalf("New(unknown driver) = %#v, %q, %v; want nil database with message and error", db, message, err)
	}
}

func TestNewOpensSQLiteMemoryDatabase(t *testing.T) {
	db, message, err := New(context.Background(), &Config{
		Driver: SQLITE,
		Name:   ":memory:",
	}, "UTC")
	if err != nil {
		t.Fatalf("New(SQLite memory) error = %v, message %q", err, message)
	}
	if db == nil || db.SQL == nil {
		t.Fatal("New(SQLite memory) returned no SQL database")
	}
	if err := db.SQL.Ping(); err != nil {
		t.Fatalf("ping newly opened SQLite memory database: %v", err)
	}
	if err := db.SQL.Close(); err != nil {
		t.Fatalf("close SQLite memory database: %v", err)
	}
}

func TestGenerateSQLiteDatabasePreservesExistingFile(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "nested")
	config := &Config{DatabaseDirectory: directory, Name: "service"}
	path, err := generateSQLiteDatabase(config)
	if err != nil {
		t.Fatalf("generateSQLiteDatabase() error = %v", err)
	}
	if want := filepath.Join(directory, "service.db"); path != want {
		t.Fatalf("SQLite path = %q, want %q", path, want)
	}
	if err := os.WriteFile(path, []byte("existing content"), 0o600); err != nil {
		t.Fatalf("write existing SQLite file: %v", err)
	}
	if got, err := generateSQLiteDatabase(config); err != nil {
		t.Fatalf("generateSQLiteDatabase() for existing file error = %v", err)
	} else if got != path {
		t.Fatalf("existing SQLite path = %q, want %q", got, path)
	}
	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read existing SQLite file: %v", err)
	}
	if string(content) != "existing content" {
		t.Fatalf("existing SQLite file content = %q; file was unexpectedly truncated", content)
	}
}

func TestSQLiteFilePathRejectsEmptyNameAndSupportsMemory(t *testing.T) {
	if path, err := sqliteFilePath(&Config{}); err == nil || path != "" {
		t.Fatalf("sqliteFilePath(empty config) = %q, %v; want error", path, err)
	}
	if path, err := sqliteFilePath(&Config{Name: ":memory:"}); err != nil || path != ":memory:" {
		t.Fatalf("sqliteFilePath(memory) = %q, %v; want :memory:", path, err)
	}
}
