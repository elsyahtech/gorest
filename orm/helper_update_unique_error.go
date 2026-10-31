package orm

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"

	"github.com/elsyahtech/gorest/database"
)

var (
	oracleUniqueColumnsPattern = regexp.MustCompile(`(?i)columns\s*\(([^)]*)\)`)
	postgresUniquePattern      = regexp.MustCompile(`(?i)key\s*\(([^)]*)\)\s*=\s*\(([^)]*)\)`)
	sqliteUniquePattern        = regexp.MustCompile(`(?i)unique constraint failed:\s*([^\r\n]+)`)
	mysqlDuplicatePattern      = regexp.MustCompile(`(?i)duplicate entry\s+['"]?(.*?)['"]?\s+for key\s+['"]?([^'"\s]+)`)
	sqlServerDuplicatePattern  = regexp.MustCompile(`(?i)duplicate key value is\s*\(([^)]*)\)`)
	mongoDuplicatePattern      = regexp.MustCompile(`(?i)dup key:\s*\{([^}]*)\}`)
	mongoPairPattern           = regexp.MustCompile(`([\w.$-]+):\s*(?:"([^"]*)"|'([^']*)'|([^,}\s]+))`)
)

// enrichBulkUpdateUniqueViolation adds values and primary keys from the request
// when a driver's unique-constraint error identifies the conflicting columns.
func (orm *ORM) enrichBulkUpdateUniqueViolation(err error, rows []reflect.Value, meta columnMetaData, driver string) error {
	if err == nil {
		return nil
	}

	columns, exactValues := parseUniqueViolationDetails(err.Error(), meta, driver)

	if len(columns) == 0 {
		return err
	}

	mappedColumns, columnIndexes, mappedValues := enrichUniqueViolationData(columns, exactValues, meta)

	columns = mappedColumns
	exactValues = mappedValues

	if len(columns) == 0 {
		return err
	}

	counts, rowValues := enrichBulkUpdateUniqueViolation(rows, columnIndexes)

	details := enrichBuildDuplicateDetails(rowValues, exactValues, counts, columns, meta, rows)

	valueLabel := "submitted values"

	if len(details) > 0 {
		valueLabel = "duplicate request values"
	}

	context := fmt.Sprintf("Bulk update conflicts on unique column(s) %s; %s: %s", strings.Join(columns, ", "), valueLabel, strings.Join(details, "; "))
	if len(details) == 0 {
		// Some drivers report the unique value but not the affected batch row.
		// Include matching request rows where possible, otherwise show the
		// submitted values for the named unique column(s) as candidates.
		details = buildViolationDetails(rows, columnIndexes, exactValues, columns, meta, details)

		label := "submitted values"

		if len(exactValues) > 0 {
			label = "conflicting value(s)"
		}

		context = fmt.Sprintf("Bulk update conflicts on unique column(s) %s; %s: %s", strings.Join(columns, ", "), label, strings.Join(details, "; "))
	}

	// Keep the shared troubleshooting instruction and official reference while
	// adding the request rows that can help the caller resolve this conflict.
	if orm.Message == "" {
		orm.Message = databaseTroubleshootingMessage(databaseConflictError)
	}

	orm.Message += " Details: " + context

	return err
}

func parseUniqueViolationDetails(message string, meta columnMetaData, driver string) (col []string, val []string) {
	if match := oracleUniqueColumnsPattern.FindStringSubmatch(message); len(match) == 2 {
		return splitUniqueColumns(match[1]), nil
	}

	if match := postgresUniquePattern.FindStringSubmatch(message); len(match) == 3 {
		return splitUniqueColumns(match[1]), splitUniqueColumns(match[2])
	}

	if match := sqliteUniquePattern.FindStringSubmatch(message); len(match) == 2 {
		var columns []string

		for _, raw := range strings.Split(match[1], ",") {
			parts := strings.Split(strings.TrimSpace(raw), ".")

			columns = append(columns, strings.Trim(parts[len(parts)-1], "\"'[]`"))
		}

		return columns, nil
	}

	if match := mysqlDuplicatePattern.FindStringSubmatch(message); len(match) == 3 {
		return inferUniqueColumns(match[2], meta), []string{strings.Trim(match[1], `"'`)}
	}

	if match := sqlServerDuplicatePattern.FindStringSubmatch(message); len(match) == 2 {
		return inferUniqueColumns(message, meta), splitUniqueColumns(match[1])
	}

	if match := mongoDuplicatePattern.FindStringSubmatch(message); len(match) == 2 {
		var columns, values []string

		for _, pair := range mongoPairPattern.FindAllStringSubmatch(match[1], -1) {
			columns = append(columns, pair[1])

			value := pair[2]

			if value == "" {
				value = pair[3]
			}

			if value == "" {
				value = pair[4]
			}

			values = append(values, value)
		}

		return columns, values
	}

	if driver == database.SCYLLA &&
		(strings.Contains(strings.ToLower(message), "already exists") ||
			strings.Contains(strings.ToLower(message), "duplicate")) {
		return meta.primaryKeyColumns, nil
	}

	return nil, nil
}
