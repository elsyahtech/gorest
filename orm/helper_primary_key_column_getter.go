package orm

import (
	"reflect"
	"strings"
)

func getPrimaryKeyColumn(structType reflect.Type) (string, bool) {
	for j := 0; j < structType.NumField(); j++ {
		field := structType.Field(j)
		tag := field.Tag.Get("gorest")

		if tag == "" || tag == "-" {
			continue
		}

		parts := strings.Split(tag, ",")
		cleanTag := strings.TrimSpace(parts[0])

		if cleanTag == "" {
			continue
		}

		for k := 1; k < len(parts); k++ {
			if strings.EqualFold(strings.TrimSpace(parts[k]), "primary_key") {
				return cleanTag, true
			}
		}
	}

	return "", false
}
