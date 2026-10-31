//nolint:revive,nolintlint // Package log provides logging utilities for the application.
package log

import "reflect"

func redactIndirect(value reflect.Value, depth int) any {
	if value.IsNil() {
		return nil
	}

	return redactValue(value.Elem(), depth+1)
}
