package orm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
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

	applied, _, _, err := orm.Database.MapScanCAS(execCtx, orm.DatabaseConfig, queryStr, whereClause.args...)
	if err != nil {
		return orm.setScyllaWriteError("Update", err)
	}

	if !applied {
		return orm.setNotFound("update")
	}

	const (
		note    = "note"
		message = "Data updated successfully"
	)

	orm.Message = message
	orm.RowsAffected = 1
	orm.Result = map[string]any{
		isSuccess: true,
		note:      "CQL execution completed successfully (rows affected count is not supported by Scylla)",
	}

	return nil
}

func (orm *ORM) updateBulkScylla(execCtx context.Context, tableName string, rowsVal []reflect.Value, meta columnMetaData, activeDriver string) error {
	if err := orm.validateTaggedUniqueBulkUpdateValues(rowsVal, meta); err != nil {
		return err
	}

	if err := orm.validateUniqueBulkUpdateKeys(rowsVal, meta); err != nil {
		return err
	}

	// Scylla only permits IN for specific partition-key shapes. Probe each full
	// primary key before issuing any conditional updates so composite keys work too.
	if err := orm.probeScyllaRecordsExist(execCtx, rowsVal, meta, tableName); err != nil {
		return err
	}

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
				query:                     &query,
				primaryKeyIndex:           meta.primaryKeyIndex,
				primaryKeyColumns:         meta.primaryKeyColumns,
				rows:                      []reflect.Value{row},
				driver:                    activeDriver,
				singleRowPrimaryKeyClause: true,
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

		applied, _, _, err := orm.Database.MapScanCAS(execCtx, orm.DatabaseConfig, queryStr, query.args...)
		if err != nil {
			databaseErr := orm.setScyllaWriteError("Update", err)

			return orm.enrichBulkUpdateUniqueViolation(databaseErr, []reflect.Value{row}, meta, activeDriver)
		}

		if applied {
			totalRowsAffected++
		}
	}

	if totalRowsAffected == 0 {
		return orm.setNotFound("update")
	}

	const message = "Data updated successfully"

	orm.Message = message
	orm.RowsAffected = totalRowsAffected
	orm.Result = map[string]any{
		isSuccess:    true,
		rowsAffected: totalRowsAffected,
	}

	return nil
}

func (orm *ORM) probeScyllaRecordsExist(
	execCtx context.Context,
	rowsVal []reflect.Value,
	meta columnMetaData,
	tableName string,
) error {
	var missing []string

	seenMissing := make(map[string]bool)

	for _, row := range rowsVal {
		terms := make([]string, 0, len(meta.primaryKeyIndex))

		args := make([]any, 0, len(meta.primaryKeyIndex))

		for i, fieldIdx := range meta.primaryKeyIndex {
			terms = append(terms, meta.primaryKeyColumns[i]+" = ?")
			args = append(args, row.Field(fieldIdx).Interface())
		}

		query := fmt.Sprintf("SELECT %s FROM %s WHERE %s", strings.Join(meta.primaryKeyColumns, ", "), tableName, strings.Join(terms, " AND "))

		iter, _, _, err := orm.Database.QueryCQL(execCtx, orm.DatabaseConfig, query, args...)
		if err != nil {
			return orm.setScyllaWriteError("Update", err)
		}

		values := make([]any, len(meta.primaryKeyIndex))

		for i := range values {
			values[i] = new(any)
		}

		found := iter.Scan(values...)

		if err := iter.Close(); err != nil {
			return orm.setScyllaWriteError("Update", err)
		}

		if found {
			continue
		}

		key := make([]any, len(meta.primaryKeyIndex))

		for i, idx := range meta.primaryKeyIndex {
			key[i] = row.Field(idx).Interface()
		}

		keyLabel := primaryKeyTuple(key)

		if !seenMissing[keyLabel] {
			missing = append(missing, primaryKeyLabel(key))
			seenMissing[keyLabel] = true
		}
	}

	if len(missing) > 0 {
		return orm.setError("The requested records were not found. "+
			"Check these primary keys: "+strings.Join(missing, ", ")+". Reference: "+
			"https://github.com/elsyahtech/gorest/blob/main/docs/guide/orm.md#shared-database-error-contract",
			errors.New("update: records not found: "+strings.Join(missing, ", ")),
			http.StatusNotFound,
		)
	}

	return nil
}
