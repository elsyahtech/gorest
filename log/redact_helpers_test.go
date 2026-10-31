//nolint:revive,nolintlint // Package log provides logging utilities for the application.
package log

import (
	"reflect"
	"testing"
)

func TestRedactValueHandlesNilUnsupportedAndDepth(t *testing.T) {
	if got := redactValue(reflect.Value{}, 0); got != nil {
		t.Fatalf("redactValue(invalid) = %#v, want nil", got)
	}
	if got := redactValue(reflect.ValueOf((*string)(nil)), 0); got != nil {
		t.Fatalf("redactValue(nil pointer) = %#v, want nil", got)
	}
	if got := redactValue(reflect.ValueOf(make(chan int)), 0); got != redactedValue {
		t.Fatalf("redactValue(channel) = %#v, want fallback redaction", got)
	}
	if got := redactValue(reflect.ValueOf("secret"), 17); got != redactedValue {
		t.Fatalf("redactValue(depth 17) = %#v, want depth redaction", got)
	}

	type cyclic struct{ Next *cyclic }
	value := &cyclic{}
	value.Next = value
	if got := redactValue(reflect.ValueOf(value), 0); got == nil {
		t.Fatal("redactValue(cyclic pointer) = nil")
	}
}

func TestRedactSliceAndElements(t *testing.T) {
	if got := redactSlice(reflect.ValueOf([]byte("password=secret")), 0); got != "password="+redactedValue {
		t.Fatalf("redactSlice() = %#v, want the password value redacted", got)
	}
	if got := redactSlice(reflect.ValueOf([]byte("safe")), 0); got != "safe" {
		t.Fatalf("redactSlice() = %#v, want safe string", got)
	}
	if got := redactSlice(reflect.ValueOf([]string(nil)), 0); got != nil {
		t.Fatalf("redactSlice(nil) = %#v, want nil", got)
	}

	got := redactElements(reflect.ValueOf([2]string{"safe", "token=secret"}), 0)
	if !reflect.DeepEqual(got, []any{"safe", "token=" + redactedValue}) {
		t.Fatalf("redactElements() = %#v", got)
	}
}
