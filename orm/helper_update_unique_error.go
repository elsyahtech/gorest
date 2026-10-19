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

	mappedColumns := make([]string, 0, len(columns))
	columnIndexes := make([]int, 0, len(columns))
	mappedValues := make([]string, 0, len(exactValues))
	for columnPosition, column := range columns {
		for i, mapped := range meta.columns {
			if strings.EqualFold(mapped, column) {
				mappedColumns = append(mappedColumns, mapped)
				columnIndexes = append(columnIndexes, meta.columnIndex[i])
				if columnPosition < len(exactValues) {
					mappedValues = append(mappedValues, exactValues[columnPosition])
				}
				break
			}
		}
	}
	columns = mappedColumns
	exactValues = mappedValues
	if len(columns) == 0 {
		return err
	}

	counts := make(map[string]int, len(rows))
	rowValues := make([][]any, len(rows))
	for i, row := range rows {
		values := make([]any, len(columnIndexes))
		for j, fieldIndex := range columnIndexes {
			values[j] = row.Field(fieldIndex).Interface()
		}
		rowValues[i] = values
		counts[primaryKeyTuple(values)]++
	}

	var details []string
	for i, values := range rowValues {
		if !rowMatchesExactUniqueValues(values, exactValues) {
			continue
		}
		key := primaryKeyTuple(values)
		if counts[key] < 2 {
			continue
		}
		parts := make([]string, len(columns))
		for j, column := range columns {
			parts[j] = fmt.Sprintf("%s=%v", column, values[j])
		}
		primaryKey := make([]any, len(meta.primaryKeyIndex))
		for j, fieldIndex := range meta.primaryKeyIndex {
			primaryKey[j] = rows[i].Field(fieldIndex).Interface()
		}
		detail := fmt.Sprintf("%s (%s)", primaryKeyLabel(primaryKey), strings.Join(parts, ", "))
		if !containsString(details, detail) {
			details = append(details, detail)
		}
	}

	valueLabel := "submitted values"
	if len(details) > 0 {
		valueLabel = "duplicate request values"
	}
	context := fmt.Sprintf("Bulk update conflicts on unique column(s) %s; %s: %s", strings.Join(columns, ", "), valueLabel, strings.Join(details, "; "))
	if len(details) == 0 {
		// Some drivers report the unique value but not the affected batch row.
		// Include matching request rows where possible, otherwise show the
		// submitted values for the named unique column(s) as candidates.
		for _, row := range rows {
			rowUniqueValues := make([]any, len(columnIndexes))
			for j, fieldIndex := range columnIndexes {
				rowUniqueValues[j] = row.Field(fieldIndex).Interface()
			}
			if !rowMatchesExactUniqueValues(rowUniqueValues, exactValues) {
				continue
			}
			parts := make([]string, len(columns))
			for j, column := range columns {
				parts[j] = fmt.Sprintf("%s=%v", column, rowUniqueValues[j])
			}
			primaryKey := make([]any, len(meta.primaryKeyIndex))
			for j, fieldIndex := range meta.primaryKeyIndex {
				primaryKey[j] = row.Field(fieldIndex).Interface()
			}
			detail := fmt.Sprintf("%s (%s)", primaryKeyLabel(primaryKey), strings.Join(parts, ", "))
			if !containsString(details, detail) {
				details = append(details, detail)
			}
		}
		if len(details) == 0 && len(exactValues) > 0 {
			parts := make([]string, 0, len(columns))
			for i, column := range columns {
				if i < len(exactValues) {
					parts = append(parts, fmt.Sprintf("%s=%s", column, exactValues[i]))
				}
			}
			details = append(details, strings.Join(parts, ", "))
		}
		label := "submitted values"
		if len(exactValues) > 0 {
			label = "conflicting value(s)"
		}
		context = fmt.Sprintf("Bulk update conflicts on unique column(s) %s; %s: %s", strings.Join(columns, ", "), label, strings.Join(details, "; "))
	}

	// The response layer combines Message with Error. Keep the diagnostic context
	// in Message only to avoid repeating it in the rendered error string.
	orm.Message = context
	return err
}

func parseUniqueViolationDetails(message string, meta columnMetaData, driver string) ([]string, []string) {
	var columns, values []string
	if match := oracleUniqueColumnsPattern.FindStringSubmatch(message); len(match) == 2 {
		columns = splitUniqueColumns(match[1])
	} else if match := postgresUniquePattern.FindStringSubmatch(message); len(match) == 3 {
		columns = splitUniqueColumns(match[1])
		values = splitUniqueColumns(match[2])
	} else if match := sqliteUniquePattern.FindStringSubmatch(message); len(match) == 2 {
		for _, raw := range strings.Split(match[1], ",") {
			parts := strings.Split(strings.TrimSpace(raw), ".")
			columns = append(columns, strings.Trim(parts[len(parts)-1], "\"'[]`"))
		}
	} else if match := mysqlDuplicatePattern.FindStringSubmatch(message); len(match) == 3 {
		columns = inferUniqueColumns(match[2], meta)
		values = []string{strings.Trim(match[1], `"'`)}
	} else if match := sqlServerDuplicatePattern.FindStringSubmatch(message); len(match) == 2 {
		columns = inferUniqueColumns(message, meta)
		values = splitUniqueColumns(match[1])
	} else if match := mongoDuplicatePattern.FindStringSubmatch(message); len(match) == 2 {
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
	} else if driver == database.SCYLLA && (strings.Contains(strings.ToLower(message), "already exists") || strings.Contains(strings.ToLower(message), "duplicate")) {
		columns = append(columns, meta.primaryKeyColumns...)
	}
	return columns, values
}

func splitUniqueColumns(value string) []string {
	parts := strings.Split(value, ",")
	for i := range parts {
		parts[i] = strings.Trim(strings.TrimSpace(parts[i]), `"'`)
	}
	return parts
}

func inferUniqueColumns(indexName string, meta columnMetaData) []string {
	name := strings.ToLower(indexName)
	var columns []string
	for _, column := range meta.columns {
		if strings.Contains(name, strings.ToLower(column)) {
			columns = append(columns, column)
		}
	}
	return columns
}

func rowMatchesExactUniqueValues(rowValues []any, exactValues []string) bool {
	if len(exactValues) == 0 {
		return true
	}
	if len(rowValues) != len(exactValues) {
		return false
	}
	for i, value := range exactValues {
		if value != "" && !strings.EqualFold(fmt.Sprint(rowValues[i]), value) {
			return false
		}
	}
	return true
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
