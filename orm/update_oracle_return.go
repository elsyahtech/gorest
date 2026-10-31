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

type oracleReturnBind struct {
	holder      reflect.Value
	column      string
	destination int
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

		scanned, affected, err := orm.execOracleUpdateReturning(&structExecOracleUpdateReturning{
			execCtx:     execCtx,
			tableName:   tableName,
			setClause:   strings.Join(field.setClauses, ", "),
			whereClause: strings.Join(where.parts, " AND "),
			args:        where.args,
			inputRow:    row,
			meta:        meta,
			plan:        plan,
		})
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

	const message = "Data updated successfully"

	orm.RowsAffected = int64(len(returned))
	orm.Message = message

	return nil
}

type structExecOracleUpdateReturning struct {
	execCtx     context.Context
	plan        *returnPlan
	inputRow    reflect.Value
	tableName   string
	setClause   string
	whereClause string
	meta        columnMetaData
	args        []any
}

func (orm *ORM) execOracleUpdateReturning(req *structExecOracleUpdateReturning) (reflect.Value, bool, error) {
	binds, err := buildOracleReturnBinds(req.plan, req.inputRow, req.meta)
	if err != nil {
		return reflect.Value{}, false, orm.setError(
			"Oracle Return() requires each returned column to map to a supported Go field type.",
			err,
			http.StatusBadRequest,
		)
	}

	placeholders := make([]string, 0, len(binds))

	for idx := range binds {
		placeholders = append(placeholders, getPlaceholder(database.ORACLE, len(req.args)+idx+1))
		req.args = append(req.args, sql.Out{Dest: binds[idx].holder.Addr().Interface()})
	}

	query := fmt.Sprintf(
		"UPDATE %s SET %s WHERE %s RETURNING %s INTO %s",
		req.tableName,
		req.setClause,
		req.whereClause,
		strings.Join(req.plan.queryCols, ", "),
		strings.Join(placeholders, ", "),
	)

	req.args = orm.sanitizeArgs(req.args)

	result, _, _, err := orm.Database.ExecSQL(req.execCtx, orm.DatabaseConfig, query, req.args...)
	if err != nil {
		return reflect.Value{}, false, orm.setDatabaseOperationError("Update return", err)
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

	returned := reflect.New(req.plan.destType).Elem()

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
