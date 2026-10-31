package orm

import (
	"fmt"
	"net/http"
	"reflect"
	"strings"
)

func (orm *ORM) validateRequiredFields(value reflect.Value, structType reflect.Type, opName string) error {
	validateStruct := func(row reflect.Value, rowIndex int) error {
		row, isNil := dereferenceValue(row)
		if !isNil {
			return nil
		}

		if err := orm.validateRowRequiredFields(structType, row, rowIndex, opName); err != nil {
			return orm.Error
		}

		return nil
	}

	if value.Kind() == reflect.Slice {
		for index := 0; index < value.Len(); index++ {
			if err := validateStruct(value.Index(index), index+1); err != nil {
				return err
			}
		}

		return nil
	}

	return validateStruct(value, 0)
}

func dereferenceValue(rowParam reflect.Value) (reflect.Value, bool) {
	row := rowParam

	for row.Kind() == reflect.Pointer || row.Kind() == reflect.Interface {
		if row.IsNil() {
			return reflect.Value{}, false // Nil slice elements are reported by the existing payload validation.
		}

		row = row.Elem()
	}

	return row, true
}

func (orm *ORM) validateRowRequiredFields(
	structType reflect.Type,
	row reflect.Value,
	rowIndex int,
	opName string,
) error {
	for fieldIndex := 0; fieldIndex < structType.NumField(); fieldIndex++ {
		field, fieldValue, skip, err := orm.validateAndExtractRequiredField(structType, fieldIndex, row, rowIndex, opName)
		if err != nil {
			return orm.Error
		}

		if skip {
			continue
		}

		isEmpty := false

		switch fieldValue.Kind() {
		case reflect.String:
			isEmpty = strings.TrimSpace(fieldValue.String()) == ""
		case reflect.Slice, reflect.Map, reflect.Array:
			isEmpty = fieldValue.Len() == 0
		case reflect.Invalid,
			reflect.Bool,
			reflect.Int,
			reflect.Int8,
			reflect.Int16,
			reflect.Int32,
			reflect.Int64,
			reflect.Uint,
			reflect.Uint8,
			reflect.Uint16,
			reflect.Uint32,
			reflect.Uint64,
			reflect.Uintptr,
			reflect.Float32,
			reflect.Float64,
			reflect.Complex64,
			reflect.Complex128,
			reflect.Chan,
			reflect.Func,
			reflect.Interface,
			reflect.Pointer,
			reflect.Struct,
			reflect.UnsafePointer:
		default:
			continue
		}

		if isEmpty {
			return orm.requiredFieldError(field.Tag.Get("gorest"), rowIndex, opName)
		}
	}

	return nil
}

func (orm *ORM) validateAndExtractRequiredField(
	structType reflect.Type,
	fieldIndex int,
	row reflect.Value,
	rowIndex int,
	opName string,
) (field reflect.StructField, fieldValue reflect.Value, skip bool, err error) {
	field = structType.Field(fieldIndex)

	tagVal := field.Tag.Get("gorest")
	if strings.TrimSpace(strings.Split(tagVal, ",")[0]) == "-" ||
		!hasGorestTagOption(tagVal, "required") {
		return field, reflect.Value{}, true, nil
	}

	fieldValue = row.Field(fieldIndex)

	for fieldValue.Kind() == reflect.Pointer || fieldValue.Kind() == reflect.Interface {
		if fieldValue.IsNil() {
			return field, fieldValue, false, orm.requiredFieldError(tagVal, rowIndex, opName)
		}

		fieldValue = fieldValue.Elem()
	}

	return field, fieldValue, false, nil
}

func (orm *ORM) requiredFieldError(tag string, rowIndex int, opName string) error {
	column := strings.TrimSpace(strings.Split(tag, ",")[0])
	message := fmt.Sprintf("Field %q is required and must not be empty.", column)

	if rowIndex > 0 {
		message = fmt.Sprintf("Field %q is required and must not be empty (record %d).", column, rowIndex)
	}

	return orm.setError(
		message,
		fmt.Errorf("%s: required field %q is empty", strings.ToLower(opName), column),
		http.StatusBadRequest,
	)
}
