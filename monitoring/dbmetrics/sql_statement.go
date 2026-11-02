//nolint:staticcheck,wrapcheck,wsl_v5 // Legacy statement interfaces are required as a fallback for SQL drivers.
package dbmetrics

import (
	"context"
	"database/sql/driver"
	"errors"
)

type instrumentedStmt struct {
	driver.Stmt
	metrics *Stats
}

func (s *instrumentedStmt) Exec(args []driver.Value) (driver.Result, error) {
	//nolint:staticcheck // Legacy drivers may only implement Stmt.Exec.
	start := s.metrics.startQuery()
	result, err := s.Stmt.Exec(args)
	s.metrics.finishQuery(start, err)
	return result, err
}

func (s *instrumentedStmt) Query(args []driver.Value) (driver.Rows, error) {
	//nolint:staticcheck // Legacy drivers may only implement Stmt.Query.
	start := s.metrics.startQuery()
	rows, err := s.Stmt.Query(args)
	if err != nil {
		s.metrics.finishQuery(start, err)
		return nil, err
	}
	if rows == nil {
		err = errors.New("SQL driver returned nil rows")
		s.metrics.finishQuery(start, err)
		return nil, err
	}
	return &instrumentedRows{Rows: rows, metrics: s.metrics, start: start}, nil
}

func (s *instrumentedStmt) ExecContext(ctx context.Context, args []driver.NamedValue) (driver.Result, error) {
	stmt, ok := s.Stmt.(driver.StmtExecContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	start := s.metrics.startQuery()
	result, err := stmt.ExecContext(ctx, args)
	if errors.Is(err, driver.ErrSkip) {
		s.metrics.skipQuery()
		return nil, err
	}
	s.metrics.finishQuery(start, err)
	return result, err
}

func (s *instrumentedStmt) QueryContext(ctx context.Context, args []driver.NamedValue) (driver.Rows, error) {
	stmt, ok := s.Stmt.(driver.StmtQueryContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	start := s.metrics.startQuery()
	rows, err := stmt.QueryContext(ctx, args)
	if err != nil {
		if errors.Is(err, driver.ErrSkip) {
			s.metrics.skipQuery()
			return nil, err
		}
		s.metrics.finishQuery(start, err)
		return nil, err
	}
	if rows == nil {
		err = errors.New("SQL driver returned nil rows")
		s.metrics.finishQuery(start, err)
		return nil, err
	}
	return &instrumentedRows{Rows: rows, metrics: s.metrics, start: start}, nil
}

type instrumentedTx struct {
	driver.Tx
	metrics *Stats
}

func (tx *instrumentedTx) Commit() error {
	err := tx.Tx.Commit()
	if err == nil {
		tx.metrics.recordCommit()
	}
	return err
}

func (tx *instrumentedTx) Rollback() error {
	err := tx.Tx.Rollback()
	if err == nil {
		tx.metrics.recordRollback()
	}
	return err
}
