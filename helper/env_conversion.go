package helper

import (
	"crypto/tls"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// ReadTrimmedEnvironmentVariable reads an environment variable by its name (key)
// and removes leading and trailing whitespace from the value.
// If the variable is not set, the result is an empty string.
// Use this for values where surrounding whitespace is not significant.
func ReadTrimmedEnvironmentVariable(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

// ReadEnvironmentNonNegativeInteger reads an environment variable as a whole number
// that must not be negative (0 or greater).
// If the variable is empty or unset, it returns 0 with no error.
// If the value is not a number or is negative, it returns an error.
// Examples: "10" -> 10, "-5" -> error, "abc" -> error.
func ReadEnvironmentNonNegativeInteger(key string) (int, error) {
	value := ReadTrimmedEnvironmentVariable(key)
	if value == "" {
		return 0, nil
	}

	number, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a non-negative integer: %w", key, err)
	}
	if number < 0 {
		return 0, fmt.Errorf("%s must not be negative", key)
	}

	return number, nil
}

// ReadEnvironmentFileMode reads an environment variable as a file permission mode
// written in octal, the same format used by the chmod command on Linux/Unix.
// If the variable is empty or unset, it returns 0 with no error (meaning: not configured).
// Valid values range from 0000 to 0777.
// Examples: "644" -> rw-r--r--, "755" -> rwxr-xr-x, "999" -> error.
func ReadEnvironmentFileMode(key string) (os.FileMode, error) {
	value := ReadTrimmedEnvironmentVariable(key)
	if value == "" {
		return 0, nil
	}

	mode, err := strconv.ParseUint(value, 8, 32)
	if err != nil || mode > 0o777 {
		return 0, fmt.Errorf("%s must be an octal permission mode between 0000 and 0777", key)
	}

	return os.FileMode(mode), nil
}

// ReadEnvironmentTLSMinimumVersion reads an environment variable as the minimum
// TLS version allowed for secure connections (HTTPS).
// Accepted values (case-insensitive):
//   - TLS 1.2: "1.2", "tls1.2", "tlsv1.2"
//   - TLS 1.3: "1.3", "tls1.3", "tlsv1.3"
//
// If the variable is empty or unset, it returns 0 with no error (meaning: use Go's default).
// Any other value returns an error.
func ReadEnvironmentTLSMinimumVersion(key string) (uint16, error) {
	switch strings.ToLower(ReadTrimmedEnvironmentVariable(key)) {
	case "":
		return 0, nil
	case "1.2", "tls1.2", "tlsv1.2":
		return tls.VersionTLS12, nil
	case "1.3", "tls1.3", "tlsv1.3":
		return tls.VersionTLS13, nil
	default:
		return 0, fmt.Errorf("%s must be 1.2 or 1.3", key)
	}
}

// ReadEnvironmentStringWithDefault reads an environment variable as text without
// trimming it. If the variable is unset or empty, it returns defaultValue.
func ReadEnvironmentStringWithDefault(key, defaultValue string) string {
	value, isSet := os.LookupEnv(key)
	if !isSet || value == "" {
		return defaultValue
	}

	return value
}

// ReadEnvironmentStringPointer reads an environment variable as text without
// trimming it. It returns nil when the variable is unset and a pointer to its
// value when set, including when the value is empty.
func ReadEnvironmentStringPointer(key string) *string {
	value, isSet := os.LookupEnv(key)
	if !isSet {
		return nil
	}

	return &value
}

// ReadEnvironmentStringPointerWithDefault behaves like ReadEnvironmentStringWithDefault,
// but returns a pointer to the string (*string).
// The returned pointer is always non-nil, including when defaultValue is used.
func ReadEnvironmentStringPointerWithDefault(key, defaultValue string) *string {
	value := ReadEnvironmentStringWithDefault(key, defaultValue)

	return &value
}

// ReadEnvironmentIntegerWithDefault reads an environment variable as a whole number.
// If the variable is empty or unset, it returns defaultValue. An invalid value
// returns defaultValue and an error; callers should not silently ignore the error.
func ReadEnvironmentIntegerWithDefault(key string, defaultValue int) (int, error) {
	value := ReadTrimmedEnvironmentVariable(key)
	if value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue, fmt.Errorf("%s must be a valid integer: %w", key, err)
	}

	return parsed, nil
}

// ReadEnvironmentDurationWithDefault reads an environment variable as a time duration
// using Go's duration format, for example "30s" (30 seconds), "5m" (5 minutes),
// or "1h30m" (1.5 hours).
// If the variable is empty or unset, it returns defaultValue with no error.
// If the format is invalid or the duration is negative, it returns an error
// (along with defaultValue as a fallback value).
func ReadEnvironmentDurationWithDefault(key string, defaultValue time.Duration) (time.Duration, error) {
	value := ReadTrimmedEnvironmentVariable(key)
	if value == "" {
		return defaultValue, nil
	}

	duration, err := time.ParseDuration(value)
	if err != nil {
		return defaultValue, fmt.Errorf("%s must be a valid Go duration: %w", key, err)
	}

	if duration < 0 {
		return defaultValue, fmt.Errorf("%s cannot be negative", key)
	}

	return duration, nil
}

// ReadEnvironmentStringSliceWithDefault reads an environment variable as a slice of
// strings from comma-separated values, for example "a, b, c" -> []string{"a", "b", "c"}.
// Whitespace around each item is removed, and empty items are ignored.
//
// Behavior:
//   - Variable is not set at all: returns defaultValue.
//   - Variable is set but empty: returns an empty, non-nil slice (not defaultValue),
//     so users can intentionally clear a built-in default slice.
func ReadEnvironmentStringSliceWithDefault(key string, defaultValue []string) []string {
	value, isSet := os.LookupEnv(key)
	if !isSet {
		if defaultValue == nil {
			return nil
		}

		result := make([]string, len(defaultValue))
		copy(result, defaultValue)

		return result
	}

	value = strings.TrimSpace(value)
	if value == "" {
		return make([]string, 0)
	}

	items := strings.Split(value, ",")
	result := make([]string, 0, len(items))

	for _, item := range items {
		if trimmedItem := strings.TrimSpace(item); trimmedItem != "" {
			result = append(result, trimmedItem)
		}
	}

	return result
}

// ReadEnvironmentBooleanWithDefault reads an environment variable as a true/false value.
// Accepted values (as defined by strconv.ParseBool):
//   - true : "1", "t", "T", "true", "TRUE", "True"
//   - false: "0", "f", "F", "false", "FALSE", "False"
//
// If the variable is empty or unset, it returns defaultValue with no error.
// If the value is not recognized, it returns an error.
func ReadEnvironmentBooleanWithDefault(key string, defaultValue bool) (bool, error) {
	value := ReadTrimmedEnvironmentVariable(key)
	if value == "" {
		return defaultValue, nil
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return defaultValue, fmt.Errorf("%s must be a boolean: %w", key, err)
	}

	return parsed, nil
}

// ReadEnvironmentBooleanPointer reads an environment variable as a bool
// pointer. It returns nil when the variable is unset or empty, and an error if
// a configured non-empty value is not a valid boolean.
func ReadEnvironmentBooleanPointer(key string) (*bool, error) {
	value := ReadTrimmedEnvironmentVariable(key)
	if value == "" {
		return nil, nil
	}

	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return nil, fmt.Errorf("%s must be a boolean: %w", key, err)
	}

	return &parsed, nil
}

// ValidateApplicationEnvironmentMode rejects an empty application environment
// mode. GOREST also accepts custom non-empty labels such as "staging" and
// "uat"; these use the production-like transport security policy.
func ValidateApplicationEnvironmentMode(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("GOREST_APP_ENV must not be empty; common values are development, testing, and production")
	}

	return nil
}
