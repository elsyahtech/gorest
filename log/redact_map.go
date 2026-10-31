//nolint:revive,nolintlint // Package log provides logging utilities for the application.
package log

import (
	"fmt"
	"reflect"
)

func redactMap(value reflect.Value, depth int) any {
	if value.IsNil() {
		return nil
	}

	result := make(map[string]any, value.Len())
	iter := value.MapRange()

	for iter.Next() {
		keyString := fmt.Sprint(iter.Key().Interface())

		result[redactString(keyString)] = redactEntry(keyString, iter.Value(), depth)
	}

	return result
}
