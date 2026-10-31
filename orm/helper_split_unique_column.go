package orm

import "strings"

func splitUniqueColumns(value string) []string {
	parts := strings.Split(value, ",")

	for idx := range parts {
		parts[idx] = strings.Trim(strings.TrimSpace(parts[idx]), `"'`)
	}

	return parts
}
