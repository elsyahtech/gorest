package orm

import (
	"fmt"
	"reflect"
	"strings"
)

func buildPreloadColumns(
	relatedStructType reflect.Type,
	tableRef string,
	currentColumnsToSelect []string,
) []string {
	selectedAliases := make(map[string]struct{})
	columnsToSelect := currentColumnsToSelect

	for _, existing := range columnsToSelect {
		existingLower := strings.ToLower(existing)

		selectedAliases[existingLower] = struct{}{}

		// Extract explicit SQL alias if " AS " keyword is present
		if idx := strings.LastIndex(existingLower, " as "); idx != -1 {
			aliasPart := strings.TrimSpace(existingLower[idx+4:])

			selectedAliases[aliasPart] = struct{}{}
		}
	}

	// Auto-select related columns securely
	for k := 0; k < relatedStructType.NumField(); k++ {
		relField := relatedStructType.Field(k)
		dbTag := relField.Tag.Get("gorest")

		// db:"-" means completely ignored by ORM mapping
		if dbTag == "" || dbTag == "-" {
			continue
		}

		// KUNCI UTAMA: Ambil nama kolom bersih sebelum tanda koma (mengabaikan ", primary_key")
		parts := strings.Split(dbTag, ",")
		cleanCol := strings.TrimSpace(parts[0])

		if cleanCol == "" {
			continue
		}

		colAlias := fmt.Sprintf("%s_%s", tableRef, cleanCol)
		colAliasLower := strings.ToLower(colAlias)

		colSelect := fmt.Sprintf("%s.%s AS %s", tableRef, cleanCol, colAlias)

		// Exact match check: to substring collision (e.g. _backup / _identity)
		if _, exists := selectedAliases[colAliasLower]; exists {
			continue
		}

		// Register to local map to prevent duplicate appends in the same batch
		selectedAliases[colAliasLower] = struct{}{}

		columnsToSelect = append(columnsToSelect, colSelect)
	}

	return columnsToSelect
}
