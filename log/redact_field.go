//nolint:revive,nolintlint // Package log provides logging utilities for the application.
package log

import (
	"errors"
	"fmt"
	"reflect"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func redactField(field zap.Field) zap.Field {
	if isSensitiveKey(field.Key) {
		field.Type = zapcore.StringType
		field.String = redactedValue
		field.Integer = 0
		field.Interface = nil

		return field
	}

	switch field.Type {
	case zapcore.StringType:
		field.String = redactString(field.String)
	case zapcore.ByteStringType:
		bytes, ok := field.Interface.([]byte)
		if !ok {
			return field
		}

		field.Interface = []byte(redactString(string(bytes)))
	case zapcore.ErrorType:
		err, ok := field.Interface.(error)
		if !ok {
			return field
		}

		field.Interface = errors.New(redactString(err.Error()))
	case zapcore.StringerType:
		stringer, ok := field.Interface.(fmt.Stringer)
		if !ok {
			return field
		}

		field.Type = zapcore.StringType
		field.String = redactString(stringer.String())
		field.Interface = nil
	case zapcore.ReflectType, zapcore.ArrayMarshalerType, zapcore.ObjectMarshalerType, zapcore.InlineMarshalerType:
		if field.Interface == nil {
			return field
		}

		field.Type = zapcore.ReflectType
		field.Interface = redactValue(reflect.ValueOf(field.Interface), 0)
	case zapcore.UnknownType,
		zapcore.BinaryType,
		zapcore.BoolType,
		zapcore.Complex128Type,
		zapcore.Complex64Type,
		zapcore.DurationType,
		zapcore.Float64Type,
		zapcore.Float32Type,
		zapcore.Int64Type,
		zapcore.Int32Type,
		zapcore.Int16Type,
		zapcore.Int8Type,
		zapcore.TimeType,
		zapcore.TimeFullType,
		zapcore.Uint64Type,
		zapcore.Uint32Type,
		zapcore.Uint16Type,
		zapcore.Uint8Type,
		zapcore.UintptrType,
		zapcore.NamespaceType,
		zapcore.SkipType:
	default:
		return field
	}

	return field
}

func redactFields(fields []zap.Field) []zap.Field {
	redacted := make([]zap.Field, len(fields))

	for index, field := range fields {
		redacted[index] = redactField(field)
	}

	return redacted
}
