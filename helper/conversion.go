package helper

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// ConvertTimePointerToString converts a *time.Time pointer to its formatted string representation ("YYYY-MM-DD HH:MM:SS").
// Returns an empty string if the pointer is nil or the time is zero.
func ConvertTimePointerToString(data *time.Time) string {
	if data == nil || data.IsZero() {
		return ""
	}

	result := data.Format("2006-01-02 15:04:05")

	return result
}

// ConvertTimeToString converts a time.Time value to its formatted string representation ("YYYY-MM-DD HH:MM:SS").
// Returns an empty string if the time is zero.
func ConvertTimeToString(data time.Time) string {
	if data.IsZero() {
		return ""
	}

	result := data.Format("2006-01-02 15:04:05")

	return result
}

// ConvertTimePointerToStringPointer converts a *time.Time pointer to a formatted string pointer (*string) ("YYYY-MM-DD HH:MM:SS").
// Returns nil if the input pointer is nil or the time is zero.
func ConvertTimePointerToStringPointer(data *time.Time) *string {
	if data == nil || data.IsZero() {
		return nil
	}

	result := data.Format("2006-01-02 15:04:05")

	return &result
}

// ConvertTimeToStringPointer converts a time.Time value to a formatted string pointer (*string) ("YYYY-MM-DD HH:MM:SS").
// Returns nil if the time is zero.
func ConvertTimeToStringPointer(data time.Time) *string {
	if data.IsZero() {
		return nil
	}

	result := data.Format("2006-01-02 15:04:05")

	return &result
}

// ConvertStringToInt converts a string to an int.
// The name argument is the parameter name, used only to build the error message.
// Returns a 400 Bad Request with a client-friendly message if data is empty or not a valid number.
func ConvertStringToInt(name, data string) (res int, msg string, code int, err error) {
	if data == "" {
		return 0, fmt.Sprintf("%s must not be empty", name),
			http.StatusBadRequest,
			fmt.Errorf("string to int conversion error: %s cannot be empty", name)
	}

	result, err := strconv.Atoi(data)
	if err != nil {
		return 0, fmt.Sprintf("%s must be a valid number", name),
			http.StatusBadRequest,
			fmt.Errorf("string to int conversion error: %s: %w", name, err)
	}

	return result, "", http.StatusOK, nil
}

// ConvertStringPointerToInt converts a string pointer (*string) to an int.
// Use it for a required value: returns a 400 Bad Request if data is nil, empty, or not a valid number.
// The name argument is the parameter name, used only to build the error message.
func ConvertStringPointerToInt(name string, data *string) (res int, msg string, code int, err error) {
	if data == nil {
		return 0, fmt.Sprintf("%s is required", name),
			http.StatusBadRequest,
			fmt.Errorf("string to int conversion error: %s cannot be nil", name)
	}

	return ConvertStringToInt(name, *data)
}

// ConvertStringToIntPointer converts a string to an int pointer (*int).
// Use it for an optional value: returns nil without an error if data is empty.
// Returns a 400 Bad Request if data is not a valid number.
// The name argument is the parameter name, used only to build the error message.
func ConvertStringToIntPointer(name, data string) (res *int, msg string, code int, err error) {
	if data == "" {
		return nil, "", http.StatusOK, nil
	}

	result, msg, code, err := ConvertStringToInt(name, data)
	if err != nil {
		return nil, msg, code, err
	}

	return &result, "", http.StatusOK, nil
}

// ConvertStringPointerToIntPointer converts a string pointer (*string) to an int pointer (*int).
// Use it for an optional value: returns nil without an error if data is nil or empty.
// Returns a 400 Bad Request if data is not a valid number.
// The name argument is the parameter name, used only to build the error message.
func ConvertStringPointerToIntPointer(name string, data *string) (res *int, msg string, code int, err error) {
	if data == nil || *data == "" {
		return nil, "", http.StatusOK, nil
	}

	result, msg, code, err := ConvertStringToInt(name, *data)
	if err != nil {
		return nil, msg, code, err
	}

	return &result, "", http.StatusOK, nil
}
