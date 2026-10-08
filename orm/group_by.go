package orm

import "strings"

func (orm *ORM) GroupBy(cols ...string) *ORM {
	for _, col := range cols {
		if col = strings.TrimSpace(col); col != "" {
			orm.GroupByClauses = append(orm.GroupByClauses, col)
		}
	}

	return orm
}
