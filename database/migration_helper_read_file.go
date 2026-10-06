package database

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// readFiles reads all migration files from the migration directory
// Reads .json, .sql, .cql files, ignores directories and other file types
// Returns migrations sorted by number (001, 002, 003, etc)
// If directory doesn't exist, returns empty list (not an error).
func readFiles(driver, migrationDir string, execType executionType) ([]File, string, error) {
	// Try to read directory
	entries, err := os.ReadDir(migrationDir)
	if err != nil {
		message := fmt.Sprintf("ensure the directory %s exists, has proper read permissions, "+
			"and the path is correctly configured.", migrationDir)

		// Directory doesn't exist is OK (no migrations yet)
		if os.IsNotExist(err) {
			message := fmt.Sprintf("ensure the directory %s exists, has proper read permissions, "+
				"and the path is correctly configured. Please create %s file in directory: %s", migrationDir, execType, migrationDir)

			return make([]File, 0), message, errors.New("directory doesn't exist")
		}

		return nil, message, fmt.Errorf("read directory %s failed: %w", migrationDir, err)
	}

	var (
		files []File
		ext   string
	)

	switch driver {
	case MONGO:
		ext = ".json"
	case MYSQL, POSTGRES, SQLSERVER, ORACLE, SQLITE:
		ext = ".sql"
	case SCYLLA:
		ext = ".cql"
	default:
		const message = "ensure your driver database configured using mysql, postgresql, mssql, oracle, sqlite, mongodb or scylladb"

		return nil, message, fmt.Errorf("driver %s not support", driver)
	}

	// Process each file in directory
	for _, entry := range entries {
		// Skip directories
		if entry.IsDir() {
			continue
		}

		// Only process .sql files
		if !strings.HasSuffix(entry.Name(), ext) {
			continue
		}

		// Read file content
		filePath := filepath.Join(migrationDir, entry.Name())

		content, err := os.ReadFile(filePath)
		if err != nil {
			message := fmt.Sprintf("check file %s read permissions or if the file is currently locked/corrupted.", filePath)

			return nil, message, fmt.Errorf("read file %s failed: %w", filePath, err)
		}

		// Create migration file struct
		migration := File{
			Name:    entry.Name(),
			Path:    filePath,
			Content: string(content),
			Number:  extractNumber(ext, entry.Name()),
		}

		files = append(files, migration)
	}

	// Sort by number (001 < 002 < 003, etc)
	// Ensures migrations execute in correct order
	slices.SortFunc(files, func(a, b File) int {
		if a.Number < b.Number {
			return -1
		}

		if a.Number > b.Number {
			return 1
		}

		return 0
	})

	return files, "", nil
}
