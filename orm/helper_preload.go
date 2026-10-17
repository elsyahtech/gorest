package orm

func (orm *ORM) validatePreloadCondition(condition string) error {
	return orm.validateWhereClause(condition)
}
