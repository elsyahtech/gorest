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

// DecodeMongoCursor decodes MongoDB documents into a struct or slice of
// structs using the first value in each field's gorest tag as its BSON field
// name. The caller retains ownership of the cursor and should close it.
func (*Database) DecodeMongoCursor(ctx context.Context, cursor *mongo.Cursor, dest any) (string, int, error) {
	if ctx == nil || cursor == nil {
		return setError(
			"Ensure the MongoDB context and cursor are not nil.",
			fmt.Errorf("decodeMongoCursor: context or cursor is nil"),
			http.StatusBadRequest,
		)
	}

	destination := reflect.ValueOf(dest)
	if !destination.IsValid() || destination.Kind() != reflect.Pointer || destination.IsNil() {
		return setError(
			"Pass a non-nil pointer to a struct or a slice of structs.",
			fmt.Errorf("decodeMongoCursor: destination must be a non-nil pointer"),
			http.StatusBadRequest,
		)
	}

	destination = destination.Elem()
	if destination.Kind() != reflect.Struct && destination.Kind() != reflect.Slice {
		return setError(
			"Pass a pointer to a struct or a slice of structs.",
			fmt.Errorf("decodeMongoCursor: unsupported destination type %s", destination.Type()),
			http.StatusBadRequest,
		)
	}

	isSlice := destination.Kind() == reflect.Slice
	structType := destination.Type()
	if isSlice {
		structType = structType.Elem()
		if structType.Kind() == reflect.Pointer {
			structType = structType.Elem()
		}
	}
	if structType.Kind() != reflect.Struct {
		return setError(
			"The destination slice must contain structs or pointers to structs.",
			fmt.Errorf("decodeMongoCursor: unsupported destination element type %s", structType),
			http.StatusBadRequest,
		)
	}

	if isSlice && destination.IsNil() {
		destination.Set(reflect.MakeSlice(destination.Type(), 0, 0))
	}

	rowsAffected := 0
	for cursor.Next(ctx) {
		var doc bson.M
		if err := bson.Unmarshal(cursor.Current, &doc); err != nil {
			return setError(
				"Failed to decode the MongoDB document.",
				fmt.Errorf("decodeMongoCursor: decode BSON document: %w", err),
			)
		}

		value := reflect.New(structType).Elem()
		if err := assignMongoDocumentByGorestTag(value, doc); err != nil {
			return setError(
				"Check that MongoDB document values match the destination struct field types and gorest tags.",
				err,
			)
		}

		if !isSlice {
			destination.Set(value)
			rowsAffected++
			break
		}

		if destination.Type().Elem().Kind() == reflect.Pointer {
			ptr := reflect.New(structType)
			ptr.Elem().Set(value)
			destination.Set(reflect.Append(destination, ptr))
		} else {
			destination.Set(reflect.Append(destination, value))
		}
		rowsAffected++
	}

	if err := cursor.Err(); err != nil {
		return setError(
			"Failed while reading MongoDB query results.",
			fmt.Errorf("decodeMongoCursor: iterate cursor: %w", err),
		)
	}

	if !isSlice && rowsAffected == 0 {
		return setError("MongoDB document was not found.", errors.New("decodeMongoCursor: record not found"), http.StatusNotFound)
	}

	return "", http.StatusOK, nil
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

func assignMongoFieldValue(target reflect.Value, raw any) error {
	if !target.CanSet() {
		return fmt.Errorf("field %s cannot be set", target.Type())
	}

	if raw == nil {
		target.SetZero()
		return nil
	}

	if target.Kind() == reflect.Pointer {
		value := reflect.New(target.Type().Elem())
		if err := assignMongoFieldValue(value.Elem(), raw); err != nil {
			return err
		}
		target.Set(value)
		return nil
	}

	if date, ok := raw.(bson.DateTime); ok {
		raw = date.Time()
	}
	if objectID, ok := raw.(bson.ObjectID); ok && target.Kind() == reflect.String {
		target.SetString(objectID.Hex())
		return nil
	}
	if binary, ok := raw.(bson.Binary); ok && target.Kind() == reflect.Slice && target.Type().Elem().Kind() == reflect.Uint8 {
		raw = binary.Data
	}

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
}
