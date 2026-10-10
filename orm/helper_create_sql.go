package orm

import (
	"database/sql"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/elsyahtech/gorest/database"
)

// buildUpsertCondition assembles the DO UPDATE ... WHERE / WHEN MATCHED AND ... condition from UpsertClauses.
// startIdx = the next placeholder number (important for Postgres/SQL Server: $N / @pN must continue after the INSERT arguments).
func (orm *ORM) buildUpsertCondition(tableName, driver string, startIdx int) (string, []any) {
	parts := make([]string, 0, len(orm.UpsertClauses))

	startIndex := startIdx

	for _, clause := range orm.UpsertClauses {
		// Postgres (target vs EXCLUDED), SQL Server and Oracle (target vs source): columns without prefixes are ambiguous.
		if driver == database.POSTGRES || driver == database.SQLSERVER || driver == database.ORACLE {
			clause = orm.upsertClause(clause, tableName)
		}

		// Just like in Where(): "??" is preserved as a literal operator, while "?" is replaced by a driver placeholder.
		normalized, next := normalizeWherePlaceholders(clause, driver, startIndex)

		startIndex = next

		parts = append(parts, "("+normalized+")")
	}

	return strings.Join(parts, " AND "), orm.UpsertArgs
}

// placeholders = "(@p1, @p2)" per row from buildValueArgCreateSQL, nextIdx = next placeholder number.
func (orm *ORM) buildMergeSQL(tableName string, columns, pkCols, placeholders []string, driver string, nextIdx int) (string, []any) {
	var (
		args []any
	)

	// Prevent race condition between the MATCHED check and the INSERT
	orm.safeWriteString(fmt.Sprintf(
		"MERGE INTO %s WITH (HOLDLOCK) AS target USING (VALUES %s) AS source (%s) ON ",
		tableName, strings.Join(placeholders, ", "), strings.Join(columns, ", "),
	))

	orm.safeWriteString(upsertMergeOn(orm.UpsertConflictCols, "target", "source"))

	// Columns to update: UpsertUpdateCols, or all columns except conflict cols & primary key
	updateCols := resolveUpsertUpdateCols(columns, orm.UpsertConflictCols, pkCols, orm.UpsertUpdateCols)

	// If no columns are updated, the WHEN MATCHED clause is skipped (resulting in an insert-if-not-exists operation).
	if len(updateCols) > 0 {
		orm.safeWriteString(" WHEN MATCHED")

		if len(orm.UpsertClauses) > 0 {
			cond, condArgs := orm.buildUpsertCondition("target", driver, nextIdx)

			orm.safeWriteString(" AND ")
			orm.safeWriteString(cond)

			args = append(args, condArgs...)
		}

		updates := make([]string, 0, len(updateCols))

		for _, col := range updateCols {
			updates = append(updates, fmt.Sprintf("target.%s = source.%s", col, col))
		}

		orm.safeWriteString(" THEN UPDATE SET ")
		orm.safeWriteString(strings.Join(updates, ", "))
	}

	sourceCols := make([]string, 0, len(columns))

	for _, col := range columns {
		sourceCols = append(sourceCols, "source."+col)
	}

	// MERGE in SQL Server must end with a semicolon
	orm.safeWriteString(fmt.Sprintf(
		" WHEN NOT MATCHED THEN INSERT (%s) VALUES (%s);",
		strings.Join(columns, ", "), strings.Join(sourceCols, ", "),
	))

	return orm.StringBuilder.String(), args
}

// upsert BergeOn assembles the ON condition for MERGE (SQL Server and Oracle).
// NULL-safe: conflict columns that are NULL are still considered matches.
func upsertMergeOn(conflictCols []string, target, source string) string {
	onParts := make([]string, 0, len(conflictCols))

	for _, col := range conflictCols {
		onParts = append(onParts, fmt.Sprintf(
			"(%[2]s.%[1]s = %[3]s.%[1]s OR (%[2]s.%[1]s IS NULL AND %[3]s.%[1]s IS NULL))", col, target, source,
		))
	}

	return strings.Join(onParts, " AND ")
}

// buildOracleMergeSQL: Oracle version of MERGE. Differences from SQL Server:
//   - table aliases without AS, no HOLDLOCK, and no trailing semicolon
//   - data source is SELECT ... FROM dual (UNION ALL per row), not VALUES
//   - update condition is in UPDATE SET ... WHERE, not WHEN MATCHED AND
//
// rowCount = number of rows to INSERT, nextIdx = next placeholder index.
func (orm *ORM) buildOracleMergeSQL(tableName string, columns, pkCols []string, rowCount int, driver string, nextIdx int) (string, []any) {
	const (
		target = "tgt"
		source = "src"
	)

	var (
		args []any
	)

	// Nomor placeholder sama dengan buildValueArgCreateSQL: berurutan per baris, per kolom, mulai dari 1.
	selects := make([]string, 0, rowCount)

	for row := 0; row < rowCount; row++ {
		items := make([]string, 0, len(columns))

		for colIdx, col := range columns {
			item := getPlaceholder(driver, row*len(columns)+colIdx+1)

			if row == 0 {
				item += " AS " + col
			}

			items = append(items, item)
		}

		selects = append(selects, "SELECT "+strings.Join(items, ", ")+" FROM dual")
	}

	orm.safeWriteString(fmt.Sprintf(
		"MERGE INTO %s %s USING (%s) %s ON (%s)",
		tableName, target, strings.Join(selects, " UNION ALL "), source,
		upsertMergeOn(orm.UpsertConflictCols, target, source),
	))

	// Kalau nggak ada kolom yang di-update, WHEN MATCHED dilewati (jadi insert-if-not-exists)
	updateCols := resolveUpsertUpdateCols(columns, orm.UpsertConflictCols, pkCols, orm.UpsertUpdateCols)

	if len(updateCols) > 0 {
		updates := make([]string, 0, len(updateCols))

		for _, col := range updateCols {
			updates = append(updates, fmt.Sprintf("%s.%s = %s.%s", target, col, source, col))
		}

		orm.safeWriteString(" WHEN MATCHED THEN UPDATE SET ")
		orm.safeWriteString(strings.Join(updates, ", "))

		if len(orm.UpsertClauses) > 0 {
			cond, condArgs := orm.buildUpsertCondition(target, driver, nextIdx)

			orm.safeWriteString(" WHERE ")
			orm.safeWriteString(cond)

			args = append(args, condArgs...)
		}
	}

	sourceCols := make([]string, 0, len(columns))

	for _, col := range columns {
		sourceCols = append(sourceCols, source+"."+col)
	}

	orm.safeWriteString(fmt.Sprintf(
		" WHEN NOT MATCHED THEN INSERT (%s) VALUES (%s)",
		strings.Join(columns, ", "), strings.Join(sourceCols, ", "),
	))

	return orm.StringBuilder.String(), args
}

type resBuildValueArgCreateSQL struct {
	valueArgs    []any
	placeholders []string
}

func buildValueArgCreateSQL(rowsVal []reflect.Value, columnIndex []int, activeDriver string) *resBuildValueArgCreateSQL {
	placeholders := make([]string, 0, len(rowsVal))
	valueArgs := make([]any, 0, len(columnIndex)*len(rowsVal))

	argCounter := 1

	for _, rowVal := range rowsVal {
		rowPlaceholders := make([]string, 0, len(columnIndex))

		for _, columnIdx := range columnIndex {
			fieldVal := rowVal.Field(columnIdx).Interface()

			valueArgs = append(valueArgs, fieldVal)

			rowPlaceholders = append(rowPlaceholders, getPlaceholder(activeDriver, argCounter))

			argCounter++
		}

		placeholders = append(placeholders, fmt.Sprintf("(%s)", strings.Join(rowPlaceholders, ", ")))
	}

	return &resBuildValueArgCreateSQL{
		valueArgs:    valueArgs,
		placeholders: placeholders,
	}
}

func (orm *ORM) buildUpsertCreateSQL(
	meta columnMetaData,
	table string,
	driver string,
	resultArg *resBuildValueArgCreateSQL,
) error {
	if !orm.IsUpsert {
		return nil
	}

	// Validation + fallback conflict columns to primary key + check placeholder count
	if err := orm.prepareUpsertSQL(meta, driver); err != nil {
		return err
	}

	updateCols := resolveUpsertUpdateCols(meta.columns, orm.UpsertConflictCols, meta.primaryKeyColumns, orm.UpsertUpdateCols)

	switch driver {
	case database.POSTGRES, database.SQLITE:
		conflictTarget := strings.Join(orm.UpsertConflictCols, ", ")

		// If no columns are updated, the SET clause is empty, resulting in a syntax error. Fall back to DO NOTHING.
		if len(updateCols) == 0 {
			orm.safeWriteString(fmt.Sprintf(" ON CONFLICT (%s) DO NOTHING", conflictTarget))

			break
		}

		updates := make([]string, 0, len(updateCols))

		for _, col := range updateCols {
			updates = append(updates, fmt.Sprintf("%s = EXCLUDED.%s", col, col))
		}

		orm.safeWriteString(fmt.Sprintf(" ON CONFLICT (%s) DO UPDATE SET %s", conflictTarget, strings.Join(updates, ", ")))

		// WHERE must come AFTER the SET list
		if len(orm.UpsertClauses) == 0 {
			return nil
		}

		cond, condArgs := orm.buildUpsertCondition(table, driver, len(resultArg.valueArgs)+1)

		orm.safeWriteString(" WHERE ")
		orm.safeWriteString(cond)

		resultArg.valueArgs = append(resultArg.valueArgs, condArgs...)
	case database.MYSQL:
		updates := make([]string, 0, len(updateCols))

		for _, col := range updateCols {
			updates = append(updates, fmt.Sprintf("%s = VALUES(%s)", col, col))
		}

		// If no columns are being updated, use a no-op to keep the syntax valid.
		if len(updates) == 0 {
			first := orm.UpsertConflictCols[0]

			updates = append(updates, fmt.Sprintf("%s = %s", first, first))
		}

		orm.safeWriteString(" ON DUPLICATE KEY UPDATE ")
		orm.safeWriteString(strings.Join(updates, ", "))
	case database.SQLSERVER:
		orm.StringBuilder.Reset()

		mergeQuery, mergeArgs := orm.buildMergeSQL(
			table,
			meta.columns,
			meta.primaryKeyColumns,
			resultArg.placeholders,
			driver,
			len(resultArg.valueArgs)+1,
		)

		orm.safeWriteString(mergeQuery)

		resultArg.valueArgs = append(resultArg.valueArgs, mergeArgs...)
	case database.ORACLE:
		orm.StringBuilder.Reset()

		_, mergeArgs := orm.buildOracleMergeSQL(
			table,
			meta.columns,
			meta.primaryKeyColumns,
			len(resultArg.placeholders),
			driver,
			len(resultArg.valueArgs)+1,
		)
		resultArg.valueArgs = append(resultArg.valueArgs, mergeArgs...)
	default:
	}

	return nil
}

// extractResultCreateSQL fills RowsAffected, LastInsertId, Message, and Result after a create/upsert query.
func (orm *ORM) extractResultCreateSQL(result sql.Result, primaryKeyIndex []int, rowsVal []reflect.Value) {
	orm.RowsAffected = rowsAffectedOf(result)

	if len(primaryKeyIndex) > 0 {
		orm.LastInsertId = joinPrimaryKeys(primaryKeyIndex, rowsVal)
	} else if lastID, err := result.LastInsertId(); err == nil && lastID > 0 {
		orm.LastInsertId = strconv.FormatInt(lastID, 10)
	} else {
		orm.LastInsertId = ""
	}

	// Message depends on the operation, not on whether an ID was returned.
	idKey := lastInsertID
	if orm.IsUpsert {
		orm.Message = "Data saved successfully"
		idKey = upsertedID
	} else {
		orm.Message = "Data created successfully"
	}

	orm.Result = map[string]any{
		isSuccess:    true,
		rowsAffected: orm.RowsAffected,
		idKey:        orm.LastInsertId,
	}
}

// rowsAffectedOf returns the number of affected rows, or 0 if the driver cannot report it.
func rowsAffectedOf(result sql.Result) int64 {
	affected, err := result.RowsAffected()
	if err != nil {
		return 0
	}

	return affected
}

// joinPrimaryKeys builds "pk1-pk2, pk1-pk2, ..." from the primary key fields of each row.
// Composite keys are joined with "-", and rows are joined with ", ".
func joinPrimaryKeys(primaryKeyIndex []int, rowsVal []reflect.Value) string {
	ids := make([]string, 0, len(rowsVal))

	for _, rowVal := range rowsVal {
		parts := make([]string, 0, len(primaryKeyIndex))

		for _, idx := range primaryKeyIndex {
			parts = append(parts, fmt.Sprint(rowVal.Field(idx).Interface()))
		}

		ids = append(ids, strings.Join(parts, "-"))
	}

	return strings.Join(ids, ", ")
}

func (orm *ORM) buildReturnCreateSQL(
	data any,
	rowsVal []reflect.Value,
	meta columnMetaData,
	activeDriver, tableName string,
	resultArg *resBuildValueArgCreateSQL,
) (*returnPlan, error) {
	var retPlan *returnPlan

	if orm.IsReturn {
		plan, err := orm.planReturn(data, rowsVal, meta, activeDriver)
		if err != nil {
			return nil, err
		}

		retPlan = plan

		switch activeDriver {
		case database.POSTGRES, database.SQLITE:
			orm.safeWriteString(retPlan.returningClause())
		case database.SQLSERVER:
			if orm.IsUpsert {
				// MERGE: OUTPUT harus sebelum titik koma penutup
				mergeQuery := strings.TrimSuffix(orm.StringBuilder.String(), ";")

				orm.StringBuilder.Reset()
				orm.safeWriteString(mergeQuery + " " + retPlan.outputClause() + ";")
			} else {
				// INSERT: OUTPUT berada di antara daftar kolom dan VALUES
				orm.StringBuilder.Reset()
				orm.safeWriteString(fmt.Sprintf(
					"INSERT INTO %s (%s) %s VALUES %s",
					tableName, strings.Join(meta.columns, ", "), retPlan.outputClause(), strings.Join(resultArg.placeholders, ", "),
				))
			}
		default:
		}
	}

	return retPlan, nil
} //nolint:revive
