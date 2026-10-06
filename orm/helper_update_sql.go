package orm

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
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

	queryStr := buildUpdateStatement(tableName, strings.Join(setClauses, ", "), strings.Join(whereClause.parts, " AND "), retPlan, activeDriver)

	if retPlan != nil {
		scanned, err := orm.queryReturnRows(execCtx, queryStr, whereClause.args, retPlan)
		if err != nil {
			return orm.Error
		}

		return orm.finishReturn(retPlan, scanned, rowsVal)
	}

	result, message, err := orm.Database.ExecSQL(execCtx, queryStr, whereClause.args...)
	if err != nil {
		return orm.setError(message, err)
	}

	var affected int64

	if result != nil {
		if aff, err := result.RowsAffected(); err == nil {
			affected = aff
		}
	}

	orm.RowsAffected = affected
	orm.Result = map[string]any{
		isSuccess:    true,
		rowsAffected: affected,
	}

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
	updatableCols, err := orm.extractUpdatableColumns(meta)
	if err != nil {
		return orm.Error
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

	orm.RowsAffected = totalRowsAffected
	orm.Result = map[string]any{
		isSuccess:    true,
		rowsAffected: totalRowsAffected,
	}

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

		var (
			query *resBuildQueryUpdateSQL
		)

		argCounter := 1

		for _, col := range req.updatableCols {
			query = orm.buildQueryUpdateSQL(chunkRows, req.meta, argCounter, &col, req.activeDriver)
		}

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

		orm.safeWriteString(buildUpdateStatement(
			req.tableName,
			strings.Join(query.clauses, ", "),
			strings.Join(whereClauses, " AND "),
			req.retPlan,
			req.activeDriver,
		))

		queryStr := orm.StringBuilder.String()

		for idx, arg := range query.args {
			query.args[idx] = orm.sanitizeArg(arg)
		}

		if req.retPlan != nil {
			chunkScanned, err := orm.queryReturnRows(req.execCtx, queryStr, query.args, req.retPlan)
			if err != nil {
				return 0, nil, err
			}

			scanned = append(scanned, chunkScanned...)
			totalRowsAffected += int64(len(chunkScanned))

			continue
		}

		result, message, err := orm.Database.ExecSQL(req.execCtx, queryStr, query.args...)
		if err != nil {
			return 0, nil, orm.setError(message, err)
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
		setClauses = make([]string, 0, 1)
		valueArgs  = make([]any, 0, (len(meta.primaryKeyIndex)+1)*len(chunkRows))
	)

	currentArgCounter := argCounter

	orm.safeWriteString(col.name)
	orm.safeWriteString(" = CASE")

	for _, row := range chunkRows {
		orm.safeWriteString(" WHEN ")

		pkConditions := make([]string, 0, len(meta.primaryKeyIndex))

		for pkIdxPos, pkFieldIdx := range meta.primaryKeyIndex {
			placeholder := getPlaceholder(activeDriver, currentArgCounter)

			pkConditions = append(pkConditions, fmt.Sprintf("%s = %s", meta.primaryKeyColumns[pkIdxPos], placeholder))
			valueArgs = append(valueArgs, row.Field(pkFieldIdx).Interface())

			currentArgCounter++
		}

		orm.safeWriteString(strings.Join(pkConditions, " AND "))
		orm.safeWriteString(" THEN ")

		valPlaceholder := getPlaceholder(activeDriver, currentArgCounter)

		orm.safeWriteString(valPlaceholder)

		valueArgs = append(valueArgs, row.Field(col.idx).Interface())

		currentArgCounter++
	}

	orm.safeWriteString(" ELSE ")
	orm.safeWriteString(col.name)
	orm.safeWriteString(" END")

	setClauses = append(setClauses, orm.StringBuilder.String())

	return &resBuildQueryUpdateSQL{
		args:       valueArgs,
		clauses:    setClauses,
		argCounter: currentArgCounter,
	}
} //nolint:revive
