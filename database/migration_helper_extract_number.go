package database

import (
	"fmt"
	"strings"
)

func extractNumber(ext, filename string) int {
	name := strings.TrimSuffix(strings.ToLower(filename), ext)

	// Split by underscore, first part is the number
	parts := strings.Split(name, "_")

	if len(parts) == 0 {
		return 0
	}

	var number int

	_, err := fmt.Sscanf(parts[0], "%d", &number)
	if err != nil {
		return 0
	}

	return number
}
