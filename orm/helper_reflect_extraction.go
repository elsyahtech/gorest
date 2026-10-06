package orm

import (
	"fmt"
	"reflect"
)

func (orm *ORM) extractReflectionValue(data any, opName string) (*reflect.Value, error) {
	val := reflect.ValueOf(data)

	for val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return nil, orm.setError(
				"Ensure the payload data passed is a struct, slice of structs (array), or pointers",
				fmt.Errorf("%s: data pointer cannot be nil", opName),
			)
		}

		val = val.Elem()
	}

	return &val, nil
}
