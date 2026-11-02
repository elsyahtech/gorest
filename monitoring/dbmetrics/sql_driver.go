//nolint:staticcheck,wrapcheck,wsl_v5 // The adapter must preserve driver sentinels and legacy-interface fallback behavior.
package dbmetrics

import (
	"context"
	"database/sql/driver"
	"errors"
)

// Connector wraps an SQL driver connector to record operations by its client.
type Connector struct {
	inner   driver.Connector
	metrics *Stats
}

// WrapConnector instruments SQL operations and successful transaction finalization.
func WrapConnector(connector driver.Connector, metrics *Stats) driver.Connector {
	collector := metrics
	if collector == nil {
		collector = New()
	}
	collector.EnableQueries()
	collector.enableTransactions()
	return &Connector{inner: connector, metrics: collector}
}

func (c *Connector) Connect(ctx context.Context) (driver.Conn, error) {
	conn, err := c.inner.Connect(ctx)
	if err != nil {
		return nil, err
	}
	if conn == nil {
		return nil, errors.New("SQL driver returned a nil connection")
	}
	return &instrumentedConn{Conn: conn, metrics: c.metrics}, nil
}

func (c *Connector) Driver() driver.Driver {
	return c.inner.Driver()
}

type instrumentedConn struct {
	driver.Conn
	metrics *Stats
}

func (c *instrumentedConn) Prepare(query string) (driver.Stmt, error) {
	stmt, err := c.Conn.Prepare(query)
	if err != nil {
		return nil, err
	}
	return wrapStatement(stmt, c.metrics)
}

func (c *instrumentedConn) PrepareContext(ctx context.Context, query string) (driver.Stmt, error) {
	var (
		stmt driver.Stmt
		err  error
	)
	if conn, ok := c.Conn.(driver.ConnPrepareContext); ok {
		stmt, err = conn.PrepareContext(ctx, query)
	} else {
		stmt, err = c.Conn.Prepare(query)
	}
	if err != nil {
		return nil, err
	}
	return wrapStatement(stmt, c.metrics)
}

func wrapStatement(stmt driver.Stmt, metrics *Stats) (driver.Stmt, error) {
	if stmt == nil {
		return nil, errors.New("SQL driver returned a nil statement")
	}
	return &instrumentedStmt{Stmt: stmt, metrics: metrics}, nil
}

func (c *instrumentedConn) Begin() (driver.Tx, error) {
	return c.BeginTx(context.Background(), driver.TxOptions{})
}

func (c *instrumentedConn) BeginTx(ctx context.Context, options driver.TxOptions) (driver.Tx, error) {
	var (
		transaction driver.Tx
		err         error
	)
	switch conn := c.Conn.(type) {
	case driver.ConnBeginTx:
		transaction, err = conn.BeginTx(ctx, options)
	default:
		if options.Isolation != driver.IsolationLevel(0) {
			return nil, errors.New("SQL driver does not support non-default transaction isolation")
		}
		if options.ReadOnly {
			return nil, errors.New("SQL driver does not support read-only transactions")
		}
		//nolint:staticcheck // Legacy drivers may only implement Conn.Begin.
		transaction, err = c.Conn.Begin()
	}
	if err != nil {
		return nil, err
	}
	if transaction == nil {
		return nil, errors.New("SQL driver returned a nil transaction")
	}
	return &instrumentedTx{Tx: transaction, metrics: c.metrics}, nil
}

func (c *instrumentedConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	conn, ok := c.Conn.(driver.ExecerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	start := c.metrics.startQuery()
	result, err := conn.ExecContext(ctx, query, args)
	if errors.Is(err, driver.ErrSkip) {
		c.metrics.skipQuery()
		return nil, err
	}
	c.metrics.finishQuery(start, err)
	return result, err
}

func (c *instrumentedConn) QueryContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Rows, error) {
	conn, ok := c.Conn.(driver.QueryerContext)
	if !ok {
		return nil, driver.ErrSkip
	}
	start := c.metrics.startQuery()
	rows, err := conn.QueryContext(ctx, query, args)
	if err != nil {
		if errors.Is(err, driver.ErrSkip) {
			c.metrics.skipQuery()
			return nil, err
		}
		c.metrics.finishQuery(start, err)
		return nil, err
	}
	if rows == nil {
		err = errors.New("SQL driver returned nil rows")
		c.metrics.finishQuery(start, err)
		return nil, err
	}
	return &instrumentedRows{Rows: rows, metrics: c.metrics, start: start}, nil
}

func (c *instrumentedConn) Ping(ctx context.Context) error {
	if conn, ok := c.Conn.(driver.Pinger); ok {
		return conn.Ping(ctx)
	}
	return nil
}

func (c *instrumentedConn) IsValid() bool {
	if conn, ok := c.Conn.(driver.Validator); ok {
		return conn.IsValid()
	}
	return true
}

func (c *instrumentedConn) ResetSession(ctx context.Context) error {
	if conn, ok := c.Conn.(driver.SessionResetter); ok {
		return conn.ResetSession(ctx)
	}
	return nil
}

func (c *instrumentedConn) CheckNamedValue(value *driver.NamedValue) error {
	if conn, ok := c.Conn.(driver.NamedValueChecker); ok {
		return conn.CheckNamedValue(value)
	}
	return driver.ErrSkip
}
