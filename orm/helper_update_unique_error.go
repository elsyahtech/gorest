package orm

import (
	"fmt"
	"reflect"
	"regexp"
	"strings"
)

var oracleUniqueColumnsPattern = regexp.MustCompile(`(?i)columns\s*\(([^)]*)\)`)

// enrichBulkUpdateUniqueViolation adds the request values for Oracle's
// ORA-00001 column detail, which identifies the unique columns but not the
// offending values.
func (orm *ORM) enrichBulkUpdateUniqueViolation(err error, rows []reflect.Value, meta columnMetaData) error {
	if err == nil {
		return nil
	}

	match := oracleUniqueColumnsPattern.FindStringSubmatch(err.Error())
	if len(match) != 2 {
		return err
	}

	columns := make([]string, 0)
	columnIndexes := make([]int, 0)
	for _, rawColumn := range strings.Split(match[1], ",") {
		column := strings.Trim(strings.TrimSpace(rawColumn), `"`)
		for i, mapped := range meta.columns {
			if strings.EqualFold(mapped, column) {
				columns = append(columns, mapped)
				columnIndexes = append(columnIndexes, meta.columnIndex[i])
				break
			}
		}
	}
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
		// The constraint detail alone cannot identify which of several distinct
		// submitted values collided with a row already stored in the database.
		for _, row := range rows {
			parts := make([]string, len(columns))
			for j, column := range columns {
				parts[j] = fmt.Sprintf("%s=%v", column, row.Field(columnIndexes[j]).Interface())
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
		context = fmt.Sprintf("Bulk update conflicts on unique column(s) %s; submitted values: %s", strings.Join(columns, ", "), strings.Join(details, "; "))
	}

	orm.Message = context
	orm.Error = fmt.Errorf("%s: %w", context, err)
	return orm.Error
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
