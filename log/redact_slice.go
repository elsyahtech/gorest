//nolint:revive,nolintlint // Package log provides logging utilities for the application.
package log

import "reflect"

func redactSlice(value reflect.Value, depth int) any {
	if value.IsNil() {
		return nil
	}

	if value.Type().Elem().Kind() == reflect.Uint8 {
		return redactString(string(value.Bytes()))
	}

	return redactElements(value, depth)
}
