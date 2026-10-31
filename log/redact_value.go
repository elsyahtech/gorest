//nolint:revive,nolintlint // Package log provides logging utilities for the application.
package log

import "reflect"

func redactValue(value reflect.Value, depth int) any {
	if !value.IsValid() {
		return nil
	}

	if depth > 16 {
		return redactedValue
	}

	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		return redactIndirect(value, depth)
	case reflect.String:
		return redactString(value.String())
	case reflect.Map:
		return redactMap(value, depth)
	case reflect.Slice:
		return redactSlice(value, depth)
	case reflect.Array:
		return redactElements(value, depth)
	case reflect.Struct:
		return redactStruct(value, depth)
	case reflect.Bool:
		return value.Bool()
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return value.Int()
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return value.Uint()
	case reflect.Float32, reflect.Float64:
		return value.Float()
	case reflect.Complex64, reflect.Complex128:
		return value.Complex()
	case reflect.Invalid:
		return nil
	case reflect.Chan, reflect.Func, reflect.UnsafePointer:
		return redactedValue
	default:
	}

	return redactFallback(value)
}
