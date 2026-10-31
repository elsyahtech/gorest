package orm

import (
	"fmt"
	"net/http"
	"reflect"
)

func (orm *ORM) buildValueValidateStructTags(data any, opName string) (reflect.Value, error) {
	val := reflect.ValueOf(data)
	if !val.IsValid() {
		return val, orm.setError(
			"Ensure the payload data passed is a struct, a slice of structs, or a valid pointer.",
			fmt.Errorf("%s - validateStructTags: InvalidReflectValue", opName),
			http.StatusBadRequest,
		)
	}

	if val.Kind() == reflect.Pointer {
		if val.IsNil() {
			return val, orm.setError(
				fmt.Sprintf("Ensure the pointer passed to the %s operation is not nil; initialize it before passing.", opName),
				fmt.Errorf("%s - validateStructTags: NilPointerDereference", opName),
				http.StatusBadRequest,
			)
		}

		val = val.Elem()
	}

	return val, nil
}
