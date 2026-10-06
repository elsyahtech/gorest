package orm

import (
	"errors"
	"reflect"

	"go.mongodb.org/mongo-driver/v2/bson"
)

func (orm *ORM) buildBulkMongoFilter(row reflect.Value, meta *columnMetaData) (bson.M, error) {
	if len(meta.primaryKeyIndex) == 0 {
		return nil, orm.setError(
			"Ensure your struct has a primary_key tag designated for bulk updates.",
			errors.New("update: missing primary key for bulk update"),
		)
	}

	pkFilter := bson.M{}
	for pkIdxPos, pkFieldIdx := range meta.primaryKeyIndex {
		pkFilter[meta.primaryKeyColumns[pkIdxPos]] = row.Field(pkFieldIdx).Interface()
	}

	if len(orm.WhereClauses) == 0 {
		return pkFilter, nil
	}

	customFilter, message, err := convertWhereToMongoFilter(orm.WhereClauses, orm.WhereArgs)
	if err != nil {
		return nil, orm.setError(message, err)
	}

	if len(pkFilter) == 0 {
		return nil, orm.setError(
			"ensure that your update query has a WHERE clause or primary key.",
			errors.New("update: update without filter/WHERE clause is forbidden"),
		)
	}

	return bson.M{"$and": []bson.M{pkFilter, customFilter}}, nil
}

func (orm *ORM) buildSingleMongoFilter(row reflect.Value, meta *columnMetaData) (bson.M, error) {
	if len(orm.WhereClauses) > 0 {
		customFilter, message, err := convertWhereToMongoFilter(orm.WhereClauses, orm.WhereArgs)
		if err != nil {
			return nil, orm.setError(message, err)
		}

		return customFilter, nil
	}

	pkFilter := bson.M{}
	for pkIdxPos, pkFieldIdx := range meta.primaryKeyIndex {
		pkFilter[meta.primaryKeyColumns[pkIdxPos]] = row.Field(pkFieldIdx).Interface()
	}

	if len(pkFilter) == 0 {
		return nil, orm.setError(
			"ensure that your update query has a WHERE clause or primary key.",
			errors.New("update: update without filter/WHERE clause is forbidden"),
		)
	}

	return pkFilter, nil
}
