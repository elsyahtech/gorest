package orm

import "database/sql"

func (orm *ORM) extractResultDeleteSQL(result sql.Result) {
	var rowAffected int64

	if result != nil {
		if affected, err := result.RowsAffected(); err == nil {
			rowAffected = affected
		}
	}

	// 10. Report the aggregated result back to the caller/handler.
	orm.RowsAffected = rowAffected
	orm.Result = map[string]any{
		isSuccess:    true,
		rowsAffected: rowAffected,
	}
}
