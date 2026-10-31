package orm

import (
	"net/http"
	"testing"
)

func TestLimitNegativeKeepsORMChainAvailable(t *testing.T) {
	query := (&ORM{}).Limit(-1)
	if query == nil {
		t.Fatal("Limit() returned nil for an invalid value")
	}
	if query.Error == nil {
		t.Fatal("Limit() did not record the validation error")
	}
	if query.HTTPCode != http.StatusBadRequest {
		t.Errorf("HTTPCode = %d, want %d", query.HTTPCode, http.StatusBadRequest)
	}

	if got := query.Offset(0).Find(&[]struct{}{}); got != query {
		t.Fatal("chained query did not retain the original ORM")
	}
}

func TestOffsetNegativeKeepsORMChainAvailable(t *testing.T) {
	query := (&ORM{}).Offset(-1)
	if query == nil {
		t.Fatal("Offset() returned nil for an invalid value")
	}
	if query.Error == nil {
		t.Fatal("Offset() did not record the validation error")
	}
	if query.HTTPCode != http.StatusBadRequest {
		t.Errorf("HTTPCode = %d, want %d", query.HTTPCode, http.StatusBadRequest)
	}

	if got := query.Limit(1).Find(&[]struct{}{}); got != query {
		t.Fatal("chained query did not retain the original ORM")
	}
}
