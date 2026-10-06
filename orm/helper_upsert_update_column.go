package orm

// resolveUpsertUpdateCols: columns to be updated = explicit (UpsertUpdateCols),
// or all INSERT columns except conflict columns and the primary key.
func resolveUpsertUpdateCols(columns, conflictCols, pkCols, explicit []string) []string {
	if len(explicit) > 0 {
		return explicit
	}

	var updateCols []string

	for _, col := range columns {
		if containsFold(conflictCols, col) || containsFold(pkCols, col) {
			continue
		}

		updateCols = append(updateCols, col)
	}

	return updateCols
}
