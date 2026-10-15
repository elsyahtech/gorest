package orm

import "fmt"

func (orm *ORM) setScyllaWriteError(operation string, err error, httpCode int) error {
	return orm.setError(
		"Check that the ScyllaDB connection and target table are available, and verify the CQL statement, primary-key restrictions, column names, and value types.",
		fmt.Errorf("%s: ScyllaDB write: %w", operation, err),
		httpCode,
	)
}
