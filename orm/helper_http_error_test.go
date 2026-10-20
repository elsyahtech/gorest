package orm

import (
	"errors"
	"net/http"
	"testing"
)

func TestWithHTTPCodeUsesLatestStatusAndPreservesCause(t *testing.T) {
	cause := errors.New("database unavailable")
	err := withHTTPCode(cause, http.StatusInternalServerError)
	err = withHTTPCode(err, http.StatusServiceUnavailable)

	var coded interface{ HTTPStatusCode() int }
	if !errors.As(err, &coded) || coded.HTTPStatusCode() != http.StatusServiceUnavailable {
		t.Fatalf("HTTP status = %v; want %d", coded, http.StatusServiceUnavailable)
	}
	if !errors.Is(err, cause) {
		t.Fatal("wrapping changed the original error cause")
	}
}
