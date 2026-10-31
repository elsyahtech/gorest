package orm

import (
	"fmt"
	"reflect"
	"strings"
)

func buildViolationDetails(
	rows []reflect.Value,
	columnIndexes []int,
	exactValues []string,
	columns []string,
	meta columnMetaData,
	detailsParam []string,
) []string {
	details := detailsParam

	for _, row := range rows {
		rowUniqueValues := make([]any, len(columnIndexes))

		for jdx, fieldIndex := range columnIndexes {
			rowUniqueValues[jdx] = row.Field(fieldIndex).Interface()
		}

		if !rowMatchesExactUniqueValues(rowUniqueValues, exactValues) {
			continue
		}

		parts := make([]string, len(columns))

		for jdx, column := range columns {
			parts[jdx] = fmt.Sprintf("%s=%v", column, rowUniqueValues[jdx])
		}

		primaryKey := make([]any, len(meta.primaryKeyIndex))

		for jdx, fieldIndex := range meta.primaryKeyIndex {
			primaryKey[jdx] = row.Field(fieldIndex).Interface()
		}

		detail := fmt.Sprintf("%s (%s)", primaryKeyLabel(primaryKey), strings.Join(parts, ", "))

		if !containsString(details, detail) {
			details = append(details, detail)
		}
	}

	if len(details) == 0 && len(exactValues) > 0 {
		parts := make([]string, 0, len(columns))

		for idx, column := range columns {
			if idx < len(exactValues) {
				parts = append(parts, fmt.Sprintf("%s=%s", column, exactValues[idx]))
			}
		}

		details = append(details, strings.Join(parts, ", "))
	}

	return details
}
