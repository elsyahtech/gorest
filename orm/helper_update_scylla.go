package orm

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
)

func (orm *ORM) updateSingleScylla(execCtx context.Context, tableName string, rowsVal []reflect.Value, meta columnMetaData, activeDriver string) error {
	row := rowsVal[0]
	field := buildUpdatePayload(row, meta, activeDriver, 1)
	setClauses := field.setClauses

	if len(setClauses) == 0 {
		return orm.setError(
			"Ensure that there are columns to update. Specify columns using .Select() or ensure your struct has non-primary key fields.",
			errors.New("update: no columns to update"),
		)
	}

	whereClause, err := orm.buildSingleWhereClause(&row, field, &meta, activeDriver)
	if err != nil {
		return orm.Error
	}

	queryStr := fmt.Sprintf("UPDATE %s SET %s WHERE %s IF EXISTS",
		strings.TrimSpace(tableName),
		strings.Join(setClauses, ", "),
		strings.Join(whereClause.parts, " AND "),
	)

	applied, troubleshootMsg, httpCode, err := orm.Database.ScanCQL(execCtx, queryStr, whereClause.args...)
	if err != nil {
		return orm.setError(troubleshootMsg, err, httpCode)
	}

	if !applied {
		return orm.setNotFound("update")
	}

	const note = "note"

	orm.RowsAffected = 1
	orm.Result = map[string]any{
		isSuccess: true,
		note:      "CQL execution completed successfully (rows affected count is not supported by Scylla)",
	}

	return nil
}

func (orm *ORM) updateBulkScylla(execCtx context.Context, tableName string, rowsVal []reflect.Value, meta columnMetaData, activeDriver string) error {
	var (
		totalRowsAffected int64
		query             resBuildQueryUpdateSQL
	)

	for _, row := range rowsVal {
		field := buildUpdatePayload(row, meta, activeDriver, 1)

		if len(field.setClauses) == 0 {
			return orm.setError(
				"Ensure that there are columns to update. Specify columns using .Select() or ensure your struct has non-primary key fields.",
				errors.New("update: no columns to update"),
			)
		}

		query.clauses = field.setClauses
		query.args = field.values
		query.argCounter = len(query.args) + 1

		whereClauses, err := orm.buildBulkWhereClause(
			&reqBuildBulkWhereClause{
				query:             &query,
				primaryKeyIndex:   meta.primaryKeyIndex,
				primaryKeyColumns: meta.primaryKeyColumns,
				rows:              []reflect.Value{row},
				driver:            activeDriver,
			},
		)
		if err != nil {
			return orm.Error
		}

		queryStr := fmt.Sprintf("UPDATE %s SET %s WHERE %s IF EXISTS",
			strings.TrimSpace(tableName),
			strings.Join(query.clauses, ", "),
			strings.Join(whereClauses, " AND "),
		)

		applied, message, httpCode, err := orm.Database.ScanCQL(execCtx, queryStr, query.args...)
		if err != nil {
			return orm.setError(message, err, httpCode)
		}
		if applied {
			totalRowsAffected++
		}
	}
	if totalRowsAffected == 0 {
		return orm.setNotFound("update")
	}

	orm.RowsAffected = totalRowsAffected
	orm.Result = map[string]any{
		isSuccess:    true,
		rowsAffected: totalRowsAffected,
	}

	return nil
}
