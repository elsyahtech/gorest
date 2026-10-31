//nolint:revive,nolintlint // Package log provides logging utilities for the application.
package log

import "reflect"

func redactElements(value reflect.Value, depth int) []any {
	result := make([]any, value.Len())

	for index := 0; index < value.Len(); index++ {
		result[index] = redactValue(value.Index(index), depth+1)
	}

	return result
}
