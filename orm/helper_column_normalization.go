package orm

import "strings"

type strucTextractColumnMetaData struct {
	columnsValue           []string
	primaryKeyColumnsValue []string
	normalizedSelectedCols []string
	columnIndexValue       []int
	primaryKeyIndexValue   []int
}

func (orm *ORM) normalizedSelectedCols(req *strucTextractColumnMetaData) *strucTextractColumnMetaData {
	data := req

	for _, col := range orm.SelectedCols {
		for _, part := range strings.Split(col, ",") {
			trimmed := strings.TrimSpace(part)

			if trimmed != "" {
				data.normalizedSelectedCols = append(data.normalizedSelectedCols, trimmed)
			}
		}
	}

	return data
}
