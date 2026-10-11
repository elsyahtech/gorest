package database

import (
	"cmp"
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
func readFiles(driver, dir string) ([]File, string, error) {
	ext, message, err := fileExtension(driver)
	if err != nil {
		return nil, message, err
	}

	entries, err := os.ReadDir(dir)

	if err != nil {
		if os.IsNotExist(err) {
			return nil, "", nil // no directory = no files, not an error
		}

		message := fmt.Sprintf("Ensure the directory %s has proper read permissions", dir)

		return nil, message, fmt.Errorf("read directory %s: %w", dir, err)
	}

	files := make([]File, 0, len(entries))

	// Process each file in directory
	for _, entry := range entries {
		// Case-insensitive match so ".SQL" is also read.
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ext) {
			continue
		}

		filePath := filepath.Join(dir, entry.Name())

		content, err := os.ReadFile(filePath)
		if err != nil {
			message := fmt.Sprintf("Check file %s read permissions or if the file is locked", filePath)

			return nil, message, fmt.Errorf("read file %s: %w", filePath, err)
		}

		files = append(files, File{
			Name:    entry.Name(),
			Path:    filePath,
			Content: string(content),
			Number:  extractNumber(ext, entry.Name()),
		})
	}

	// Sort by number, then by name so duplicate numbers have a deterministic order.
	slices.SortFunc(files, func(goyang, joget File) int {
		if cmpr := cmp.Compare(goyang.Number, joget.Number); cmpr != 0 {
			return cmpr
		}

		return strings.Compare(goyang.Name, joget.Name)
	})

	return files, "", nil
}

// fileExtension returns the migration file extension for a driver.
func fileExtension(driver string) (string, string, error) {
	switch driver {
	case MONGO:
		return ".json", "", nil
	case MYSQL, POSTGRES, SQLSERVER, ORACLE, SQLITE:
		return ".sql", "", nil
	case SCYLLA:
		return ".cql", "", nil
	default:
		return "", "Ensure your database driver is mysql, postgresql, mssql, oracle, sqlite, mongodb or scylladb",
			fmt.Errorf("driver %s is not supported", driver)
	}
}
