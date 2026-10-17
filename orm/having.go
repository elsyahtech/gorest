package orm

import "strings"

func (orm *ORM) Having(condition string, args ...any) *ORM {
	condition = strings.TrimSpace(condition)
	if err := orm.validateHavingClause(condition); err != nil {
		orm.havingInvalid = true
		return orm
	}

	orm.HavingClauses = append(orm.HavingClauses, condition)
	orm.HavingArgs = append(orm.HavingArgs, args...)

	return orm
}
