package orm

import (
	"fmt"
	"reflect"
	"strings"
)

// findRelatedDBColumn finds the database column name for a given target key in the related struct.
func (orm *ORM) findRelatedDBColumn(relatedStructType reflect.Type, targetKey string, relationName string) (string, error) {
	for idx := 0; idx < relatedStructType.NumField(); idx++ {
		field := relatedStructType.Field(idx)

		dbTag := field.Tag.Get("gorest")

		if dbTag == "-" || dbTag == "" {
			continue
		}

		// Strip tags of additional modifiers (such as ", primary_key", etc.)
		parts := strings.Split(dbTag, ",")
		cleanColumn := strings.TrimSpace(parts[0])

		if cleanColumn == "" {
			continue
		}

		// Check if the cleaned name or the struct field name matches targetKey
		if strings.EqualFold(cleanColumn, targetKey) || strings.EqualFold(field.Name, targetKey) {
			return cleanColumn, nil
		}
	}

	const message = "ensure that the references field exists on the related struct"

	return "", orm.setError(message, fmt.Errorf("preload error: reference %q for relation %q was not found", targetKey, relationName))
}
