package orm

import "strings"

type resultBuildSelectedColumnsByTable struct {
	table    map[string][]string
	isSelect bool
}

func buildSelectedColumnsByTable(selectedCols []string, parentTable string) *resultBuildSelectedColumnsByTable {
	byTable := make(map[string][]string)
	isSelect := false

	for _, col := range selectedCols {
		for _, raw := range strings.Split(col, ",") {
			rawTrimed := strings.TrimSpace(raw)
			if rawTrimed == "" {
				continue
			}

			table := parentTable
			column := rawTrimed

			if tbl, col, ok := strings.Cut(rawTrimed, "."); ok {
				table = tbl
				column = col
			}

			if column == "*" {
				isSelect = true

				continue
			}

			key := strings.ToLower(table)
			byTable[key] = append(byTable[key], column)
		}
	}

	return &resultBuildSelectedColumnsByTable{
		table:    byTable,
		isSelect: isSelect,
	}
}
