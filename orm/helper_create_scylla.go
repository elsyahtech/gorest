package orm

import (
	"fmt"
	"reflect"
	"strings"
)

func (orm *ORM) buildUpsertCreateScylla(meta columnMetaData, tableName string) (func(rowVal reflect.Value) (string, []any), error) {
	useUpdateStmt := false

	if orm.IsUpsert {
		if err := orm.prepareScyllaUpsert(meta); err != nil {
			return nil, orm.Error
		}

		useUpdateStmt = len(orm.UpsertUpdateCols) > 0
	}

	// 6. Build CQL statement per row (ScyllaDB compatible using ? placeholders)
	buildRow := func(rowVal reflect.Value) (string, []any) {
		if useUpdateStmt {
			return buildScyllaUpsertUpdate(tableName, rowVal, meta, orm.UpsertUpdateCols)
		}

		rowArgs := make([]any, 0, len(meta.columnIndex))
		rowPlaceholders := make([]string, 0, len(meta.columnIndex))

		for _, columnIdx := range meta.columnIndex {
			rowArgs = append(rowArgs, rowVal.Field(columnIdx).Interface())
			rowPlaceholders = append(rowPlaceholders, "?")
		}

		stmt := fmt.Sprintf("INSERT INTO %s (%s) VALUES (%s)", tableName, strings.Join(meta.columns, ", "), strings.Join(rowPlaceholders, ", "))

		return stmt, rowArgs
	}

	return buildRow, nil
}

type resBuildQueryStatement struct {
	queryStr  string
	valueArgs []any
}

func buildQueryStatement(rowsVal []reflect.Value, buildRow func(rowVal reflect.Value) (string, []any)) *resBuildQueryStatement {
	var (
		valueArgs []any
		queryStr  string
	)

	if len(rowsVal) == 1 {
		// Single statement
		queryStr, valueArgs = buildRow(rowsVal[0])
	} else {
		// Batch using ScyllaDB BEGIN BATCH syntax
		batchStatements := make([]string, 0, len(rowsVal))

		for _, rowVal := range rowsVal {
			stmt, rowArgs := buildRow(rowVal)

			valueArgs = append(valueArgs, rowArgs...)
			batchStatements = append(batchStatements, stmt+";")
		}

		queryStr = fmt.Sprintf("BEGIN BATCH\n%s\nAPPLY BATCH;", strings.Join(batchStatements, "\n"))
	}

	return &resBuildQueryStatement{
		valueArgs: valueArgs,
		queryStr:  queryStr,
	}
}

func (orm *ORM) extractResultCreateScylla(rowsVal []reflect.Value, primaryKeyIndex []int) {
	orm.RowsAffected = int64(len(rowsVal))

	if len(primaryKeyIndex) > 0 {
		var allIDs []string

		for _, rowVal := range rowsVal {
			rowPKs := make([]string, 0, len(primaryKeyIndex))

			for _, idx := range primaryKeyIndex {
				fieldVal := rowVal.Field(idx).Interface()
				rowPKs = append(rowPKs, fmt.Sprintf("%v", fieldVal))
			}

			if len(rowPKs) > 0 {
				allIDs = append(allIDs, strings.Join(rowPKs, "-"))
			}
		}

		orm.LastInsertId = strings.Join(allIDs, ", ")
	} else {
		orm.LastInsertId = ""
	}

	if orm.IsUpsert {
		orm.Result = map[string]any{
			isSuccess:    true,
			rowsAffected: orm.RowsAffected,
			upsertedID:   orm.LastInsertId,
		}
	} else {
		orm.Result = map[string]any{
			isSuccess:    true,
			rowsAffected: orm.RowsAffected,
			lastInsertID: orm.LastInsertId,
		}
	}
}
