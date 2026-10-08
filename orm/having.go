package orm

import "strings"

func (orm *ORM) Having(condition string, args ...any) *ORM {
	if condition = strings.TrimSpace(condition); condition != "" {
		orm.HavingClauses = append(orm.HavingClauses, condition)
		orm.HavingArgs = append(orm.HavingArgs, args...)
	}

	return orm
}
