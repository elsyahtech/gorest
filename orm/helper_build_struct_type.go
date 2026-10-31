package orm

import (
	"fmt"
	"net/http"
	"reflect"
)

func (orm *ORM) buildStructTypeByElemInValidateStructTags(val reflect.Value, opName string) (reflect.Type, error) {
	elemType := val.Type().Elem()

	if elemType.Kind() == reflect.Pointer {
		elemType = elemType.Elem()
	}

	if elemType.Kind() != reflect.Struct {
		return nil, orm.setError(
			fmt.Sprintf("Ensure the slice contains structs, but found a slice of type '%s'.", elemType.Kind()),
			fmt.Errorf("%s - validateStructTags: "+
				"InvalidSliceElementType: expected slice of structs, got slice of %s", opName, elemType.Kind()),
			http.StatusBadRequest,
		)
	}

	structType := elemType

	return structType, nil
}
