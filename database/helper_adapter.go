package database

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// EncodeMongoCollection converts a struct to a BSON document using gorest tags.
func (*Database) EncodeMongoCollection(dest any) (bson.M, string, int, error) {
	value := reflect.ValueOf(dest)
	if !value.IsValid() {
		message, httpCode, err := setError(
			"Pass a struct with valid gorest tags.",
			errors.New("encodeMongoCollection: destination is nil"),
			http.StatusBadRequest,
		)

		return nil, message, httpCode, err
	}

	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			message, httpCode, err := setError(
				"Pass a non-nil struct with valid gorest tags.",
				errors.New("encodeMongoCollection: destination is a nil pointer"),
				http.StatusBadRequest,
			)

			return nil, message, httpCode, err
		}

		value = value.Elem()
	}

	document, err := mongoDocumentByGorestTag(value)
	if err != nil {
		message, httpCode, wrappedErr := setError(
			"Pass a struct with unique, valid gorest tags.",
			fmt.Errorf("encodeMongoCollection: %w", err),
			http.StatusBadRequest,
		)

		return nil, message, httpCode, wrappedErr
	}

	return document, "", http.StatusOK, nil
}

func mongoDocumentByGorestTag(valueParam reflect.Value) (bson.M, error) {
	value := valueParam

	for value.Kind() == reflect.Pointer {
		if value.IsNil() {
			return nil, errors.New("document is a nil pointer")
		}

		value = value.Elem()
	}

	if value.Kind() != reflect.Struct {
		return nil, fmt.Errorf("expected a struct, got %s", value.Kind())
	}

	document := make(bson.M)
	typ := value.Type()

	for idx := 0; idx < value.NumField(); idx++ {
		field := typ.Field(idx)
		if field.PkgPath != "" {
			continue
		}

		tag := strings.TrimSpace(field.Tag.Get("gorest"))
		if tag == "" || tag == "-" {
			continue
		}

		name := strings.TrimSpace(strings.Split(tag, ",")[0])
		if name == "" || name == "-" {
			continue
		}

		if _, exists := document[name]; exists {
			return nil, fmt.Errorf("multiple struct fields use gorest tag %q", name)
		}

		fieldValue := value.Field(idx)
		if fieldValue.Kind() == reflect.Pointer && fieldValue.IsNil() {
			document[name] = nil

			continue
		}

		document[name] = fieldValue.Interface()
	}

	if len(document) == 0 {
		return nil, fmt.Errorf("struct %s has no fields with gorest tags", typ.Name())
	}

	return document, nil
}

// DecodeMongoCursor decodes MongoDB documents into a struct or slice of
// structs using the first value in each field's gorest tag as its BSON field
// name. The caller retains ownership of the cursor and should close it.
func (*Database) DecodeMongoCursor(ctx context.Context, cursor *mongo.Cursor, dest any) (string, int, error) {
	if ctx == nil || cursor == nil {
		return setError(
			"Ensure the MongoDB context and cursor are not nil.",
			errors.New("decodeMongoCursor: context or cursor is nil"),
			http.StatusBadRequest,
		)
	}

	destination, message, code, err := validateMongoCursorDestination(dest)
	if err != nil {
		return message, code, err
	}

	return decodeMongoCursorRows(ctx, cursor, destination)
}

type mongoCursorDestination struct {
	structType      reflect.Type
	value           reflect.Value
	isSlice         bool
	pointerElements bool
}

func validateMongoCursorDestination(dest any) (mongoCursorDestination, string, int, error) {
	destination := reflect.ValueOf(dest)
	if !destination.IsValid() || destination.Kind() != reflect.Pointer || destination.IsNil() {
		message, code, err := setError(
			"Pass a non-nil pointer to a struct or a slice of structs.",
			errors.New("decodeMongoCursor: destination must be a non-nil pointer"),
			http.StatusBadRequest,
		)

		return mongoCursorDestination{}, message, code, err
	}

	destination = destination.Elem()
	if destination.Kind() != reflect.Struct && destination.Kind() != reflect.Slice {
		message, code, err := setError(
			"Pass a pointer to a struct or a slice of structs.",
			fmt.Errorf("decodeMongoCursor: unsupported destination type %s", destination.Type()),
			http.StatusBadRequest,
		)

		return mongoCursorDestination{}, message, code, err
	}

	isSlice := destination.Kind() == reflect.Slice

	structType, pointerElements, err := mongoCursorStructType(destination)
	if err != nil {
		message, code, err := setError(
			"The destination slice must contain structs or pointers to structs.",
			err,
			http.StatusBadRequest,
		)

		return mongoCursorDestination{}, message, code, err
	}

	if isSlice && destination.IsNil() {
		destination.Set(reflect.MakeSlice(destination.Type(), 0, 0))
	}

	return mongoCursorDestination{
		value: destination, structType: structType, isSlice: isSlice, pointerElements: pointerElements,
	}, "", http.StatusOK, nil
}

func mongoCursorStructType(destination reflect.Value) (reflect.Type, bool, error) {
	switch destination.Kind() {
	case reflect.Struct:
		return destination.Type(), false, nil
	case reflect.Slice:
		elementType := destination.Type().Elem()
		pointerElements := elementType.Kind() == reflect.Pointer

		if pointerElements {
			elementType = elementType.Elem()
		}

		if elementType.Kind() != reflect.Struct {
			return nil, false, fmt.Errorf("decodeMongoCursor: unsupported destination element type %s", elementType)
		}

		return elementType, pointerElements, nil
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
		reflect.Array,
		reflect.Chan,
		reflect.Func,
		reflect.Interface,
		reflect.Map,
		reflect.Pointer,
		reflect.String,
		reflect.UnsafePointer:
	default:
		return nil, false, fmt.Errorf("decodeMongoCursor: unsupported destination element type %s", destination.Type())
	}

	return nil, false, fmt.Errorf("decodeMongoCursor: unsupported destination element type %s", destination.Type())
}

func decodeMongoCursorRows(
	ctx context.Context,
	cursor *mongo.Cursor,
	destination mongoCursorDestination,
) (string, int, error) {
	rowsAffected := 0

	for cursor.Next(ctx) {
		value, message, code, err := decodeMongoCursorDocument(cursor, destination.structType)
		if err != nil {
			return message, code, err
		}

		if !destination.isSlice {
			destination.value.Set(value)

			rowsAffected++

			break
		}

		appendMongoCursorValue(destination, value)

		rowsAffected++
	}

	if err := cursor.Err(); err != nil {
		return setError(
			"Failed while reading MongoDB query results.",
			fmt.Errorf("decodeMongoCursor: iterate cursor: %w", err),
		)
	}

	if !destination.isSlice && rowsAffected == 0 {
		return setError("MongoDB document was not found.", errors.New("decodeMongoCursor: record not found"), http.StatusNotFound)
	}

	return "", http.StatusOK, nil
}

func decodeMongoCursorDocument(cursor *mongo.Cursor, structType reflect.Type) (reflect.Value, string, int, error) {
	var doc bson.M
	if err := bson.Unmarshal(cursor.Current, &doc); err != nil {
		message, code, decodeErr := setError(
			"Failed to decode the MongoDB document.",
			fmt.Errorf("decodeMongoCursor: decode BSON document: %w", err),
		)

		return reflect.Value{}, message, code, decodeErr
	}

	value := reflect.New(structType).Elem()
	if err := assignMongoDocumentByGorestTag(value, doc); err != nil {
		message, code, decodeErr := setError(
			"Check that MongoDB document values match the destination struct field types and gorest tags.",
			err,
		)

		return reflect.Value{}, message, code, decodeErr
	}

	return value, "", http.StatusOK, nil
}

func appendMongoCursorValue(destination mongoCursorDestination, value reflect.Value) {
	if destination.pointerElements {
		pointer := reflect.New(destination.structType)
		pointer.Elem().Set(value)
		destination.value.Set(reflect.Append(destination.value, pointer))

		return
	}

	destination.value.Set(reflect.Append(destination.value, value))
}

func assignMongoDocumentByGorestTag(target reflect.Value, doc bson.M) error {
	fieldLookup := make(map[string]int, target.NumField())
	targetType := target.Type()

	for idx := 0; idx < target.NumField(); idx++ {
		field := targetType.Field(idx)
		if field.PkgPath != "" {
			continue
		}

		gorestTag := strings.TrimSpace(field.Tag.Get("gorest"))
		if gorestTag == "" || gorestTag == "-" {
			continue
		}

		column := strings.TrimSpace(strings.Split(gorestTag, ",")[0])
		if column != "" && column != "-" {
			fieldLookup[strings.ToLower(column)] = idx
		}
	}

	for column, raw := range doc {
		fieldIdx, found := fieldLookup[strings.ToLower(column)]
		if !found {
			continue
		}

		fieldValue := target.Field(fieldIdx)
		if err := assignMongoFieldValue(fieldValue, raw); err != nil {
			return fmt.Errorf("decodeMongoCursor: map field %q: %w", column, err)
		}
	}

	return nil
}

func assignMongoFieldValue(target reflect.Value, rawParam any) error {
	raw := rawParam

	if !target.CanSet() {
		return fmt.Errorf("field %s cannot be set", target.Type())
	}

	if raw == nil {
		target.SetZero()

		return nil
	}

	if target.Kind() == reflect.Pointer {
		return assignPointerValue(target, raw)
	}

	raw = preprocessRaw(target, raw)

	if done, err := trySpecialAssignments(target, raw); done {
		return err
	}

	return assignSourceValue(target, raw)
}

func assignPointerValue(target reflect.Value, raw any) error {
	value := reflect.New(target.Type().Elem())

	if err := assignMongoFieldValue(value.Elem(), raw); err != nil {
		return err
	}

	target.Set(value)

	return nil
}

func preprocessRaw(target reflect.Value, rawParam any) any {
	raw := rawParam

	if date, ok := raw.(bson.DateTime); ok {
		raw = date.Time()
	}

	if binary, ok := raw.(bson.Binary); ok && target.Kind() == reflect.Slice && target.Type().Elem().Kind() == reflect.Uint8 {
		raw = binary.Data
	}

	return raw
}

//nolint:unparam
func trySpecialAssignments(target reflect.Value, raw any) (bool, error) {
	if objectID, ok := raw.(bson.ObjectID); ok && target.Kind() == reflect.String {
		target.SetString(objectID.Hex())

		return true, nil
	}

	return false, nil
}

func assignSourceValue(target reflect.Value, raw any) error {
	source := reflect.ValueOf(raw)

	if source.Type().AssignableTo(target.Type()) {
		target.Set(source)

		return nil
	}

	if source.Type().ConvertibleTo(target.Type()) {
		target.Set(source.Convert(target.Type()))

		return nil
	}

	if target.Type() == reflect.TypeOf(time.Time{}) {
		if value, ok := raw.(time.Time); ok {
			target.Set(reflect.ValueOf(value))

			return nil
		}
	}

	if target.Kind() == reflect.Interface {
		target.Set(source)

		return nil
	}

	return fmt.Errorf("cannot assign BSON value of type %T to field type %s", raw, target.Type())
} //nolint:revive
