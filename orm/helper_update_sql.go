package orm

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/elsyahtech/gorest/database"
)

func (orm *ORM) updateSingleSQL(
	execCtx context.Context,
	tableName string,
	rowsVal []reflect.Value,
	meta columnMetaData,
	activeDriver string,
	retPlan *returnPlan,
) error {
	row := rowsVal[0]
	field := buildUpdatePayload(row, meta, activeDriver, 1)
	setClauses := field.setClauses

	if len(setClauses) == 0 {
		return orm.setError(
			"Ensure that there are columns to update. Specify columns using .Select() or ensure your struct has non-primary key fields.",
			errors.New("update: no columns to update"),
		)
	}

	// Build the WHERE clause.
	whereClause, err := orm.buildSingleWhereClause(&row, field, &meta, activeDriver)
	if err != nil {
		return orm.Error
	}

	whereClause.args = orm.sanitizeArgs(whereClause.args)
	if retPlan != nil && activeDriver == database.MYSQL {
		return orm.updateMySQLSingleWithReturn(
			execCtx,
			tableName,
			strings.Join(setClauses, ", "),
			field,
			whereClause,
			retPlan,
			rowsVal,
			meta,
		)
	}

	queryStr := buildUpdateStatement(tableName, strings.Join(setClauses, ", "), strings.Join(whereClause.parts, " AND "), retPlan, activeDriver)

	if retPlan != nil {
		scanned, err := orm.queryReturnRows(execCtx, queryStr, whereClause.args, retPlan, "Update")
		if err != nil {
			return orm.Error
		}

		if len(scanned) == 0 {
			return orm.setNotFound("update")
		}

		if err := orm.finishReturn(retPlan, scanned, rowsVal); err != nil {
			return err
		}

		orm.Message = "Data updated successfully"
		orm.HTTPCode = http.StatusOK

		return nil
	}

	result, _, _, err := orm.Database.ExecSQL(execCtx, orm.DatabaseConfig, queryStr, whereClause.args...)
	if err != nil {
		return orm.setDatabaseOperationError("Update", err)
	}

	var affected int64

	if result != nil {
		if aff, err := result.RowsAffected(); err == nil {
			affected = aff
		}
	}

	if affected == 0 {
		return orm.setNotFound("update")
	}

	orm.RowsAffected = affected
	orm.Result = map[string]any{
		isSuccess:    true,
		rowsAffected: affected,
	}

	orm.Message = "Data updated successfully"

	return nil
}

func (orm *ORM) updateBulkSQL(
	execCtx context.Context,
	tableName string,
	rowsVal []reflect.Value,
	meta columnMetaData,
	activeDriver string,
	retPlan *returnPlan,
) error {
	if err := orm.validateTaggedUniqueBulkUpdateValues(rowsVal, meta); err != nil {
		return err
	}

	if err := orm.validateBulkUpdateRowsSQL(execCtx, tableName, rowsVal, meta, activeDriver); err != nil {
		return err
	}

	updatableCols, err := orm.extractUpdatableColumns(meta)
	if err != nil {
		return orm.Error
	}

	if retPlan != nil && activeDriver == database.MYSQL {
		return orm.updateMySQLBulkWithReturn(execCtx, tableName, rowsVal, meta, updatableCols, retPlan)
	}

	chunkSize := buildChunkSize(activeDriver, len(updatableCols), len(meta.primaryKeyIndex))

	totalRowsAffected, scanned, err := orm.execBulkUpdateSQL(
		&structExecBulkUpdateSQL{
			execCtx:       execCtx,
			tableName:     tableName,
			rowsVal:       rowsVal,
			chunkSize:     chunkSize,
			updatableCols: updatableCols,
			meta:          &meta,
			activeDriver:  activeDriver,
			retPlan:       retPlan,
		},
	)
	if err != nil {
		return orm.Error
	}

	if totalRowsAffected == 0 {
		return orm.setNotFound("update")
	}

	orm.RowsAffected = totalRowsAffected
	orm.Result = map[string]any{
		isSuccess:    true,
		rowsAffected: totalRowsAffected,
	}

	orm.Message = "Data updated successfully"

	if retPlan != nil {
		return orm.finishReturn(retPlan, scanned, rowsVal)
	}

	return nil
}

type updatableColumn struct {
	name string
	idx  int
}

func (orm *ORM) extractUpdatableColumns(meta columnMetaData) ([]updatableColumn, error) {
	var updatableCols []updatableColumn

	for colIndex, colIdx := range meta.columnIndex {
		isPK := false

		for _, pkFieldIdx := range meta.primaryKeyIndex {
			if pkFieldIdx == colIdx {
				isPK = true

				break
			}
		}

		if !isPK {
			updatableCols = append(updatableCols, updatableColumn{
				name: meta.columns[colIndex],
				idx:  colIdx,
			})
		}
	}

	if len(updatableCols) == 0 {
		return nil, orm.setError(
			"Ensure that there are columns to update. Specify columns using .Select() or ensure your struct has non-primary key fields.",
			errors.New("update: no columns to update"),
		)
	}

	return updatableCols, nil
}

type structExecBulkUpdateSQL struct {
	execCtx       context.Context
	meta          *columnMetaData
	retPlan       *returnPlan
	tableName     string
	activeDriver  string
	rowsVal       []reflect.Value
	updatableCols []updatableColumn
	chunkSize     int
}

func (orm *ORM) execBulkUpdateSQL(req *structExecBulkUpdateSQL) (int64, []reflect.Value, error) {
	var (
		totalRowsAffected int64
		scanned           []reflect.Value
	)

	for idx := 0; idx < len(req.rowsVal); idx += req.chunkSize {
		end := idx + req.chunkSize

		if end > len(req.rowsVal) {
			end = len(req.rowsVal)
		}

		chunkRows := req.rowsVal[idx:end]

		query := &resBuildQueryUpdateSQL{}
		argCounter := 1

		for _, col := range req.updatableCols {
			columnQuery := orm.buildQueryUpdateSQL(chunkRows, req.meta, argCounter, &col, req.activeDriver)
			query.clauses = append(query.clauses, columnQuery.clauses...)
			query.args = append(query.args, columnQuery.args...)
			argCounter = columnQuery.argCounter
		}

		query.argCounter = argCounter

		// Build the array WHERE clause
		whereClauses, err := orm.buildBulkWhereClause(
			&reqBuildBulkWhereClause{
				query:             query,
				primaryKeyIndex:   req.meta.primaryKeyIndex,
				primaryKeyColumns: req.meta.primaryKeyColumns,
				rows:              chunkRows,
				driver:            req.activeDriver,
			},
		)
		if err != nil {
			return 0, nil, orm.Error
		}

		orm.StringBuilder.Reset()
		orm.safeWriteString(buildUpdateStatement(
			req.tableName,
			strings.Join(query.clauses, ", "),
			strings.Join(whereClauses, " AND "),
			req.retPlan,
			req.activeDriver,
		))

		queryStr := orm.StringBuilder.String()

		query.args = orm.sanitizeArgs(query.args)

		if req.retPlan != nil {
			chunkScanned, err := orm.queryReturnRows(req.execCtx, queryStr, query.args, req.retPlan, "Update")
			if err != nil {
				return 0, nil, err
			}

			scanned = append(scanned, chunkScanned...)
			totalRowsAffected += int64(len(chunkScanned))

			continue
		}

		result, _, _, err := orm.Database.ExecSQL(req.execCtx, orm.DatabaseConfig, queryStr, query.args...)
		if err != nil {
			databaseErr := orm.setDatabaseOperationError("Update", err)
			return 0, nil, orm.enrichBulkUpdateUniqueViolation(databaseErr, chunkRows, *req.meta, req.activeDriver)
		}

		if result == nil {
			continue
		}

		if affected, err := result.RowsAffected(); err == nil {
			totalRowsAffected += affected
		}
	}

	return totalRowsAffected, scanned, nil
}

type resBuildQueryUpdateSQL struct {
	args       []any
	clauses    []string
	argCounter int
}

func (orm *ORM) buildQueryUpdateSQL(
	chunkRows []reflect.Value,
	meta *columnMetaData,
	argCounter int,
	col *updatableColumn,
	activeDriver string,
) *resBuildQueryUpdateSQL {
	var (
		valueArgs = make([]any, 0, (len(meta.primaryKeyIndex)+1)*len(chunkRows))
		clause    strings.Builder
	)

	currentArgCounter := argCounter

	clause.WriteString(col.name)
	clause.WriteString(" = CASE")

	for _, row := range chunkRows {
		clause.WriteString(" WHEN ")

		pkConditions := make([]string, 0, len(meta.primaryKeyIndex))

		for pkIdxPos, pkFieldIdx := range meta.primaryKeyIndex {
			placeholder := getPlaceholder(activeDriver, currentArgCounter)

			pkConditions = append(pkConditions, fmt.Sprintf("%s = %s", meta.primaryKeyColumns[pkIdxPos], placeholder))
			valueArgs = append(valueArgs, row.Field(pkFieldIdx).Interface())

			currentArgCounter++
		}

		clause.WriteString(strings.Join(pkConditions, " AND "))
		clause.WriteString(" THEN ")

		valPlaceholder := getPlaceholder(activeDriver, currentArgCounter)

		clause.WriteString(valPlaceholder)

		valueArgs = append(valueArgs, row.Field(col.idx).Interface())

		currentArgCounter++
	}

	clause.WriteString(" ELSE ")
	clause.WriteString(col.name)
	clause.WriteString(" END")

	return &resBuildQueryUpdateSQL{
		args:       valueArgs,
		clauses:    []string{clause.String()},
		argCounter: currentArgCounter,
	}
} //nolint:revive
