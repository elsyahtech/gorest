package orm

import (
	"fmt"
	"strings"
)

func (orm *ORM) Upsert(args ...any) *ORM {
	orm.IsUpsert = true

	if len(args) == 0 {
		return orm
	}

	// Clause format: the conflict column is derived from the "column = ?" pattern.
	// Validation of the number of placeholders versus arguments is performed in
	// prepareUpsert (the driver is only known at Create).
	if first, ok := args[0].(string); ok && isUpsertClause(first) {
		orm.UpsertClauses = append(orm.UpsertClauses, first)
		orm.UpsertArgs = append(orm.UpsertArgs, args[1:]...)

		for _, m := range upsertEqColRe.FindAllStringSubmatch(first, -1) {
			orm.UpsertConflictCols = appendUnique(orm.UpsertConflictCols, m[1])
		}

		return orm
	}

	// Column
	for _, arg := range args {
		strArg, isOk := arg.(string)
		if !isOk {
			orm.Message = "upsert(columns...) only accepts string column names"
			orm.Error = fmt.Errorf("upsert: invalid column argument %v", arg)

			return orm
		}

		for _, col := range strings.Split(strArg, ",") {
			if col = strings.TrimSpace(col); col != "" {
				orm.UpsertConflictCols = appendUnique(orm.UpsertConflictCols, col)
			}
		}
	}

	return orm
}

func containsFold(list []string, val string) bool {
	for _, v := range list {
		if strings.EqualFold(v, val) {
			return true
		}
	}

	return false
}

func appendUnique(list []string, val string) []string {
	if containsFold(list, val) {
		return list
	}

	return append(list, val)
}
