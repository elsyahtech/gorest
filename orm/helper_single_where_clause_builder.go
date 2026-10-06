package orm

import (
	"errors"
	"fmt"
	"reflect"
)

type resBuildSingleWhereClause struct {
	parts []string
	args  []any
}

func (orm *ORM) buildSingleWhereClause(
	row *reflect.Value,
	field *resultBuildUpdatePayload,
	meta *columnMetaData,
	activeDriver string,
) (*resBuildSingleWhereClause, error) {
	var whereParts []string

	valueArgs := field.values
	argCounter := len(valueArgs) + 1

	if len(orm.WhereClauses) > 0 {
		whereArgIndex := 0

		for _, clause := range orm.WhereClauses {
			placeholderCount := countActivePlaceholders(clause, activeDriver)

			if whereArgIndex+placeholderCount > len(orm.WhereArgs) {
				return nil, orm.setError(
					"Ensure the number of placeholders in .Where() matches the number of arguments provided.",
					errors.New("update: mismatched where clause placeholders and arguments"),
				)
			}

			normalized, nextCounter := normalizeWherePlaceholders(clause, activeDriver, argCounter)

			argCounter = nextCounter

			whereParts = append(whereParts, normalized)
			valueArgs = append(valueArgs, orm.WhereArgs[whereArgIndex:whereArgIndex+placeholderCount]...)

			whereArgIndex += placeholderCount
		}
	} else {
		for pkIdxPos, pkFieldIdx := range meta.primaryKeyIndex {
			placeholder := getPlaceholder(activeDriver, argCounter)

			whereParts = append(whereParts, fmt.Sprintf("%s = %s", meta.primaryKeyColumns[pkIdxPos], placeholder))
			valueArgs = append(valueArgs, row.Field(pkFieldIdx).Interface())

			argCounter++
		}
	}

	if len(whereParts) == 0 {
		return nil, orm.setError(
			"ensure that your update query has a WHERE clause to prevent accidental mass modifications.",
			errors.New("update: update without WHERE clause is forbidden"),
		)
	}

	return &resBuildSingleWhereClause{
		parts: whereParts,
		args:  valueArgs,
	}, nil
}
