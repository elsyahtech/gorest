package helper

import (
	"crypto/tls"
	"os"
	"reflect"
	"testing"
	"time"
)

func unsetEnvironmentVariable(t *testing.T, key string) {
	t.Helper()

	value, wasSet := os.LookupEnv(key)
	if err := os.Unsetenv(key); err != nil {
		t.Fatalf("unset environment variable %s: %v", key, err)
	}
	t.Cleanup(func() {
		if wasSet {
			if err := os.Setenv(key, value); err != nil {
				t.Errorf("restore environment variable %s: %v", key, err)
			}
		} else if err := os.Unsetenv(key); err != nil {
			t.Errorf("unset environment variable %s: %v", key, err)
		}
	})
}

func TestEnvironmentStringReaders(t *testing.T) {
	const key = "GOREST_HELPER_TEST_STRING"
	unsetEnvironmentVariable(t, key)

	if got := ReadTrimmedEnvironmentVariable(key); got != "" {
		t.Errorf("ReadTrimmedEnvironmentVariable(unset) = %q, want empty", got)
	}
	if got := ReadEnvironmentStringWithDefault(key, "fallback"); got != "fallback" {
		t.Errorf("ReadEnvironmentStringWithDefault(unset) = %q, want fallback", got)
	}
	if got := ReadEnvironmentStringPointer(key); got != nil {
		t.Errorf("ReadEnvironmentStringPointer(unset) = %v, want nil", got)
	}

	t.Setenv(key, "  secret value  ")
	if got := ReadTrimmedEnvironmentVariable(key); got != "secret value" {
		t.Errorf("ReadTrimmedEnvironmentVariable() = %q, want trimmed value", got)
	}
	if got := ReadEnvironmentStringWithDefault(key, "fallback"); got != "  secret value  " {
		t.Errorf("ReadEnvironmentStringWithDefault() = %q, want original whitespace", got)
	}
	if got := ReadEnvironmentStringPointer(key); got == nil || *got != "  secret value  " {
		t.Errorf("ReadEnvironmentStringPointer() = %v, want pointer to original value", got)
	}

	t.Setenv(key, "")
	if got := ReadEnvironmentStringWithDefault(key, "fallback"); got != "fallback" {
		t.Errorf("ReadEnvironmentStringWithDefault(empty) = %q, want fallback", got)
	}
	if got := ReadEnvironmentStringPointer(key); got == nil || *got != "" {
		t.Errorf("ReadEnvironmentStringPointer(empty) = %v, want pointer to empty value", got)
	}
	if got := ReadEnvironmentStringPointerWithDefault(key, "fallback"); got == nil || *got != "fallback" {
		t.Errorf("ReadEnvironmentStringPointerWithDefault(empty) = %v, want pointer to fallback", got)
	}
}

func TestEnvironmentIntegerReaders(t *testing.T) {
	const key = "GOREST_HELPER_TEST_INTEGER"
	unsetEnvironmentVariable(t, key)

	if got, err := ReadEnvironmentNonNegativeInteger(key); got != 0 || err != nil {
		t.Errorf("ReadEnvironmentNonNegativeInteger(unset) = (%d, %v), want (0, nil)", got, err)
	}
	if got, err := ReadEnvironmentIntegerWithDefault(key, 17); got != 17 || err != nil {
		t.Errorf("ReadEnvironmentIntegerWithDefault(unset) = (%d, %v), want (17, nil)", got, err)
	}

	t.Setenv(key, " 42 ")
	if got, err := ReadEnvironmentNonNegativeInteger(key); got != 42 || err != nil {
		t.Errorf("ReadEnvironmentNonNegativeInteger() = (%d, %v), want (42, nil)", got, err)
	}
	if got, err := ReadEnvironmentIntegerWithDefault(key, 17); got != 42 || err != nil {
		t.Errorf("ReadEnvironmentIntegerWithDefault() = (%d, %v), want (42, nil)", got, err)
	}

	t.Setenv(key, "-3")
	if got, err := ReadEnvironmentNonNegativeInteger(key); got != 0 || err == nil {
		t.Errorf("ReadEnvironmentNonNegativeInteger(negative) = (%d, %v), want an error", got, err)
	}
	if got, err := ReadEnvironmentIntegerWithDefault(key, 17); got != -3 || err != nil {
		t.Errorf("ReadEnvironmentIntegerWithDefault(negative) = (%d, %v), want (-3, nil)", got, err)
	}

	t.Setenv(key, "invalid")
	if got, err := ReadEnvironmentNonNegativeInteger(key); got != 0 || err == nil {
		t.Errorf("ReadEnvironmentNonNegativeInteger(invalid) = (%d, %v), want an error", got, err)
	}
	if got, err := ReadEnvironmentIntegerWithDefault(key, 17); got != 17 || err == nil {
		t.Errorf("ReadEnvironmentIntegerWithDefault(invalid) = (%d, %v), want (17, error)", got, err)
	}
}

func TestReadEnvironmentFileMode(t *testing.T) {
	const key = "GOREST_HELPER_TEST_FILE_MODE"

	tests := []struct {
		value string
		want  os.FileMode
		valid bool
	}{
		{value: "", want: 0, valid: true},
		{value: "0644", want: 0o644, valid: true},
		{value: "755", want: 0o755, valid: true},
		{value: "0000", want: 0, valid: true},
		{value: "888", valid: false},
		{value: "1000", valid: false},
		{value: "invalid", valid: false},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			t.Setenv(key, test.value)
			got, err := ReadEnvironmentFileMode(key)
			if (err == nil) != test.valid {
				t.Fatalf("ReadEnvironmentFileMode() error = %v, valid %t", err, test.valid)
			}
			if err == nil && got != test.want {
				t.Errorf("ReadEnvironmentFileMode() = %04o, want %04o", got, test.want)
			}
		})
	}
}

func TestReadEnvironmentTLSMinimumVersion(t *testing.T) {
	const key = "GOREST_HELPER_TEST_TLS_VERSION"

	tests := []struct {
		value string
		want  uint16
		valid bool
	}{
		{value: "", want: 0, valid: true},
		{value: "1.2", want: tls.VersionTLS12, valid: true},
		{value: " TLSV1.3 ", want: tls.VersionTLS13, valid: true},
		{value: "1.1", valid: false},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			t.Setenv(key, test.value)
			got, err := ReadEnvironmentTLSMinimumVersion(key)
			if (err == nil) != test.valid {
				t.Fatalf("ReadEnvironmentTLSMinimumVersion() error = %v, valid %t", err, test.valid)
			}
			if err == nil && got != test.want {
				t.Errorf("ReadEnvironmentTLSMinimumVersion() = %d, want %d", got, test.want)
			}
		})
	}
}

func TestReadEnvironmentDurationWithDefault(t *testing.T) {
	const key = "GOREST_HELPER_TEST_DURATION"
	defaultValue := 5 * time.Second

	tests := []struct {
		value string
		want  time.Duration
		valid bool
	}{
		{value: "", want: defaultValue, valid: true},
		{value: "2m", want: 2 * time.Minute, valid: true},
		{value: "-1s", want: defaultValue, valid: false},
		{value: "invalid", want: defaultValue, valid: false},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			t.Setenv(key, test.value)
			got, err := ReadEnvironmentDurationWithDefault(key, defaultValue)
			if (err == nil) != test.valid {
				t.Fatalf("ReadEnvironmentDurationWithDefault() error = %v, valid %t", err, test.valid)
			}
			if got != test.want {
				t.Errorf("ReadEnvironmentDurationWithDefault() = %s, want %s", got, test.want)
			}
		})
	}
}

func TestReadEnvironmentStringSliceWithDefault(t *testing.T) {
	const key = "GOREST_HELPER_TEST_STRING_SLICE"
	unsetEnvironmentVariable(t, key)

	defaultValue := []string{"default", "value"}
	got := ReadEnvironmentStringSliceWithDefault(key, defaultValue)
	if !reflect.DeepEqual(got, defaultValue) {
		t.Fatalf("ReadEnvironmentStringSliceWithDefault(unset) = %v, want %v", got, defaultValue)
	}
	got[0] = "changed"
	if defaultValue[0] != "default" {
		t.Fatal("returned default slice aliases caller's slice")
	}

	emptyDefault := []string{}
	if got := ReadEnvironmentStringSliceWithDefault(key, emptyDefault); got == nil {
		t.Fatal("ReadEnvironmentStringSliceWithDefault(non-nil empty default) = nil, want non-nil empty slice")
	}

	t.Setenv(key, "")
	got = ReadEnvironmentStringSliceWithDefault(key, defaultValue)
	if got == nil || len(got) != 0 {
		t.Errorf("ReadEnvironmentStringSliceWithDefault(empty) = %#v, want non-nil empty slice", got)
	}

	t.Setenv(key, " one, , two ,three ")
	got = ReadEnvironmentStringSliceWithDefault(key, defaultValue)
	if want := []string{"one", "two", "three"}; !reflect.DeepEqual(got, want) {
		t.Errorf("ReadEnvironmentStringSliceWithDefault() = %v, want %v", got, want)
	}
}

func TestEnvironmentBooleanReaders(t *testing.T) {
	const key = "GOREST_HELPER_TEST_BOOLEAN"
	unsetEnvironmentVariable(t, key)

	if got, err := ReadEnvironmentBooleanWithDefault(key, true); !got || err != nil {
		t.Errorf("ReadEnvironmentBooleanWithDefault(unset) = (%t, %v), want (true, nil)", got, err)
	}
	if got, err := ReadEnvironmentBooleanPointer(key); got != nil || err != nil {
		t.Errorf("ReadEnvironmentBooleanPointer(unset) = (%v, %v), want (nil, nil)", got, err)
	}

	t.Setenv(key, " true ")
	if got, err := ReadEnvironmentBooleanWithDefault(key, false); !got || err != nil {
		t.Errorf("ReadEnvironmentBooleanWithDefault() = (%t, %v), want (true, nil)", got, err)
	}
	if got, err := ReadEnvironmentBooleanPointer(key); got == nil || !*got || err != nil {
		t.Errorf("ReadEnvironmentBooleanPointer() = (%v, %v), want pointer to true", got, err)
	}

	t.Setenv(key, "invalid")
	if got, err := ReadEnvironmentBooleanWithDefault(key, true); !got || err == nil {
		t.Errorf("ReadEnvironmentBooleanWithDefault(invalid) = (%t, %v), want (true, error)", got, err)
	}
	if got, err := ReadEnvironmentBooleanPointer(key); got != nil || err == nil {
		t.Errorf("ReadEnvironmentBooleanPointer(invalid) = (%v, %v), want (nil, error)", got, err)
	}
}

func TestValidateApplicationEnvironmentMode(t *testing.T) {
	for _, value := range []string{"development", "Production", " TESTING ", "staging", "uat"} {
		if err := ValidateApplicationEnvironmentMode(value); err != nil {
			t.Errorf("ValidateApplicationEnvironmentMode(%q) error = %v", value, err)
		}
	}
	for _, value := range []string{"", "  "} {
		if err := ValidateApplicationEnvironmentMode(value); err == nil {
			t.Errorf("ValidateApplicationEnvironmentMode(%q) error = nil, want error", value)
		}
	}
}
