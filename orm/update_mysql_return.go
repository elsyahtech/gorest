package orm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"

	"github.com/elsyahtech/gorest/database"
)

// updateMySQLSingleWithReturn emulates UPDATE ... RETURNING for MySQL by
// locking the matching primary keys, updating the rows, and selecting the
// requested columns before the transaction is committed.
func (orm *ORM) updateMySQLSingleWithReturn(
	execCtx context.Context,
	tableName, setClause string,
	field *resultBuildUpdatePayload,
	whereClause *resBuildSingleWhereClause,
	plan *returnPlan,
	inputRows []reflect.Value,
	meta columnMetaData,
) error {
	if len(meta.primaryKeyColumns) == 0 || len(meta.primaryKeyColumns) != len(meta.primaryKeyIndex) {
		return orm.setError(
			"MySQL Update().Return() requires primary key columns to retrieve the updated record.",
			errors.New("update: MySQL Return requires a complete primary key mapping"),
			http.StatusBadRequest,
		)
	}

	if len(whereClause.args) < len(field.values) {
		return orm.setError(
			"Unable to prepare MySQL Update().Return(); the update and filter arguments do not match.",
			errors.New("update: invalid MySQL return argument layout"),
			http.StatusInternalServerError,
		)
	}

	whereArgs := append([]any(nil), whereClause.args[len(field.values):]...)

	for idx, arg := range whereArgs {
		whereArgs[idx] = orm.sanitizeArg(arg)
	}

	whereSQL := strings.Join(whereClause.parts, " AND ")
	if whereSQL == "" {
		return orm.setError(
			"MySQL Update().Return() requires a WHERE condition.",
			errors.New("update: MySQL Return requires a WHERE condition"),
			http.StatusBadRequest,
		)
	}

	// Use a local transaction when the caller has not already supplied one.
	// Copying the Database wrapper keeps this operation's Tx isolated from
	// other goroutines using the application's shared Database instance.
	execDB := orm.Database

	var transaction *sql.Tx

	ownsTx := execDB.Tx == nil
	if ownsTx {
		var err error

		transaction, err = execDB.SQL.BeginTx(execCtx, nil)
		if err != nil {
			return orm.setError(
				"Ensure the MySQL server is reachable and can start a transaction.",
				fmt.Errorf("update: begin MySQL Return transaction: %w", err),
				http.StatusInternalServerError,
			)
		}

		txDB := *execDB
		txDB.Tx = transaction
		execDB = &txDB
	}

	committed := false

	if ownsTx {
		defer func() {
			if !committed {
				_ = transaction.Rollback()
			}
		}()
	}

	// Lock the rows matched by the UPDATE before changing them. Capturing their
	// keys lets the follow-up SELECT still find them if an updated filter column
	// no longer matches the original WHERE condition.
	keyQuery := fmt.Sprintf(
		"SELECT %s FROM %s WHERE %s FOR UPDATE",
		strings.Join(meta.primaryKeyColumns, ", "), tableName, whereSQL,
	)

	keyRows, _, _, err := execDB.QuerySQL(execCtx, orm.DatabaseConfig, keyQuery, whereArgs...)
	if err != nil {
		return orm.setDatabaseOperationError("Update", err)
	}

	keys, err := scanMySQLReturnKeys(keyRows, len(meta.primaryKeyColumns), meta, inputRows[0].Type())

	//nolint:sqlclosecheck
	closeErr := keyRows.Close()

	if err != nil {
		return orm.setError(
			"Ensure MySQL can read the primary keys of rows matching the update filter.",
			fmt.Errorf("update: read MySQL Return keys: %w", err),
			http.StatusInternalServerError,
		)
	}

	if closeErr != nil {
		return orm.setError(
			"Ensure the MySQL result stream can be closed cleanly.",
			fmt.Errorf("update: close MySQL Return key rows: %w", closeErr),
			http.StatusInternalServerError,
		)
	}

	if len(keys) == 0 {
		return orm.setNotFound("update")
	}

	updateQuery := buildUpdateStatement(tableName, setClause, whereSQL, nil, database.MYSQL)

	updateArgs := append([]any(nil), whereClause.args...)

	for idx, arg := range updateArgs {
		updateArgs[idx] = orm.sanitizeArg(arg)
	}

	if _, _, _, err := execDB.ExecSQL(execCtx, orm.DatabaseConfig, updateQuery, updateArgs...); err != nil {
		return orm.setDatabaseOperationError("Update", err)
	}

	// Keep each SELECT below MySQL's parameter limit, including composite keys.
	chunkSize := 1000
	if keyWidth := len(meta.primaryKeyColumns); keyWidth > 0 && chunkSize*keyWidth > 60000 {
		chunkSize = 60000 / keyWidth
	}

	if chunkSize < 1 {
		chunkSize = 1
	}

	var scanned []reflect.Value

	for start := 0; start < len(keys); start += chunkSize {
		end := start + chunkSize
		if end > len(keys) {
			end = len(keys)
		}

		selectQuery, selectArgs := buildMySQLReturnSelect(tableName, plan.queryCols, meta.primaryKeyColumns, keys[start:end])

		chunk, err := orm.queryReturnRowsWithDatabase(execDB, execCtx, selectQuery, selectArgs, plan, "Update")
		if err != nil {
			return err
		}

		scanned = append(scanned, chunk...)
	}

	if len(scanned) == 0 {
		return orm.setNotFound("update")
	}

	if err := orm.finishReturn(plan, scanned, inputRows); err != nil {
		return err
	}

	orm.LastInsertId = lastInsertIDFromReturn(plan, scanned, inputRows, meta)
	orm.Message = "Data updated successfully"
	orm.HTTPCode = http.StatusOK

	if ownsTx {
		if err := transaction.Commit(); err != nil {
			return orm.setError(
				"Ensure the MySQL transaction can be committed successfully.",
				fmt.Errorf("update: commit MySQL Return transaction: %w", err),
				http.StatusInternalServerError,
			)
		}

		committed = true
	}

	return nil
}

// updateMySQLBulkWithReturn emulates UPDATE ... RETURNING for MySQL in a
// transaction. It locks the input keys before applying each CASE-based bulk
// update, then reads the requested return columns by those keys.
func (orm *ORM) updateMySQLBulkWithReturn(
	execCtx context.Context,
	tableName string,
	rowsVal []reflect.Value,
	meta columnMetaData,
	updatableCols []updatableColumn,
	plan *returnPlan,
) error {
	if len(meta.primaryKeyColumns) == 0 || len(meta.primaryKeyColumns) != len(meta.primaryKeyIndex) {
		return orm.setError(
			"MySQL Update().Return() requires primary key columns to retrieve the updated records.",
			errors.New("update: MySQL Return requires a complete primary key mapping"),
			http.StatusBadRequest,
		)
	}

	// Use a local transaction when the caller has not already supplied one.
	// Copying the wrapper avoids changing the shared Database.Tx field.
	execDB := orm.Database

	var transaction *sql.Tx

	ownsTx := execDB.Tx == nil

	if ownsTx {
		var err error

		transaction, err = execDB.SQL.BeginTx(execCtx, nil)
		if err != nil {
			return orm.setError(
				"Ensure the MySQL server is reachable and can start a transaction.",
				fmt.Errorf("update: begin MySQL Return transaction: %w", err),
				http.StatusInternalServerError,
			)
		}

		txDB := *execDB
		txDB.Tx = transaction
		execDB = &txDB
	}

	committed := false

	if ownsTx {
		defer func() {
			if !committed {
				_ = transaction.Rollback()
			}
		}()
	}

	chunkSize := buildChunkSize(database.MYSQL, len(updatableCols), len(meta.primaryKeyIndex))

	var keys [][]any

	for start := 0; start < len(rowsVal); start += chunkSize {
		end := start + chunkSize
		if end > len(rowsVal) {
			end = len(rowsVal)
		}

		chunkRows := rowsVal[start:end]

		query := &resBuildQueryUpdateSQL{}
		argCounter := 1

		for idx := range updatableCols {
			columnQuery := orm.buildQueryUpdateSQL(chunkRows, &meta, argCounter, &updatableCols[idx], database.MYSQL)
			query.clauses = append(query.clauses, columnQuery.clauses...)
			query.args = append(query.args, columnQuery.args...)
			argCounter = columnQuery.argCounter
		}

		query.argCounter = argCounter

		setArgCount := len(query.args)

		whereClauses, err := orm.buildBulkWhereClause(&reqBuildBulkWhereClause{
			query:             query,
			primaryKeyIndex:   meta.primaryKeyIndex,
			primaryKeyColumns: meta.primaryKeyColumns,
			rows:              chunkRows,
			driver:            database.MYSQL,
		})
		if err != nil {
			return orm.Error
		}

		whereArgs := append([]any(nil), query.args[setArgCount:]...)

		for idx, arg := range whereArgs {
			whereArgs[idx] = orm.sanitizeArg(arg)
		}

		whereSQL := strings.Join(whereClauses, " AND ")

		keyQuery := fmt.Sprintf(
			"SELECT %s FROM %s WHERE %s FOR UPDATE",
			strings.Join(meta.primaryKeyColumns, ", "), tableName, whereSQL,
		)

		keyRows, _, _, err := execDB.QuerySQL(execCtx, orm.DatabaseConfig, keyQuery, whereArgs...)
		if err != nil {
			return orm.setDatabaseOperationError("Update", err)
		}

		chunkKeys, scanErr := scanMySQLReturnKeys(keyRows, len(meta.primaryKeyColumns), meta, rowsVal[0].Type())

		//nolint:sqlclosecheck
		closeErr := keyRows.Close()

		if scanErr != nil {
			return orm.setError(
				"Ensure MySQL can read the primary keys of rows matching the bulk update.",
				fmt.Errorf("update: read MySQL Return keys: %w", scanErr),
				http.StatusInternalServerError,
			)
		}

		if closeErr != nil {
			return orm.setError(
				"Ensure the MySQL result stream can be closed cleanly.",
				fmt.Errorf("update: close MySQL Return key rows: %w", closeErr),
				http.StatusInternalServerError,
			)
		}

		keys = append(keys, chunkKeys...)

		updateQuery := buildUpdateStatement(tableName, strings.Join(query.clauses, ", "), whereSQL, nil, database.MYSQL)
		updateArgs := append([]any(nil), query.args...)

		for idx, arg := range updateArgs {
			updateArgs[idx] = orm.sanitizeArg(arg)
		}

		result, _, _, err := execDB.ExecSQL(execCtx, orm.DatabaseConfig, updateQuery, updateArgs...)
		if err != nil {
			databaseErr := orm.setDatabaseOperationError("Update", err)
			return orm.enrichBulkUpdateUniqueViolation(databaseErr, chunkRows, meta, database.MYSQL)
		}

		_ = result
	}

	if len(keys) == 0 {
		return orm.setNotFound("update")
	}

	chunkSize = 1000
	if keyWidth := len(meta.primaryKeyColumns); keyWidth > 0 && chunkSize*keyWidth > 60000 {
		chunkSize = 60000 / keyWidth
	}

	if chunkSize < 1 {
		chunkSize = 1
	}

	var scanned []reflect.Value

	for start := 0; start < len(keys); start += chunkSize {
		end := start + chunkSize
		if end > len(keys) {
			end = len(keys)
		}

		selectQuery, selectArgs := buildMySQLReturnSelect(tableName, plan.queryCols, meta.primaryKeyColumns, keys[start:end])

		chunk, err := orm.queryReturnRowsWithDatabase(execDB, execCtx, selectQuery, selectArgs, plan, "Update")
		if err != nil {
			return err
		}

		scanned = append(scanned, chunk...)
	}

	if len(scanned) == 0 {
		return orm.setNotFound("update")
	}

	if err := orm.finishReturn(plan, scanned, rowsVal); err != nil {
		return err
	}

	orm.LastInsertId = lastInsertIDFromReturn(plan, scanned, rowsVal, meta)
	// UPDATE ... RETURN semantics count matched rows, including rows whose new
	// values equal their existing values (MySQL may report only changed rows).
	orm.RowsAffected = int64(len(scanned))
	orm.Message = "Data updated successfully"
	orm.HTTPCode = http.StatusOK

	if ownsTx {
		if err := transaction.Commit(); err != nil {
			return orm.setError(
				"Ensure the MySQL transaction can be committed successfully.",
				fmt.Errorf("update: commit MySQL Return transaction: %w", err),
				http.StatusInternalServerError,
			)
		}

		committed = true
	}

	return nil
}

func scanMySQLReturnKeys(rows *sql.Rows, columnCount int, meta columnMetaData, rowType reflect.Type) ([][]any, error) {
	if rows == nil {
		return nil, errors.New("query returned a nil rows handle") //nolint:wrapcheck
	}

	keys := make([][]any, 0)

	for rows.Next() {
		values := make([]any, columnCount)
		destinations := make([]any, columnCount)

		for idx := range values {
			destinations[idx] = &values[idx]
		}

		if err := rows.Scan(destinations...); err != nil {
			return nil, fmt.Errorf("scan MySQL Return keys: %w", err)
		}

		for idx, fieldIndex := range meta.primaryKeyIndex {
			fieldType := rowType.Field(fieldIndex).Type

			for fieldType.Kind() == reflect.Pointer {
				fieldType = fieldType.Elem()
			}

			if fieldType.Kind() == reflect.String {
				if bytes, ok := values[idx].([]byte); ok {
					values[idx] = string(bytes)
				}
			}
		}

		keys = append(keys, values)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("%w", err)
	}

	return keys, nil
}

func buildMySQLReturnSelect(tableName string, returnColumns, primaryKeyColumns []string, keys [][]any) (string, []any) {
	keyPredicates := make([]string, 0, len(keys))
	args := make([]any, 0, len(keys)*len(primaryKeyColumns))

	for _, key := range keys {
		parts := make([]string, 0, len(primaryKeyColumns))

		for idx, column := range primaryKeyColumns {
			parts = append(parts, column+" = ?")
			args = append(args, key[idx])
		}

		keyPredicates = append(keyPredicates, "("+strings.Join(parts, " AND ")+")")
	}

	query := fmt.Sprintf(
		"SELECT %s FROM %s WHERE %s",
		strings.Join(returnColumns, ", "), tableName, strings.Join(keyPredicates, " OR "),
	)

	return query, args
} //nolint:revive
