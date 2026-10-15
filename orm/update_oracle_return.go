package orm

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"github.com/elsyahtech/gorest/database"
)

type oracleReturnBind struct {
	column      string
	destination int
	holder      reflect.Value
}

// updateOracleWithReturn keeps Oracle's RETURNING INTO bind handling isolated
// from the row-returning implementations used by other SQL drivers.
func (orm *ORM) updateOracleWithReturn(
	execCtx context.Context,
	tableName string,
	rowsVal []reflect.Value,
	meta columnMetaData,
	plan *returnPlan,
) error {
	var returned []reflect.Value

	for _, row := range rowsVal {
		field := buildUpdatePayload(row, meta, database.ORACLE, 1)

		if len(field.setClauses) == 0 {
			return orm.setError(
				"Ensure that there are columns to update. Specify columns using .Select() or ensure your struct has non-primary key fields.",
				errors.New("update: no columns to update"),
			)
		}

		where, err := orm.buildSingleWhereClause(&row, field, &meta, database.ORACLE)
		if err != nil {
			return orm.Error
		}

		scanned, affected, err := orm.execOracleUpdateReturning(
			execCtx,
			tableName,
			strings.Join(field.setClauses, ", "),
			strings.Join(where.parts, " AND "),
			where.args,
			row,
			meta,
			plan,
		)

		if err != nil {
			return err
		}

		if affected {
			returned = append(returned, scanned)
		}
	}

	if len(returned) == 0 {
		return orm.setNotFound("update")
	}

	if err := orm.finishReturn(plan, returned, rowsVal); err != nil {
		return err
	}

	orm.RowsAffected = int64(len(returned))
	orm.Message = "Data updated successfully"

	return nil
}

func (orm *ORM) execOracleUpdateReturning(
	execCtx context.Context,
	tableName string,
	setClause string,
	whereClause string,
	args []any,
	inputRow reflect.Value,
	meta columnMetaData,
	plan *returnPlan,
) (reflect.Value, bool, error) {
	binds, err := buildOracleReturnBinds(plan, inputRow, meta)
	if err != nil {
		return reflect.Value{}, false, orm.setError(
			"Oracle Return() requires each returned column to map to a supported Go field type.",
			err,
			http.StatusBadRequest,
		)
	}

	placeholders := make([]string, 0, len(binds))

	for idx := range binds {
		placeholders = append(placeholders, getPlaceholder(database.ORACLE, len(args)+idx+1))
		args = append(args, sql.Out{Dest: binds[idx].holder.Addr().Interface()})
	}

	query := fmt.Sprintf(
		"UPDATE %s SET %s WHERE %s RETURNING %s INTO %s",
		tableName,
		setClause,
		whereClause,
		strings.Join(plan.queryCols, ", "),
		strings.Join(placeholders, ", "),
	)

	args = orm.sanitizeArgs(args)
	result, message, httpCode, err := orm.Database.ExecSQL(execCtx, orm.DatabaseConfig, query, args...)
	if err != nil {
		return reflect.Value{}, false, orm.setError(message, err, httpCode)
	}

	if result == nil {
		return reflect.Value{}, false, orm.setError(
			"Oracle did not return an execution result for UPDATE RETURNING INTO.",
			errors.New("update: Oracle returned a nil SQL result"),
		)
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return reflect.Value{}, false, orm.setError(
			"Ensure Oracle can report the affected row count for the UPDATE statement.",
			fmt.Errorf("update: Oracle RowsAffected: %w", err),
		)
	}

	if affected == 0 {
		return reflect.Value{}, false, nil
	}

	returned := reflect.New(plan.destType).Elem()

	for _, bind := range binds {
		if bind.destination < 0 {
			continue
		}
		if err := assignReflectValue(returned.Field(bind.destination), bind.holder.Interface()); err != nil {
			return reflect.Value{}, false, orm.setError(
				"Ensure Oracle returned column values can be assigned to the Return() destination fields.",
				fmt.Errorf("update: map Oracle RETURNING column %q: %w", bind.column, err),
			)
		}
	}

	return returned, true, nil
}

func buildOracleReturnBinds(plan *returnPlan, inputRow reflect.Value, meta columnMetaData) ([]oracleReturnBind, error) {
	_, destinationFields := taggedColumns(plan.destType)
	inputType := inputRow.Type()
	binds := make([]oracleReturnBind, 0, len(plan.queryCols))

	for _, column := range plan.queryCols {
		destinationIdx, hasDestination := destinationFields[strings.ToLower(column)]

		var fieldType reflect.Type

		if hasDestination {
			fieldType = plan.destType.Field(destinationIdx).Type
		} else {
			inputIdx, exists := meta.allColumnPrimaryKeyIdx[column]
			if !exists {
				for inputColumn, idx := range meta.allColumnPrimaryKeyIdx {
					if strings.EqualFold(inputColumn, column) {
						inputIdx, exists = idx, true
						break
					}
				}
			}

			if !exists {
				return nil, fmt.Errorf("return column %q has no matching tagged field", column)
			}

			fieldType = inputType.Field(inputIdx).Type
		}

		for fieldType.Kind() == reflect.Pointer {
			fieldType = fieldType.Elem()
		}

		if !isOracleOutputBindType(fieldType) {
			return nil, fmt.Errorf("return column %q has unsupported output type %s", column, fieldType)
		}

		binds = append(binds, oracleReturnBind{
			column:      column,
			destination: destinationIdxOrMinusOne(destinationIdx, hasDestination),
			holder:      reflect.New(fieldType).Elem(),
		})
	}

	if len(binds) == 0 {
		return nil, errors.New("Return() has no columns to bind")
	}

	return binds, nil
}

func destinationIdxOrMinusOne(idx int, ok bool) int {
	if !ok {
		return -1
	}
	return idx
}

func isOracleOutputBindType(typ reflect.Type) bool {
	if typ == reflect.TypeOf((*time.Time)(nil)).Elem() {
		return true
	}
	switch typ.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	case reflect.Slice:
		return typ.Elem().Kind() == reflect.Uint8
	default:
		return false
	}
}
