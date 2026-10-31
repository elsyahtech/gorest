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

	entries, message, err := readMigrationDirectory(dir)
	if err != nil {
		return nil, message, err
	}

	files, downFiles, message, err := readMigrationEntries(entries, dir, ext)
	if err != nil {
		return nil, message, err
	}

	if message, err := pairCompensationFiles(files, downFiles, ext); err != nil {
		return nil, message, err
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

func readMigrationDirectory(dir string) ([]os.DirEntry, string, error) {
	entries, err := os.ReadDir(dir)
	if err == nil {
		return entries, "", nil
	}

	if os.IsNotExist(err) {
		return nil, "", nil
	}

	message := fmt.Sprintf("Ensure the directory %s has proper read permissions", dir)

	return nil, message, fmt.Errorf("read directory %s: %w", dir, err)
}

func readMigrationEntries(entries []os.DirEntry, dir, ext string) ([]File, map[string]string, string, error) {
	files := make([]File, 0, len(entries))
	upFiles := make(map[string]struct{}, len(entries))
	downFiles := make(map[string]string)

	for _, entry := range entries {
		if !isMigrationFile(entry, ext) {
			continue
		}

		file, baseName, isCompensation, message, err := readMigrationEntry(entry, dir, ext)
		if err != nil {
			return nil, nil, message, err
		}

		if isCompensation {
			if _, exists := downFiles[baseName]; exists {
				message := fmt.Sprintf("Keep only one compensation file for %s.", baseName)

				return nil, nil, message, fmt.Errorf("duplicate compensation file for %s", baseName)
			}

			downFiles[baseName] = file.Content

			continue
		}

		if _, exists := upFiles[baseName]; exists {
			name := strings.TrimSuffix(file.Name, filepath.Ext(file.Name))
			message := fmt.Sprintf("Use unique migration/seeder filenames; %s is duplicated ignoring case.", name)

			return nil, nil, message, fmt.Errorf("duplicate migration or seeder filename %q", name)
		}

		upFiles[baseName] = struct{}{}

		files = append(files, file)
	}

	return files, downFiles, "", nil
}

func isMigrationFile(entry os.DirEntry, ext string) bool {
	return !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ext)
}

func readMigrationEntry(entry os.DirEntry, dir, ext string) (File, string, bool, string, error) {
	filePath := filepath.Join(dir, entry.Name())

	content, err := os.ReadFile(filePath)
	if err != nil {
		message := fmt.Sprintf("Check file %s read permissions or if the file is locked", filePath)

		return File{}, "", false, message, fmt.Errorf("read file %s: %w", filePath, err)
	}

	nameWithoutExt := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
	baseName := strings.ToLower(nameWithoutExt)

	isCompensation := strings.HasSuffix(baseName, ".down")
	if isCompensation {
		baseName = strings.TrimSuffix(baseName, ".down")
	}

	return File{
		Name:    entry.Name(),
		Path:    filePath,
		Content: string(content),
		Number:  extractNumber(ext, entry.Name()),
	}, baseName, isCompensation, "", nil
}

func pairCompensationFiles(files []File, downFiles map[string]string, ext string) (string, error) {
	upFiles := make(map[string]struct{}, len(files))

	for _, file := range files {
		baseName := strings.ToLower(strings.TrimSuffix(file.Name, filepath.Ext(file.Name)))
		upFiles[baseName] = struct{}{}
	}

	for baseName := range downFiles {
		if _, exists := upFiles[baseName]; !exists {
			message := fmt.Sprintf(
				"Remove the orphan compensation file for %s or add its matching migration/seeder file.",
				baseName,
			)

			return message, fmt.Errorf("compensation file has no matching migration or seeder: %s.down%s", baseName, ext)
		}
	}

	for index := range files {
		baseName := strings.ToLower(strings.TrimSuffix(files[index].Name, filepath.Ext(files[index].Name)))
		files[index].CompensationContent = downFiles[baseName]
	}

	return "", nil
}

// fileExtension returns the migration file extension for a driver.
func fileExtension(driver string) (res string, msg string, err error) {
	switch driver {
	case MONGO:
		return ".json", "", nil
	case MYSQL, POSTGRES, SQLSERVER, ORACLE, SQLITE:
		return ".sql", "", nil
	case SCYLLA:
		return ".cql", "", nil
	default:
		return "", "Ensure your database driver is MYSQL, POSTGRES, SQLSERVER, ORACLE, SQLITE, MONGO or SCYLLA",
			fmt.Errorf("driver %s is not supported", driver)
	}
}
