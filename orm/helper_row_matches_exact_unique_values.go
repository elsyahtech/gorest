package orm

import (
	"fmt"
	"strings"
)

func rowMatchesExactUniqueValues(rowValues []any, exactValues []string) bool {
	if len(exactValues) == 0 {
		return true
	}

	if len(rowValues) != len(exactValues) {
		return false
	}

	for i, value := range exactValues {
		if value != "" && !strings.EqualFold(fmt.Sprint(rowValues[i]), value) {
			return false
		}
	}

	return true
}
