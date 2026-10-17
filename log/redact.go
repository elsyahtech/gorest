package log

import (
	"errors"
	"fmt"
	"reflect"
	"regexp"
	"strings"
	"unicode"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

const redactedValue = "xxxxx"

//nolint:revive
var (
	cookieHeaderPattern        = regexp.MustCompile(`(?i)(\b(?:cookie|set-cookie)\b\s*[:=]\s*)[^\r\n]+`)
	sensitiveAssignmentPattern = regexp.MustCompile(`(?i)(\b(?:password|passwd|pwd|token|access[_ -]?token|refresh[_ -]?token|secret|credential|authorization|proxy[_ -]?authorization|cookie|set-cookie|session(?:[_ -]?(?:id|key|token))?|api[_ -]?key|private[_ -]?key|client[_ -]?secret|ssn|social[_ -]?security|credit[_ -]?card|card[_ -]?(?:number|no)|cvv|cvc)\b\s*[:=]\s*)(?:"[^"]*"|'[^']*'|[^\s,;&]+)`)
	bearerPattern              = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/=-]+`)
	jwtPattern                 = regexp.MustCompile(`\b[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\b`)
	emailPattern               = regexp.MustCompile(`(?i)\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b`)
	phonePattern               = regexp.MustCompile(`\+?[0-9][0-9(). -]{8,}[0-9]`)
)

type redactingCore struct {
	zapcore.Core
}

func (core redactingCore) With(fields []zap.Field) zapcore.Core {
	return redactingCore{Core: core.Core.With(redactFields(fields))}
}

func (core redactingCore) Check(entry zapcore.Entry, checked *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if core.Enabled(entry.Level) {
		return checked.AddCore(entry, core)
	}

	return checked
}

func (core redactingCore) Write(entry zapcore.Entry, fields []zap.Field) error {
	entry.Message = redactString(entry.Message)

	return core.Core.Write(entry, redactFields(fields)) //nolint:wrapcheck
}

func redactFields(fields []zap.Field) []zap.Field {
	redacted := make([]zap.Field, len(fields))

	for index, field := range fields {
		redacted[index] = redactField(field)
	}

	return redacted
}

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
		if bytes, ok := field.Interface.([]byte); ok {
			field.Interface = []byte(redactString(string(bytes)))
		}
	case zapcore.ErrorType:
		if err, ok := field.Interface.(error); ok {
			field.Interface = errors.New(redactString(err.Error()))
		}
	case zapcore.StringerType:
		if stringer, ok := field.Interface.(fmt.Stringer); ok {
			field.Type = zapcore.StringType
			field.String = redactString(stringer.String())
			field.Interface = nil
		}
	case zapcore.ReflectType, zapcore.ArrayMarshalerType, zapcore.ObjectMarshalerType, zapcore.InlineMarshalerType:
		if field.Interface != nil {
			field.Type = zapcore.ReflectType
			field.Interface = redactValue(reflect.ValueOf(field.Interface), 0)
		}
	default:
	}

	return field
}

func redactString(value string) string {
	value = cookieHeaderPattern.ReplaceAllString(value, "${1}"+redactedValue)
	value = sensitiveAssignmentPattern.ReplaceAllString(value, "${1}"+redactedValue)
	value = bearerPattern.ReplaceAllString(value, "${1} "+redactedValue)
	value = jwtPattern.ReplaceAllString(value, redactedValue)
	value = emailPattern.ReplaceAllString(value, redactedValue)

	return phonePattern.ReplaceAllStringFunc(value, func(candidate string) string {
		digitCount := 0

		for _, char := range candidate {
			if unicode.IsDigit(char) {
				digitCount++
			}
		}

		if digitCount >= 10 && digitCount <= 15 {
			return redactedValue
		}

		return candidate
	})
}

func isSensitiveKey(key string) bool {
	var normalized strings.Builder

	for _, char := range strings.ToLower(key) {
		if unicode.IsLetter(char) || unicode.IsDigit(char) {
			normalized.WriteRune(char)
		}
	}

	normalizedKey := normalized.String()

	for _, marker := range []string{
		"password", "passwd", "pwd", "token", "secret", "credential",
		"authorization", "cookie", "session", "sessionid", "sessionkey", "sessiontoken",
		"apikey", "privatekey", "clientsecret", "ssn", "socialsecurity",
		"creditcard", "cardnumber", "cardno", "cvv", "cvc",
	} {
		if strings.Contains(normalizedKey, marker) {
			return true
		}
	}

	//nolint:revive
	return normalizedKey == "email" ||
		strings.HasSuffix(normalizedKey, "email") ||
		strings.HasSuffix(normalizedKey, "mailaddress") ||
		normalizedKey == "phone" ||
		strings.HasSuffix(normalizedKey, "phone") ||
		normalizedKey == "mobile" ||
		strings.HasSuffix(normalizedKey, "mobile")
}

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
	}

	return redactFallback(value)
}

func redactIndirect(value reflect.Value, depth int) any {
	if value.IsNil() {
		return nil
	}

	return redactValue(value.Elem(), depth+1)
}

func redactEntry(key string, item reflect.Value, depth int) any {
	if isSensitiveKey(key) {
		return redactedValue
	}

	return redactValue(item, depth+1)
}

func redactMap(value reflect.Value, depth int) any {
	if value.IsNil() {
		return nil
	}

	result := make(map[string]any, value.Len())
	iter := value.MapRange()

	for iter.Next() {
		keyString := fmt.Sprint(iter.Key().Interface())

		result[redactString(keyString)] = redactEntry(keyString, iter.Value(), depth)
	}

	return result
}

func redactSlice(value reflect.Value, depth int) any {
	if value.IsNil() {
		return nil
	}

	if value.Type().Elem().Kind() == reflect.Uint8 {
		return redactString(string(value.Bytes()))
	}

	return redactElements(value, depth)
}

func redactElements(value reflect.Value, depth int) []any {
	result := make([]any, value.Len())

	for index := 0; index < value.Len(); index++ {
		result[index] = redactValue(value.Index(index), depth+1)
	}

	return result
}

func redactStruct(value reflect.Value, depth int) any {
	if text, ok := structAsText(value); ok {
		return redactString(text)
	}

	result := make(map[string]any, value.NumField())

	for index := 0; index < value.NumField(); index++ {
		fieldType := value.Type().Field(index)

		if fieldType.PkgPath != "" {
			continue
		}

		key := structFieldKey(fieldType)

		result[redactString(key)] = redactEntry(key, value.Field(index), depth)
	}

	return result
}

func structAsText(value reflect.Value) (string, bool) {
	if !value.CanInterface() {
		return "", false
	}

	if err, ok := value.Interface().(error); ok {
		return err.Error(), true
	}

	if stringer, ok := value.Interface().(fmt.Stringer); ok {
		return stringer.String(), true
	}

	return "", false
}

func structFieldKey(fieldType reflect.StructField) string {
	if jsonTag := strings.Split(fieldType.Tag.Get("json"), ",")[0]; jsonTag != "" && jsonTag != "-" {
		return jsonTag
	}

	return fieldType.Name
}

func redactFallback(value reflect.Value) any {
	if !value.CanInterface() {
		return redactedValue
	}

	if stringer, ok := value.Interface().(fmt.Stringer); ok {
		return redactString(stringer.String())
	}

	if err, ok := value.Interface().(error); ok {
		return redactString(err.Error())
	}

	return redactString(fmt.Sprint(value.Interface()))
}
