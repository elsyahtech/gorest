package orm

import "strings"

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
