package orm

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
)

func (orm *ORM) normalizeStructInput(val reflect.Value, opName string) ([]reflect.Value, error) {
	var rowsVal []reflect.Value

	//nolint:exhaustive
	switch val.Kind() {
	case reflect.Slice:
		for idx := 0; idx < val.Len(); idx++ {
			elem, err := orm.validateSliceStructInput(idx, val, opName)
			if err != nil {
				return nil, orm.Error
			}

			rowsVal = append(rowsVal, *elem)
		}

	case reflect.Struct:
		rowsVal = append(rowsVal, val)

	default:
		var actionType string

		switch opName {
		case create:
			actionType = "Create"
		default:
			actionType = "Delete"
		}

		return nil, orm.setError(
			"ensure the data passed is a struct, slice of structs, or pointers to them",
			fmt.Errorf("%s: invalid data type for %s", opName, actionType),
		)
	}

	return rowsVal, nil
}

func (orm *ORM) validateSliceStructInput(idx int, val reflect.Value, opName string) (*reflect.Value, error) {
	elem := val.Index(idx)

	if elem.Kind() == reflect.Pointer {
		if elem.IsNil() {
			return nil, orm.setError(
				"Ensure the data list does not contain nil elements",
				fmt.Errorf("%s: data contains nil element", opName),
			)
		}

		elem = elem.Elem()
	}

	if elem.Kind() != reflect.Struct {
		return nil, orm.setError(
			"ensure the data list contains only structs or pointers to structs",
			fmt.Errorf("%s: invalid data element type", opName),
		)
	}

	return &elem, nil
}

func (orm *ORM) normalizeStructInputMongo(val reflect.Value, opName string) ([]any, error) {
	var normalizedSelectedCols []string

	for _, col := range orm.SelectedCols {
		for _, part := range strings.Split(col, ",") {
			trimmed := strings.TrimSpace(part)

			if trimmed != "" {
				normalizedSelectedCols = append(normalizedSelectedCols, trimmed)
			}
		}
	}

	// 8. Initialize a slice to hold normalized interface values for InsertOne / InsertMany.
	var rowsVal []any

	filterStructFields := func(val reflect.Value) map[string]any {
		typ := val.Type()
		filteredMap := make(map[string]any)

		for idx := 0; idx < typ.NumField(); idx++ {
			columnName, isOk := findColumnNameNormalizeStructInputMongo(typ, idx, normalizedSelectedCols)
			if !isOk {
				continue
			}

			filteredMap[columnName] = val.Field(idx).Interface()
		}

		return filteredMap
	}

	// 9. Evaluate the resolved input as either a struct or a slice.
	//nolint:exhaustive
	switch val.Kind() {
	case reflect.Slice:
		for idx := 0; idx < val.Len(); idx++ {
			elem, err := orm.validateSliceStructInput(idx, val, opName)
			if err != nil {
				return nil, orm.Error
			}

			// Apply the "select columns" filter if available.
			filtered := filterStructFields(*elem)

			rowsVal = append(rowsVal, filtered)
		}

	case reflect.Struct:
		filtered := filterStructFields(val)

		rowsVal = append(rowsVal, filtered)

	default:
		return nil, orm.setError(
			"ensure the data passed is a struct, slice of structs, or pointers to them",
			errors.New("create: invalid data type for insertion"),
		)
	}

	return rowsVal, nil
}

func findColumnNameNormalizeStructInputMongo(typ reflect.Type, idx int, normalizedSelectedCols []string) (string, bool) {
	field := typ.Field(idx)
	tag := field.Tag.Get("bson")

	if tag == "" {
		tag = field.Tag.Get("gorest")
	}

	if tag == "" || tag == "-" {
		return "", false
	}

	// Get the field name from the tag (ignoring additional options like omitempty)
	columnName, isFound := getFieldNameFromTag(tag, field.Name, normalizedSelectedCols)
	if !isFound {
		return columnName, isFound
	}

	return columnName, true
}

func getFieldNameFromTag(tag, fieldName string, normalizedSelectedCols []string) (string, bool) {
	parts := strings.Split(tag, ",")
	columnName := strings.TrimSpace(parts[0])

	if columnName == "" || columnName == "-" {
		columnName = fieldName
	}

	// If there is a .Select(...), check whether this field is among those selected.
	if len(normalizedSelectedCols) > 0 {
		columnName, isFound := findSelectedColsNormalizeStructInputMongo(
			normalizedSelectedCols, columnName, fieldName,
		)
		if !isFound {
			return columnName, isFound
		}
	}

	return columnName, true
}

func findSelectedColsNormalizeStructInputMongo(normalizedSelectedCols []string, columnName, fieldName string) (string, bool) {
	found := false

	for _, selected := range normalizedSelectedCols {
		// Match with the struct field name
		if strings.EqualFold(selected, columnName) || strings.EqualFold(selected, fieldName) {
			found = true

			break
		}
	}

	if !found {
		return columnName, false
	}

	return columnName, found
}
