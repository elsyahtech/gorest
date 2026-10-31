package helper

import (
	"net/http"
	"testing"
	"time"
)

func TestTimeConversions(t *testing.T) {
	value := time.Date(2025, time.January, 2, 3, 4, 5, 0, time.UTC)
	want := "2025-01-02 03:04:05"

	if got := ConvertTimePointerToString(nil); got != "" {
		t.Errorf("ConvertTimePointerToString(nil) = %q, want empty", got)
	}
	if got := ConvertTimePointerToString(&value); got != want {
		t.Errorf("ConvertTimePointerToString() = %q, want %q", got, want)
	}
	if got := ConvertTimePointerToStringPointer(nil); got != nil {
		t.Errorf("ConvertTimePointerToStringPointer(nil) = %v, want nil", got)
	}
	if got := ConvertTimePointerToStringPointer(&value); got == nil || *got != want {
		t.Errorf("ConvertTimePointerToStringPointer() = %v, want %q", got, want)
	}
	if got := ConvertTimeToString(time.Time{}); got != "" {
		t.Errorf("ConvertTimeToString(zero) = %q, want empty", got)
	}
	if got := ConvertTimeToString(value); got != want {
		t.Errorf("ConvertTimeToString() = %q, want %q", got, want)
	}
	if got := ConvertTimeToStringPointer(time.Time{}); got != nil {
		t.Errorf("ConvertTimeToStringPointer(zero) = %v, want nil", got)
	}
	if got := ConvertTimeToStringPointer(value); got == nil || *got != want {
		t.Errorf("ConvertTimeToStringPointer() = %v, want %q", got, want)
	}
}

func TestStringToIntConversions(t *testing.T) {
	tests := []struct {
		name string
		call func() (int, string, int, error)
		want int
		code int
	}{
		{name: "string value", call: func() (int, string, int, error) {
			return ConvertStringToInt("limit", "42")
		}, want: 42, code: http.StatusOK},
		{name: "empty string", call: func() (int, string, int, error) {
			return ConvertStringToInt("limit", "")
		}, code: http.StatusBadRequest},
		{name: "invalid string", call: func() (int, string, int, error) {
			return ConvertStringToInt("limit", "not-a-number")
		}, code: http.StatusBadRequest},
		{name: "pointer value", call: func() (int, string, int, error) {
			value := "7"
			return ConvertStringPointerToInt("page", &value)
		}, want: 7, code: http.StatusOK},
		{name: "nil pointer", call: func() (int, string, int, error) {
			return ConvertStringPointerToInt("page", nil)
		}, code: http.StatusBadRequest},
		{name: "empty pointer value", call: func() (int, string, int, error) {
			value := ""
			return ConvertStringPointerToInt("page", &value)
		}, code: http.StatusBadRequest},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, _, code, err := test.call()
			if got != test.want || code != test.code {
				t.Fatalf("conversion = (%d, %d), want (%d, %d)", got, code, test.want, test.code)
			}
			if (err != nil) != (test.code != http.StatusOK) {
				t.Fatalf("error = %v, want error for status %d", err, test.code)
			}
		})
	}

	optionalTests := []struct {
		name string
		call func() (*int, string, int, error)
		want *int
		code int
	}{
		{name: "string value", call: func() (*int, string, int, error) {
			return ConvertStringToIntPointer("limit", "42")
		}, want: intPointer(42), code: http.StatusOK},
		{name: "empty string", call: func() (*int, string, int, error) {
			return ConvertStringToIntPointer("limit", "")
		}, code: http.StatusOK},
		{name: "invalid string", call: func() (*int, string, int, error) {
			return ConvertStringToIntPointer("limit", "invalid")
		}, code: http.StatusBadRequest},
		{name: "nil pointer", call: func() (*int, string, int, error) {
			return ConvertStringPointerToIntPointer("limit", nil)
		}, code: http.StatusOK},
		{name: "empty pointer", call: func() (*int, string, int, error) {
			value := ""
			return ConvertStringPointerToIntPointer("limit", &value)
		}, code: http.StatusOK},
		{name: "pointer value", call: func() (*int, string, int, error) {
			value := "8"
			return ConvertStringPointerToIntPointer("limit", &value)
		}, want: intPointer(8), code: http.StatusOK},
		{name: "invalid pointer", call: func() (*int, string, int, error) {
			value := "invalid"
			return ConvertStringPointerToIntPointer("limit", &value)
		}, code: http.StatusBadRequest},
	}

	for _, test := range optionalTests {
		t.Run(test.name, func(t *testing.T) {
			got, _, code, err := test.call()
			if code != test.code || (got == nil) != (test.want == nil) {
				t.Fatalf("conversion = (%v, %d), want (%v, %d)", got, code, test.want, test.code)
			}
			if got != nil && *got != *test.want {
				t.Fatalf("value = %d, want %d", *got, *test.want)
			}
			if (err != nil) != (test.code != http.StatusOK) {
				t.Fatalf("error = %v, want error for status %d", err, test.code)
			}
		})
	}
}

func intPointer(value int) *int {
	return &value
}
