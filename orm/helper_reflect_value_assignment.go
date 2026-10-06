package orm

import (
	"errors"
	"fmt"
	"reflect"
)

// assignReflectValue assigns a value to a reflect.Value field with type conversion.
// Handles direct assignment, string conversion from bytes, int-to-bool conversion, and convertible types.
func assignReflectValue(target reflect.Value, raw any) error {
	if err := guardAssignReflectValue(target); err != nil {
		return fmt.Errorf("%w", err)
	}

	if raw == nil {
		return nil
	}

	source := reflect.ValueOf(raw)

	if err := reflectNonPointerInAssignReflectValue(target, source, raw); err != nil {
		return fmt.Errorf("%w", err)
	}

	// Pointer assignment with conversion
	if target.Kind() == reflect.Pointer {
		if err := reflectPointerInAssignReflectValue(target, source, raw); err != nil {
			return fmt.Errorf("%w", err)
		}
	}

	return fmt.Errorf("cannot assign database value of type %T to field type %s", raw, target.Type())
}

func reflectNonPointerInAssignReflectValue(target, source reflect.Value, raw any) error {
	// Direct assignment without conversion
	if source.Type().AssignableTo(target.Type()) {
		target.Set(source)

		return nil
	}

	// Type conversion
	if source.Type().ConvertibleTo(target.Type()) {
		target.Set(source.Convert(target.Type()))

		return nil
	}

	// String conversion from bytes
	if target.Kind() == reflect.String {
		if str, ok := toString(raw); ok {
			target.SetString(str)

			return nil
		}
	}

	// Common scenario: is_active INT(1) in database → IsActive bool in struct
	if target.Kind() == reflect.Bool {
		if bol, ok := toBool(raw); ok {
			target.SetBool(bol)

			return nil
		}
	}

	return fmt.Errorf("cannot assign database value of type %T to field type %s", raw, target.Type())
}

func reflectPointerInAssignReflectValue(target, source reflect.Value, raw any) error {
	elemType := target.Type().Elem()

	if source.Type().AssignableTo(elemType) {
		ptr := reflect.New(target.Type().Elem())

		ptr.Elem().Set(source)

		target.Set(ptr)

		return nil
	}

	if source.Type().ConvertibleTo(target.Type().Elem()) {
		ptr := reflect.New(elemType)

		ptr.Elem().Set(source.Convert(target.Type().Elem()))

		target.Set(ptr)

		return nil
	}

	// ─── POINTER-TO-BOOL CONVERSION ───
	if elemType.Kind() == reflect.Bool {
		if bol, ok := toBool(raw); ok {
			ptr := reflect.New(reflect.TypeOf(bol))

			ptr.Elem().SetBool(bol)

			target.Set(ptr)

			return nil
		}
	}

	return fmt.Errorf("cannot assign database value of type %T to field type %s", raw, target.Type())
}

func guardAssignReflectValue(target reflect.Value) error {
	if !target.IsValid() {
		return errors.New("target field is invalid")
	}

	if !target.CanSet() {
		return errors.New("target field cannot be set")
	}

	return nil
}

func toString(raw any) (string, bool) {
	switch v := raw.(type) {
	case string:
		return v, true
	case []byte:
		return string(v), true
	default:
		return "", false
	}
}

func toInt64(raw any) (int64, bool) {
	switch value := raw.(type) {
	case int:
		return int64(value), true
	case int8:
		return int64(value), true
	case int16:
		return int64(value), true
	case int32:
		return int64(value), true
	case int64:
		return value, true
	case uint:
		return int64(value), true
	case uint8:
		return int64(value), true
	case uint16:
		return int64(value), true
	case uint32:
		return int64(value), true
	case uint64:
		return int64(value), true
	default:
		return 0, false
	}
}

func toBool(raw any) (val bool, success bool) {
	if idx, ok := toInt64(raw); ok {
		return idx != 0, true
	}

	return false, false
}
