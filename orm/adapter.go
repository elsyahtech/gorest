package orm

func (orm *ORM) Create(data any) *ORM {
	return orm.execOrm(data, "Create", orm.createSQL, orm.createMongo, orm.createScylla)
}

func (orm *ORM) Update(data any) *ORM {
	return orm.execOrm(data, "Update", orm.updateSQL, orm.updateMongo, orm.updateScylla)
}

func (orm *ORM) Delete(data any) *ORM {
	return orm.execOrm(data, "Delete", orm.deleteSQL, orm.deleteMongo, orm.deleteScylla)
}

func (orm *ORM) Find(data any) *ORM {
	return orm.execOrm(data, "Find", orm.findSQL, orm.findMongo, orm.findScylla)
}

func (orm *ORM) Select(cols ...string) *ORM {
	orm.SelectedCols = cols

	return orm
}

func (orm *ORM) Where(query string, args ...any) *ORM {
	orm.WhereClauses = append(orm.WhereClauses, query)

	if len(args) > 0 {
		orm.WhereArgs = append(orm.WhereArgs, args...)
	}

	return orm
}

func (orm *ORM) AllowFiltering() *ORM {
	orm.AllowFilteringFlag = true

	return orm
}

func (orm *ORM) Distinct() *ORM {
	orm.IsDistinct = true

	return orm
}

func (orm *ORM) Join(clause string) *ORM {
	orm.JoinClauses = append(orm.JoinClauses, clause)

	return orm
}

func (orm *ORM) Limit(limit int) *ORM {
	orm.LimitVal = limit

	return orm
}

func (orm *ORM) Offset(offset int) *ORM {
	orm.OffsetVal = offset

	return orm
}
