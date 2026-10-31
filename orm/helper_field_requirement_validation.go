package orm

import (
	"fmt"
	"net/http"
	"reflect"
)

func (orm *ORM) verifFieldRequirementValidateStructTags(requirement fieldRequirement, val reflect.Value, opName string) error {
	if requirement.requireNonEmpty {
		if val.Len() == 0 {
			return orm.setError(
				"Ensure the provided slice contains at least one record/element to perform this operation.",
				fmt.Errorf("%s - validateStructTags: EmptySliceProvided", opName),
				http.StatusBadRequest,
			)
		}

		for idx := 0; idx < val.Len(); idx++ {
			if val.Index(idx).Kind() == reflect.Pointer && val.Index(idx).IsNil() {
				return orm.setError(
					fmt.Sprintf("Ensure element at index %d in the slice is not a nil pointer.", idx),
					fmt.Errorf("%s - validateStructTags: NilElementInSlice", opName),
					http.StatusBadRequest,
				)
			}
		}
	}

	return nil
}
