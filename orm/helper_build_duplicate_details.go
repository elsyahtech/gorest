package orm

import (
	"fmt"
	"reflect"
	"strings"
)

func enrichBuildDuplicateDetails(
	rowValues [][]any,
	exactValues []string,
	counts map[string]int,
	columns []string,
	meta columnMetaData,
	rows []reflect.Value,
) []string {
	var details []string

	for idx, values := range rowValues {
		if !rowMatchesExactUniqueValues(values, exactValues) {
			continue
		}

		key := primaryKeyTuple(values)

		if counts[key] < 2 {
			continue
		}

		parts := make([]string, len(columns))

		for jdx, column := range columns {
			parts[jdx] = fmt.Sprintf("%s=%v", column, values[jdx])
		}

		primaryKey := make([]any, len(meta.primaryKeyIndex))

		for jdx, fieldIndex := range meta.primaryKeyIndex {
			primaryKey[jdx] = rows[idx].Field(fieldIndex).Interface()
		}

		detail := fmt.Sprintf("%s (%s)", primaryKeyLabel(primaryKey), strings.Join(parts, ", "))

		if !containsString(details, detail) {
			details = append(details, detail)
		}
	}

	return details
}
