package orm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"reflect"
	"strings"

	"github.com/elsyahtech/gorest/database"
)

type structUpdateMySQLSingleWithReturn struct {
	execCtx     context.Context
	tableName   string
	setClause   string
	field       *resultBuildUpdatePayload
	whereClause *resBuildSingleWhereClause
	plan        *returnPlan
	inputRows   []reflect.Value
	meta        columnMetaData
}

// updateMySQLSingleWithReturn emulates UPDATE ... RETURNING for MySQL by
// locking the matching primary keys, updating the rows, and selecting the
// requested columns before the transaction is committed.
func (orm *ORM) updateMySQLSingleWithReturn(req *structUpdateMySQLSingleWithReturn) error {
	if err := orm.validateMySQLSingleReturnInput(req.field, req.whereClause, req.meta); err != nil {
		return err
	}

	whereSQL, whereArgs := orm.mysqlSingleReturnWhere(req.field, req.whereClause)
	if whereSQL == "" {
		return orm.setError(
			"MySQL Update().Return() requires a WHERE condition.",
			errors.New("update: MySQL Return requires a WHERE condition"),
			http.StatusBadRequest,
		)
	}

	execDB, transaction, ownsTx, err := orm.beginMySQLReturnTransaction(req.execCtx)
	if err != nil {
		return err
	}

	committed := false

	if ownsTx {
		defer rollbackMySQLReturnTransaction(transaction, &committed)
	}

	keys, err := orm.lockMySQLSingleReturnKeys(req.execCtx, execDB, req.tableName, whereSQL, whereArgs, req.meta, req.inputRows[0].Type())
	if err != nil {
		return err
	}

	if len(keys) == 0 {
		return orm.setNotFound("update")
	}

	if err := orm.applyMySQLSingleReturnUpdate(req.execCtx, execDB, req.tableName, req.setClause, whereSQL, req.whereClause.args); err != nil {
		return err
	}

	scanned, err := orm.selectMySQLBulkReturnRows(req.execCtx, execDB, req.tableName, keys, req.meta, req.plan)
	if err != nil {
		return err
	}

	if len(scanned) == 0 {
		return orm.setNotFound("update")
	}

	if err := orm.finishMySQLSingleReturn(req.plan, scanned, req.inputRows, req.meta); err != nil {
		return err
	}

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

func (orm *ORM) validateMySQLSingleReturnInput(
	field *resultBuildUpdatePayload,
	whereClause *resBuildSingleWhereClause,
	meta columnMetaData,
) error {
	if len(meta.primaryKeyColumns) == 0 || len(meta.primaryKeyColumns) != len(meta.primaryKeyIndex) {
		return orm.setError(
			"MySQL Update().Return() requires primary key columns to retrieve the updated record.",
			errors.New("update: MySQL Return requires a complete primary key mapping"),
			http.StatusBadRequest,
		)
	}

	if len(whereClause.args) >= len(field.values) {
		return nil
	}

	return orm.setError(
		"Unable to prepare MySQL Update().Return(); the update and filter arguments do not match.",
		errors.New("update: invalid MySQL return argument layout"),
		http.StatusInternalServerError,
	)
}

func (orm *ORM) mysqlSingleReturnWhere(
	field *resultBuildUpdatePayload,
	whereClause *resBuildSingleWhereClause,
) (string, []any) {
	args := sanitizeMySQLReturnArgs(orm, whereClause.args[len(field.values):])

	return strings.Join(whereClause.parts, " AND "), args
}

func (orm *ORM) lockMySQLSingleReturnKeys(
	execCtx context.Context,
	execDB *database.Database,
	tableName, whereSQL string,
	whereArgs []any,
	meta columnMetaData,
	rowType reflect.Type,
) ([][]any, error) {
	return orm.queryMySQLReturnKeys(&structMySQLReturnKeyQuery{
		execCtx:       execCtx,
		execDB:        execDB,
		tableName:     tableName,
		whereSQL:      whereSQL,
		whereArgs:     whereArgs,
		meta:          meta,
		rowType:       rowType,
		matchedRecord: "the update filter",
	})
}

func (orm *ORM) applyMySQLSingleReturnUpdate(
	execCtx context.Context,
	execDB *database.Database,
	tableName, setClause, whereSQL string,
	args []any,
) error {
	updateQuery := buildUpdateStatement(tableName, setClause, whereSQL, nil, database.MYSQL)
	updateArgs := sanitizeMySQLReturnArgs(orm, args)

	if _, _, _, err := execDB.ExecSQL(execCtx, orm.DatabaseConfig, updateQuery, updateArgs...); err != nil {
		return orm.setDatabaseOperationError("Update", err)
	}

	return nil
}

func (orm *ORM) finishMySQLSingleReturn(
	plan *returnPlan,
	scanned, inputRows []reflect.Value,
	meta columnMetaData,
) error {
	if err := orm.finishReturn(plan, scanned, inputRows); err != nil {
		return err
	}

	const message = "Data updated successfully"

	orm.LastInsertId = lastInsertIDFromReturn(plan, scanned, inputRows, meta)
	orm.Message = message
	orm.HTTPCode = http.StatusOK

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
	if err := orm.validateMySQLBulkReturnKeys(meta); err != nil {
		return err
	}

	execDB, transaction, ownsTx, err := orm.beginMySQLReturnTransaction(execCtx)
	if err != nil {
		return err
	}

	committed := false

	if ownsTx {
		defer rollbackMySQLReturnTransaction(transaction, &committed)
	}

	keys, err := orm.updateMySQLBulkChunks(execCtx, execDB, tableName, rowsVal, meta, updatableCols)
	if err != nil {
		return err
	}

	if len(keys) == 0 {
		return orm.setNotFound("update")
	}

	scanned, err := orm.selectMySQLBulkReturnRows(execCtx, execDB, tableName, keys, meta, plan)
	if err != nil {
		return err
	}

	if len(scanned) == 0 {
		return orm.setNotFound("update")
	}

	if err := orm.finishMySQLBulkReturn(plan, scanned, rowsVal, meta); err != nil {
		return err
	}

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

func (orm *ORM) validateMySQLBulkReturnKeys(meta columnMetaData) error {
	if len(meta.primaryKeyColumns) > 0 && len(meta.primaryKeyColumns) == len(meta.primaryKeyIndex) {
		return nil
	}

	return orm.setError(
		"MySQL Update().Return() requires primary key columns to retrieve the updated records.",
		errors.New("update: MySQL Return requires a complete primary key mapping"),
		http.StatusBadRequest,
	)
}

func (orm *ORM) beginMySQLReturnTransaction(execCtx context.Context) (*database.Database, *sql.Tx, bool, error) {
	execDB := orm.Database

	if execDB.Tx != nil {
		return execDB, nil, false, nil
	}

	transaction, err := execDB.SQL.BeginTx(execCtx, nil)
	if err != nil {
		return nil, nil, false, orm.setError(
			"Ensure the MySQL server is reachable and can start a transaction.",
			fmt.Errorf("update: begin MySQL Return transaction: %w", err),
			http.StatusInternalServerError,
		)
	}

	txDB := *execDB
	txDB.Tx = transaction

	return &txDB, transaction, true, nil
}

func rollbackMySQLReturnTransaction(transaction *sql.Tx, committed *bool) {
	if *committed {
		return
	}

	if err := transaction.Rollback(); err != nil {
		log.Print(err)
	}
}

func (orm *ORM) updateMySQLBulkChunks(
	execCtx context.Context,
	execDB *database.Database,
	tableName string,
	rowsVal []reflect.Value,
	meta columnMetaData,
	updatableCols []updatableColumn,
) ([][]any, error) {
	chunkSize := buildChunkSize(database.MYSQL, len(updatableCols), len(meta.primaryKeyIndex))

	var keys [][]any

	for start := 0; start < len(rowsVal); start += chunkSize {
		end := min(start+chunkSize, len(rowsVal))

		chunkKeys, err := orm.updateMySQLBulkChunk(&structUpdateMySQLBulkChunk{
			execCtx:       execCtx,
			execDB:        execDB,
			tableName:     tableName,
			rowsVal:       rowsVal,
			meta:          meta,
			updatableCols: updatableCols,
			start:         start,
			end:           end,
		})
		if err != nil {
			return nil, err
		}

		keys = append(keys, chunkKeys...)
	}

	return keys, nil
}

type structUpdateMySQLBulkChunk struct {
	execCtx       context.Context
	execDB        *database.Database
	tableName     string
	rowsVal       []reflect.Value
	meta          columnMetaData
	updatableCols []updatableColumn
	start         int
	end           int
}

func (orm *ORM) updateMySQLBulkChunk(req *structUpdateMySQLBulkChunk) ([][]any, error) {
	chunkRows := req.rowsVal[req.start:req.end]
	query := orm.buildMySQLBulkUpdateQuery(chunkRows, req.meta, req.updatableCols)

	setArgCount := len(query.args)

	whereClauses, err := orm.buildBulkWhereClause(&reqBuildBulkWhereClause{
		query:             query,
		primaryKeyIndex:   req.meta.primaryKeyIndex,
		primaryKeyColumns: req.meta.primaryKeyColumns,
		rows:              chunkRows,
		driver:            database.MYSQL,
	})
	if err != nil {
		return nil, orm.Error
	}

	whereSQL := strings.Join(whereClauses, " AND ")
	whereArgs := sanitizeMySQLReturnArgs(orm, query.args[setArgCount:])

	chunkKeys, err := orm.queryMySQLBulkReturnKeys(req.execCtx, req.execDB, req.tableName, whereSQL, whereArgs, req.meta, req.rowsVal[0].Type())
	if err != nil {
		return nil, err
	}

	updateQuery := buildUpdateStatement(req.tableName, strings.Join(query.clauses, ", "), whereSQL, nil, database.MYSQL)
	updateArgs := sanitizeMySQLReturnArgs(orm, query.args)

	_, _, _, err = req.execDB.ExecSQL(req.execCtx, orm.DatabaseConfig, updateQuery, updateArgs...)
	if err != nil {
		databaseErr := orm.setDatabaseOperationError("Update", err)
		return nil, orm.enrichBulkUpdateUniqueViolation(databaseErr, chunkRows, req.meta, database.MYSQL)
	}

	return chunkKeys, nil
}

func (orm *ORM) buildMySQLBulkUpdateQuery(
	chunkRows []reflect.Value,
	meta columnMetaData,
	updatableCols []updatableColumn,
) *resBuildQueryUpdateSQL {
	query := &resBuildQueryUpdateSQL{}
	argCounter := 1

	for idx := range updatableCols {
		columnQuery := orm.buildQueryUpdateSQL(chunkRows, &meta, argCounter, &updatableCols[idx], database.MYSQL)

		query.clauses = append(query.clauses, columnQuery.clauses...)
		query.args = append(query.args, columnQuery.args...)

		argCounter = columnQuery.argCounter
	}

	query.argCounter = argCounter

	return query
}

func (orm *ORM) queryMySQLBulkReturnKeys(
	execCtx context.Context,
	execDB *database.Database,
	tableName, whereSQL string,
	whereArgs []any,
	meta columnMetaData,
	rowType reflect.Type,
) ([][]any, error) {
	return orm.queryMySQLReturnKeys(&structMySQLReturnKeyQuery{
		execCtx:       execCtx,
		execDB:        execDB,
		tableName:     tableName,
		whereSQL:      whereSQL,
		whereArgs:     whereArgs,
		meta:          meta,
		rowType:       rowType,
		matchedRecord: "the bulk update",
	})
}

type structMySQLReturnKeyQuery struct {
	execCtx       context.Context
	rowType       reflect.Type
	execDB        *database.Database
	tableName     string
	whereSQL      string
	matchedRecord string
	meta          columnMetaData
	whereArgs     []any
}

func (orm *ORM) queryMySQLReturnKeys(req *structMySQLReturnKeyQuery) ([][]any, error) {
	keyQuery := fmt.Sprintf(
		"SELECT %s FROM %s WHERE %s FOR UPDATE",
		strings.Join(req.meta.primaryKeyColumns, ", "), req.tableName, req.whereSQL,
	)

	keyRows, _, _, err := req.execDB.QuerySQL(req.execCtx, orm.DatabaseConfig, keyQuery, req.whereArgs...)
	if err != nil {
		return nil, orm.setDatabaseOperationError("Update", err)
	}

	keys, scanErr := scanMySQLReturnKeys(keyRows, len(req.meta.primaryKeyColumns), req.meta, req.rowType)

	//nolint:sqlclosecheck
	closeErr := keyRows.Close()

	if scanErr != nil {
		return nil, orm.setError(
			"Ensure MySQL can read the primary keys of rows matching "+req.matchedRecord+".",
			fmt.Errorf("update: read MySQL Return keys: %w", scanErr),
			http.StatusInternalServerError,
		)
	}

	if closeErr != nil {
		return nil, orm.setError(
			"Ensure the MySQL result stream can be closed cleanly.",
			fmt.Errorf("update: close MySQL Return key rows: %w", closeErr),
			http.StatusInternalServerError,
		)
	}

	return keys, nil
}

func (orm *ORM) selectMySQLBulkReturnRows(
	execCtx context.Context,
	execDB *database.Database,
	tableName string,
	keys [][]any,
	meta columnMetaData,
	plan *returnPlan,
) ([]reflect.Value, error) {
	chunkSize := mysqlReturnSelectChunkSize(len(meta.primaryKeyColumns))

	var scanned []reflect.Value

	for start := 0; start < len(keys); start += chunkSize {
		end := min(start+chunkSize, len(keys))

		selectQuery, selectArgs := buildMySQLReturnSelect(tableName, plan.queryCols, meta.primaryKeyColumns, keys[start:end])

		chunk, err := orm.queryReturnRowsWithDatabase(execCtx, execDB, selectQuery, selectArgs, plan, "Update")
		if err != nil {
			return nil, err
		}

		scanned = append(scanned, chunk...)
	}

	return scanned, nil
}

func mysqlReturnSelectChunkSize(keyWidth int) int {
	chunkSize := 1000

	if keyWidth > 0 && chunkSize*keyWidth > 60000 {
		chunkSize = 60000 / keyWidth
	}

	if chunkSize < 1 {
		return 1
	}

	return chunkSize
}

func (orm *ORM) finishMySQLBulkReturn(plan *returnPlan, scanned, rowsVal []reflect.Value, meta columnMetaData) error {
	if err := orm.finishReturn(plan, scanned, rowsVal); err != nil {
		return err
	}

	orm.LastInsertId = lastInsertIDFromReturn(plan, scanned, rowsVal, meta)
	// UPDATE ... RETURN semantics count matched rows, including rows whose new
	// values equal their existing values (MySQL may report only changed rows).
	orm.RowsAffected = int64(len(scanned))
	orm.Message = "Data updated successfully"
	orm.HTTPCode = http.StatusOK

	return nil
}

func sanitizeMySQLReturnArgs(orm *ORM, args []any) []any {
	sanitized := append([]any(nil), args...)

	for idx, arg := range sanitized {
		sanitized[idx] = orm.sanitizeArg(arg)
	}

	return sanitized
}

func scanMySQLReturnKeys(rows *sql.Rows, columnCount int, meta columnMetaData, rowType reflect.Type) ([][]any, error) {
	if rows == nil {
		return nil, errors.New("query returned a nil rows handle")
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

			if fieldType.Kind() != reflect.String {
				break
			}

			if bytes, ok := values[idx].([]byte); ok {
				values[idx] = string(bytes)
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
