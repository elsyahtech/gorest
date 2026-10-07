package helper

import "time"

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
