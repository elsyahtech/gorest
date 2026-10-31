package database

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadMigrationDirectoryProductionInputs(t *testing.T) {
	t.Run("missing directory is empty", func(t *testing.T) {
		entries, message, err := readMigrationDirectory(filepath.Join(t.TempDir(), "missing"))
		if err != nil || message != "" || len(entries) != 0 {
			t.Fatalf("readMigrationDirectory(missing) = (%#v, %q, %v)", entries, message, err)
		}
	})

	t.Run("regular file is not a directory", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "migration.sql")
		if err := os.WriteFile(path, []byte("SELECT 1;"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, message, err := readMigrationDirectory(path); err == nil || message == "" {
			t.Fatalf("readMigrationDirectory(file) = message %q, error %v; want directory error", message, err)
		}
	})

	t.Run("empty directory", func(t *testing.T) {
		entries, message, err := readMigrationDirectory(t.TempDir())
		if err != nil || message != "" || len(entries) != 0 {
			t.Fatalf("readMigrationDirectory(empty) = (%#v, %q, %v)", entries, message, err)
		}
	})
}

func TestReadFilesPairsDownCompensationWithoutExecutingIt(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"001_users.sql":      "CREATE TABLE users (id INTEGER);",
		"001_users.down.sql": "DROP TABLE users;",
		"002_roles.SQL":      "CREATE TABLE roles (id INTEGER);",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}

	got, _, err := readFiles(SQLITE, dir)
	if err != nil {
		t.Fatalf("readFiles() error = %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("readFiles() returned %d files, want 2", len(got))
	}
	if got[0].Name != "001_users.sql" || got[0].CompensationContent != "DROP TABLE users;" {
		t.Errorf("first file = %#v, want paired down script", got[0])
	}
	if got[1].Name != "002_roles.SQL" || got[1].CompensationContent != "" {
		t.Errorf("second file = %#v, want regular file without compensation", got[1])
	}
}

func TestReadFilesRejectsOrphanDownCompensation(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "001_users.down.sql")
	if err := os.WriteFile(path, []byte("DROP TABLE users;"), 0o600); err != nil {
		t.Fatalf("write orphan compensation: %v", err)
	}

	if _, _, err := readFiles(SQLITE, dir); err == nil {
		t.Fatal("readFiles() accepted compensation without matching up file")
	}
}

func TestRepositoryCompensationExamplesPairForAllDrivers(t *testing.T) {
	examples := []struct {
		name string
		root string
		path string
	}{
		{name: "mysql migration", root: MYSQL, path: "migrations/mysql"},
		{name: "postgres migration", root: POSTGRES, path: "migrations/postgres"},
		{name: "sqlserver migration", root: SQLSERVER, path: "migrations/sqlserver"},
		{name: "oracle migration", root: ORACLE, path: "migrations/oracle"},
		{name: "sqlite migration", root: SQLITE, path: "migrations/sqlite"},
		{name: "mongo migration", root: MONGO, path: "migrations/mongo"},
		{name: "scylla migration", root: SCYLLA, path: "migrations/scylla"},
		{name: "mysql seeder", root: MYSQL, path: "seeders/mysql"},
		{name: "postgres seeder", root: POSTGRES, path: "seeders/postgres"},
		{name: "sqlserver seeder", root: SQLSERVER, path: "seeders/sqlserver"},
		{name: "oracle seeder", root: ORACLE, path: "seeders/oracle"},
		{name: "sqlite seeder", root: SQLITE, path: "seeders/sqlite"},
		{name: "mongo seeder", root: MONGO, path: "seeders/mongo"},
		{name: "scylla seeder", root: SCYLLA, path: "seeders/scylla"},
	}

	for _, example := range examples {
		t.Run(example.name, func(t *testing.T) {
			files, _, err := readFiles(example.root, example.path)
			if err != nil {
				t.Fatalf("readFiles() error = %v", err)
			}
			if len(files) == 0 {
				t.Fatal("readFiles() returned no examples")
			}
			for _, file := range files {
				if strings.TrimSpace(file.CompensationContent) == "" {
					t.Errorf("%s has no paired compensation file", file.Name)
				}
			}
		})
	}
}
