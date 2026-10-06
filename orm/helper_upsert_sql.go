package orm

import (
	"errors"
	"fmt"

	"github.com/elsyahtech/gorest/database"
)

func (orm *ORM) prepareUpsertSQL(meta columnMetaData, driver string) error {
	switch driver {
	case database.POSTGRES, database.SQLITE, database.MYSQL, database.SQLSERVER, database.ORACLE:
	default:
		return orm.setError(
			"Upsert is not supported on this database driver",
			fmt.Errorf("create: upsert is not supported on driver %s", driver))
	}

	if err := orm.validatePrepareUpsertSQL(driver, meta); err != nil {
		return orm.Error
	}

	// The conflict column must be included in the INSERT. A PK that
	// still has a zero value (auto-increment) is not included, and Select() can exclude columns.
	for _, col := range orm.UpsertConflictCols {
		if !containsFold(meta.columns, col) {
			return orm.setError(
				fmt.Sprintf("Upsert conflict column %q is not part of the INSERT columns. "+
					"Set its value on the struct (zero-value primary keys are skipped) or include it in Select().", col),
				fmt.Errorf("create: upsert conflict column %q is not in INSERT columns", col),
			)
		}
	}

	placeholderCount := 0

	for _, clause := range orm.UpsertClauses {
		placeholderCount += countActivePlaceholders(clause, driver)
	}

	if placeholderCount != len(orm.UpsertArgs) {
		return orm.setError(
			"Ensure the number of placeholders in .Upsert() matches the number of arguments provided.",
			fmt.Errorf("create: upsert clause has %d placeholder(s) but got %d arg(s)", placeholderCount, len(orm.UpsertArgs)),
		)
	}

	return nil
}

func (orm *ORM) validatePrepareUpsertSQL(driver string, meta columnMetaData) error {
	if driver == database.MYSQL && len(orm.UpsertClauses) > 0 {
		return orm.setError("conditional upsert is not supported on MySQL. Please use Upsert(\"column\")",
			errors.New("create: Upsert clause is not supported on MySQL"))
	}

	// Empty Upsert() / clause without "column = ?" -> fallback to primary key
	if len(orm.UpsertConflictCols) == 0 {
		if len(meta.primaryKeyColumns) == 0 {
			return orm.setError(
				"Upsert() needs conflict columns, e.g. Upsert(\"email\"), or a struct with a primary_key tag",
				errors.New("create: Upsert without conflict columns"),
			)
		}

		orm.UpsertConflictCols = append([]string(nil), meta.primaryKeyColumns...)
	}

	if err := orm.validateColumnNamesUpsert(orm.UpsertConflictCols, meta.allColumnPrimaryKeyIdx, "UpsertConflictCols"); err != nil {
		return err
	}

	if len(orm.UpsertUpdateCols) > 0 {
		if err := orm.validateColumnNamesUpsert(orm.UpsertUpdateCols, meta.allColumnPrimaryKeyIdx, "UpsertUpdateCols"); err != nil {
			return err
		}
	}

	// Oracle prohibits updating columns used in the ON clause (ORA-38104)
	if driver == database.ORACLE {
		for _, col := range orm.UpsertUpdateCols {
			if containsFold(orm.UpsertConflictCols, col) {
				return orm.setError(
					fmt.Sprintf("Oracle cannot update column %q because it is used as an upsert conflict column.", col),
					fmt.Errorf("create: upsert update column %q is also a conflict column (ORA-38104)", col),
				)
			}
		}
	}

	return nil
}
