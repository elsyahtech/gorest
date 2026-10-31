package orm

import "strings"

func (orm *ORM) Having(conditionParam string, args ...any) *ORM {
	condition := strings.TrimSpace(conditionParam)

	if err := orm.validateHavingClause(condition); err != nil {
		orm.havingInvalid = true
		return orm
	}

	orm.HavingClauses = append(orm.HavingClauses, condition)
	orm.HavingArgs = append(orm.HavingArgs, args...)

	return orm
}
