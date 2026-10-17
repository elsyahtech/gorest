package orm

import (
	"fmt"
	"net/http"
	"strings"
)

func (orm *ORM) validateDistinctColumns(columns []string) error {
	for _, entry := range columns {
		for _, column := range strings.Split(entry, ",") {
			column = strings.TrimSpace(column)
			parts := strings.Split(column, ".")
			if len(parts) == 0 || len(parts) > 2 {
				return orm.invalidDistinct(column)
			}
			for _, part := range parts {
				if !isSafeOrderByIdentifier(part) {
					return orm.invalidDistinct(column)
				}
			}
		}
	}
	return nil
}

func (orm *ORM) invalidDistinct(column string) error {
	return orm.setError(
		"Distinct() accepts only column names, optionally qualified by a table name.",
		fmt.Errorf("find: invalid Distinct column %q", column),
		http.StatusBadRequest,
	)
}
