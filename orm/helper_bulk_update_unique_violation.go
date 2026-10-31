package orm

import "reflect"

func enrichBulkUpdateUniqueViolation(rows []reflect.Value, columnIndexes []int) (resCount map[string]int, resVal [][]any) {
	counts := make(map[string]int, len(rows))

	rowValues := make([][]any, len(rows))

	for idx, row := range rows {
		values := make([]any, len(columnIndexes))

		for jdx, fieldIndex := range columnIndexes {
			values[jdx] = row.Field(fieldIndex).Interface()
		}

		rowValues[idx] = values

		counts[primaryKeyTuple(values)]++
	}

	return counts, rowValues
}
