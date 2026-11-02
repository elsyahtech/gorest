//nolint:wrapcheck,wsl_v5 // The SQL adapter preserves driver row and metadata behavior.
package dbmetrics

import (
	"database/sql/driver"
	"errors"
	"io"
	"reflect"
	"sync"
	"time"
)

type instrumentedRows struct {
	driver.Rows
	metrics *Stats
	start   time.Time
	once    sync.Once
}

func (r *instrumentedRows) Next(dest []driver.Value) error {
	err := r.Rows.Next(dest)
	if err != nil && !errors.Is(err, io.EOF) {
		r.finish(err)
	}
	return err
}

func (r *instrumentedRows) Close() error {
	err := r.Rows.Close()
	r.finish(err)
	return err
}

func (r *instrumentedRows) finish(err error) {
	r.once.Do(func() {
		r.metrics.finishQuery(r.start, err)
	})
}

func (r *instrumentedRows) HasNextResultSet() bool {
	rows, ok := r.Rows.(driver.RowsNextResultSet)
	return ok && rows.HasNextResultSet()
}

func (r *instrumentedRows) NextResultSet() error {
	rows, ok := r.Rows.(driver.RowsNextResultSet)
	if !ok {
		return io.EOF
	}
	return rows.NextResultSet()
}

func (r *instrumentedRows) ColumnTypeDatabaseTypeName(index int) string {
	rows, ok := r.Rows.(driver.RowsColumnTypeDatabaseTypeName)
	if !ok {
		return ""
	}
	return rows.ColumnTypeDatabaseTypeName(index)
}

func (r *instrumentedRows) ColumnTypeLength(index int) (int64, bool) {
	rows, ok := r.Rows.(driver.RowsColumnTypeLength)
	if !ok {
		return 0, false
	}
	return rows.ColumnTypeLength(index)
}

func (r *instrumentedRows) ColumnTypeNullable(index int) (nullable bool, ok bool) {
	rows, ok := r.Rows.(driver.RowsColumnTypeNullable)
	if !ok {
		return false, false
	}
	return rows.ColumnTypeNullable(index)
}

func (r *instrumentedRows) ColumnTypePrecisionScale(index int) (precision int64, scale int64, ok bool) {
	rows, ok := r.Rows.(driver.RowsColumnTypePrecisionScale)
	if !ok {
		return 0, 0, false
	}
	return rows.ColumnTypePrecisionScale(index)
}

func (r *instrumentedRows) ColumnTypeScanType(index int) reflect.Type {
	rows, ok := r.Rows.(driver.RowsColumnTypeScanType)
	if !ok {
		return reflect.TypeOf(new(any)).Elem()
	}
	return rows.ColumnTypeScanType(index)
}
