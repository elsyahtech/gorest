//nolint:revive,nolintlint // Package log provides logging utilities for the application.
package log

import (
	"fmt"
	"reflect"
)

func redactFallback(value reflect.Value) any {
	if !value.CanInterface() {
		return redactedValue
	}

	if stringer, ok := value.Interface().(fmt.Stringer); ok {
		return redactString(stringer.String())
	}

	if err, ok := value.Interface().(error); ok {
		return redactString(err.Error())
	}

	return redactString(fmt.Sprint(value.Interface()))
}
