package orm

// Preload specifies relations to be loaded eagerly along with the main query.
func (orm *ORM) Preload(relation string, args ...any) *ORM {
	if orm.Error != nil {
		return orm
	}

	if orm.Preloads == nil {
		orm.Preloads = make(map[string][]any)
	}

	orm.Preloads[relation] = args

	orm.PreloadClauses = append(orm.PreloadClauses, relation)

	return orm
}
