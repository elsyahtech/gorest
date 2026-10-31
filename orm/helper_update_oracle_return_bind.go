package orm

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"
)

func buildOracleReturnBinds(plan *returnPlan, inputRow reflect.Value, meta columnMetaData) ([]oracleReturnBind, error) {
	_, destinationFields := taggedColumns(plan.destType)
	inputType := inputRow.Type()
	binds := make([]oracleReturnBind, 0, len(plan.queryCols))

	for _, column := range plan.queryCols {
		destinationIdx := -1
		if idx, exists := destinationFields[strings.ToLower(column)]; exists {
			destinationIdx = idx
		}

		fieldType, err := oracleReturnFieldType(plan, inputType, meta, column, destinationIdx)
		if err != nil {
			return nil, err
		}

		fieldType = dereferenceOracleOutputType(fieldType)
		if !isOracleOutputBindType(fieldType) {
			return nil, fmt.Errorf("return column %q has unsupported output type %s", column, fieldType)
		}

		binds = append(binds, oracleReturnBind{
			column:      column,
			destination: destinationIdx,
			holder:      reflect.New(fieldType).Elem(),
		})
	}

	if len(binds) == 0 {
		return nil, errors.New("return() has no columns to bind")
	}

	return binds, nil
}

func oracleReturnFieldType(
	plan *returnPlan,
	inputType reflect.Type,
	meta columnMetaData,
	column string,
	destinationIdx int,
) (reflect.Type, error) {
	if destinationIdx >= 0 {
		return plan.destType.Field(destinationIdx).Type, nil
	}

	inputIdx, exists := oracleInputColumnIndex(meta.allColumnPrimaryKeyIdx, column)
	if !exists {
		return nil, fmt.Errorf("return column %q has no matching tagged field", column)
	}

	return inputType.Field(inputIdx).Type, nil
}

func oracleInputColumnIndex(columns map[string]int, column string) (int, bool) {
	if idx, exists := columns[column]; exists {
		return idx, true
	}

	for inputColumn, idx := range columns {
		if strings.EqualFold(inputColumn, column) {
			return idx, true
		}
	}

	return 0, false
}

func dereferenceOracleOutputType(typ reflect.Type) reflect.Type {
	dereferenced := typ
	for dereferenced.Kind() == reflect.Pointer {
		dereferenced = dereferenced.Elem()
	}

	return dereferenced
}

func isOracleOutputBindType(typ reflect.Type) bool {
	if typ == reflect.TypeOf((*time.Time)(nil)).Elem() {
		return true
	}

	switch typ.Kind() {
	case reflect.String, reflect.Bool,
		reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64:
		return true
	case reflect.Slice:
		return typ.Elem().Kind() == reflect.Uint8
	case reflect.Invalid, reflect.Uintptr, reflect.Complex64, reflect.Complex128,
		reflect.Array, reflect.Chan, reflect.Func, reflect.Interface, reflect.Map,
		reflect.Pointer, reflect.Struct, reflect.UnsafePointer:
		return false
	}

	return false
}
