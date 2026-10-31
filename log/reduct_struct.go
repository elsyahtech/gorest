//nolint:revive,nolintlint // Package log provides logging utilities for the application.
package log

import (
	"fmt"
	"reflect"
	"strings"
)

func redactStruct(value reflect.Value, depth int) any {
	if text, ok := structAsText(value); ok {
		return redactString(text)
	}

	result := make(map[string]any, value.NumField())

	for index := 0; index < value.NumField(); index++ {
		fieldType := value.Type().Field(index)

		if fieldType.PkgPath != "" {
			continue
		}

		key := structFieldKey(fieldType)

		result[redactString(key)] = redactEntry(key, value.Field(index), depth)
	}

	return result
}

func structAsText(value reflect.Value) (string, bool) {
	if !value.CanInterface() {
		return "", false
	}

	if err, ok := value.Interface().(error); ok {
		return err.Error(), true
	}

	if stringer, ok := value.Interface().(fmt.Stringer); ok {
		return stringer.String(), true
	}

	return "", false
}

func structFieldKey(fieldType reflect.StructField) string {
	if jsonTag := strings.Split(fieldType.Tag.Get("json"), ",")[0]; jsonTag != "" && jsonTag != "-" {
		return jsonTag
	}

	return fieldType.Name
}
