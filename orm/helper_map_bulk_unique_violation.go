package orm

import "strings"

func enrichUniqueViolationData(
	columns []string,
	exactValues []string,
	meta columnMetaData,
) (mappedColumns []string, columnIndexes []int, mappedValues []string) {
	mappedColumns = make([]string, 0, len(columns))
	columnIndexes = make([]int, 0, len(columns))
	mappedValues = make([]string, 0, len(exactValues))

	for columnPosition, column := range columns {
		for idx, mapped := range meta.columns {
			if !strings.EqualFold(mapped, column) {
				continue
			}

			mappedColumns = append(mappedColumns, mapped)
			columnIndexes = append(columnIndexes, meta.columnIndex[idx])

			if columnPosition < len(exactValues) {
				mappedValues = append(mappedValues, exactValues[columnPosition])
			}

			break
		}
	}

	return mappedColumns, columnIndexes, mappedValues
}
