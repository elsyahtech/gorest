package orm

import (
	"fmt"
	"strings"
)

func (orm *ORM) validateColumnNamesUpsert(cols []string, allColFieldIdx map[string]int, contextName string) error {
	for _, col := range cols {
		// Allow the optional "table.column" qualifier by checking
		// the part after the last dot, if that format is used in WHERE/JOIN.
		colToCheck := col

		if idx := strings.LastIndex(col, "."); idx != -1 {
			colToCheck = col[idx+1:]
		}

		if _, ok := allColFieldIdx[colToCheck]; !ok {
			return orm.setError(
				fmt.Sprintf("Ensure the column name '%s' matches a valid field tagged "+
					"with 'gorest' in your struct definition for context '%s'.", col, contextName),
				fmt.Errorf("%s - validateColumnNames: InvalidColumnName: "+
					"invalid column %q in %s — not a recognized gorest tagged column", "create", col, contextName),
			)
		}
	}

	return nil
}
