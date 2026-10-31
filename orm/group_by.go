package orm

import "strings"

func (orm *ORM) GroupBy(cols ...string) *ORM {
	for _, col := range cols {
		if col = strings.TrimSpace(col); col != "" {
			if err := orm.validateGroupBySyntax(col); err != nil {
				orm.groupByInvalid = true
				return orm
			}

			orm.GroupByClauses = append(orm.GroupByClauses, col)
		}
	}

	return orm
}
