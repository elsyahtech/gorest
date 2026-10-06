package orm

import "reflect"

type structInfo struct {
	structType  reflect.Type
	structValue *reflect.Value
}

func resolveStructInfo(val *reflect.Value) (*structInfo, bool) {
	valElem := val
	isSlice := valElem.Kind() == reflect.Slice

	var (
		sliceVal   reflect.Value
		structType reflect.Type
	)

	if isSlice {
		structType = valElem.Type().Elem()
		sliceVal = reflect.MakeSlice(valElem.Type(), 0, 0)

		if structType.Kind() == reflect.Pointer {
			structType = structType.Elem()
		}
	} else {
		structType = valElem.Type()
	}

	return &structInfo{
		structType:  structType,
		structValue: &sliceVal,
	}, isSlice
}
