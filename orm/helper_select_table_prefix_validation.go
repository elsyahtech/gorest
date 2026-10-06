package orm

import (
	"fmt"
	"strings"
)

func (orm *ORM) validateSelectTablePrefixes(byTable map[string][]string, parentTable string, plan *scyllaPreloadPlan) error {
	known := map[string]bool{strings.ToLower(parentTable): true}

	for _, hasMany := range plan.hasMany {
		known[strings.ToLower(hasMany.rel.table)] = true
	}

	for _, belongTo := range plan.belongsTo {
		known[strings.ToLower(belongTo.core.collection)] = true
	}

	for table := range byTable {
		if !known[table] {
			return orm.setError(
				fmt.Sprintf("Select() references table %q, but it's neither %q nor a table used by an active .Preload(). "+
					"ScyllaDB has no JOIN — only select columns from %q or from a table you're preloading.",
					table, parentTable, parentTable),
				fmt.Errorf("scylla preload error: select references unknown table %q", table),
			)
		}
	}

	return nil
}
