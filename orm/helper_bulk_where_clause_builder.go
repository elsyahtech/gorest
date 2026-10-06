package orm

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

type reqBuildBulkWhereClause struct {
	query             *resBuildQueryUpdateSQL
	driver            string
	primaryKeyIndex   []int
	primaryKeyColumns []string
	rows              []reflect.Value
}

func (orm *ORM) buildBulkWhereClause(req *reqBuildBulkWhereClause) ([]string, error) {
	if len(req.primaryKeyIndex) == 0 {
		return nil, orm.setError(
			"Ensure your struct has a primary_key tag designated for bulk updates.",
			errors.New("update: missing primary key for bulk update"),
		)
	}

	var (
		whereClauses        []string
		primaryKeyInClauses []string
	)

	for _, row := range req.rows {
		var pkConds []string

		for pkIdxPos, pkFieldIdx := range req.primaryKeyIndex {
			placeholder := getPlaceholder(req.driver, req.query.argCounter)

			pkConds = append(pkConds, fmt.Sprintf("%s = %s", req.primaryKeyColumns[pkIdxPos], placeholder))
			req.query.args = append(req.query.args, row.Field(pkFieldIdx).Interface())

			req.query.argCounter++
		}

		primaryKeyInClauses = append(primaryKeyInClauses, "("+strings.Join(pkConds, " AND ")+")")
	}

	whereClauses = append(whereClauses, "("+strings.Join(primaryKeyInClauses, " OR ")+")")

	if len(orm.WhereClauses) > 0 {
		whereArgIndex := 0

		for _, clause := range orm.WhereClauses {
			placeholderCount := countActivePlaceholders(clause, req.driver)

			if whereArgIndex+placeholderCount > len(orm.WhereArgs) {
				return nil, orm.setError(
					"Ensure the number of placeholders in .Where() matches the number of arguments provided.",
					errors.New("update: mismatched where clause placeholders and arguments"),
				)
			}

			normalized, nextCounter := normalizeWherePlaceholders(clause, req.driver, req.query.argCounter)

			req.query.argCounter = nextCounter

			whereClauses = append(whereClauses, normalized)
			req.query.args = append(req.query.args, orm.WhereArgs[whereArgIndex:whereArgIndex+placeholderCount]...)

			whereArgIndex += placeholderCount
		}
	}

	return whereClauses, nil
}
