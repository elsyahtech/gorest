package orm

import (
	"reflect"
	"strings"
)

type structFieldInfo struct {
	name  string
	dbTag string
	index int
}

// buildFieldLookupMap creates O(1) field lookup by column name.
func buildStructFieldMapByTag(structType reflect.Type) map[string]*structFieldInfo {
	lookupMap := make(map[string]*structFieldInfo)

	for idx := 0; idx < structType.NumField(); idx++ {
		field := structType.Field(idx)
		dbTag := field.Tag.Get("gorest")

		if dbTag == "-" || dbTag == "" {
			continue
		}

		colName := strings.ToLower(strings.TrimSpace(strings.Split(dbTag, ",")[0]))
		if colName != "" {
			lookupMap[colName] = &structFieldInfo{index: idx, name: field.Name, dbTag: dbTag}
		}
	}

	return lookupMap
}

func buildStructFieldMap(structType reflect.Type) map[string]*structFieldInfo {
	lookupMap := make(map[string]*structFieldInfo)

	for idx := 0; idx < structType.NumField(); idx++ {
		field := structType.Field(idx)
		dbTag := field.Tag.Get("gorest")

		if dbTag == "-" {
			continue
		}

		if dbTag != "" {
			colName := strings.ToLower(strings.TrimSpace(strings.Split(dbTag, ",")[0]))

			if colName != "" {
				lookupMap[colName] = &structFieldInfo{index: idx, name: field.Name, dbTag: dbTag}
			}
		}

		// Fallback field name for non-tagOnly mode (SQL)
		fieldNameLower := strings.ToLower(field.Name)

		if _, exists := lookupMap[fieldNameLower]; !exists {
			lookupMap[fieldNameLower] = &structFieldInfo{index: idx, name: field.Name, dbTag: dbTag}
		}
	}

	return lookupMap
}
