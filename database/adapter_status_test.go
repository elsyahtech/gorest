package database

import (
	"context"
	"net/http"
	"testing"
)

func TestNativeAdaptersReportUnavailableServicesAs503(t *testing.T) {
	db := &Database{}
	config := &Config{Driver: SQLITE}
	cases := []struct {
		name string
		call func() (int, error)
	}{
		{name: "ExecSQL", call: func() (int, error) {
			_, _, code, err := db.ExecSQL(context.Background(), config, "SELECT 1")
			return code, err
		}},
		{name: "QuerySQL", call: func() (int, error) {
			_, _, code, err := db.QuerySQL(context.Background(), config, "SELECT 1")
			return code, err
		}},
		{name: "BeginTxContext", call: func() (int, error) {
			_, _, code, err := db.BeginTxContext(context.Background(), config)
			return code, err
		}},
		{name: "MongoCollection", call: func() (int, error) { _, _, code, err := db.Collection(config, "users"); return code, err }},
		{name: "CreateCollection", call: func() (int, error) {
			_, code, err := db.CreateCollection(context.Background(), config, "users")
			return code, err
		}},
		{name: "ExecCQL", call: func() (int, error) {
			_, code, err := db.ExecCQL(context.Background(), config, "SELECT * FROM users")
			return code, err
		}},
		{name: "QueryCQL", call: func() (int, error) {
			_, _, code, err := db.QueryCQL(context.Background(), config, "SELECT * FROM users")
			return code, err
		}},
		{name: "ExecBatchCQL", call: func() (int, error) {
			_, code, err := db.ExecBatchCQL(context.Background(), config, nil, nil)
			return code, err
		}},
	}

	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			code, err := test.call()
			if err == nil || code != http.StatusServiceUnavailable {
				t.Fatalf("got HTTP %d, error %v; want HTTP 503 and an error", code, err)
			}
		})
	}
}

func TestScyllaAdaptersRejectMissingConfigWithoutPanic(t *testing.T) {
	db := &Database{}
	if _, _, _, err := db.QueryCQL(context.Background(), nil, "SELECT * FROM users"); err == nil {
		t.Fatal("QueryCQL with nil config succeeded")
	}
	if _, _, err := db.ExecBatchCQL(context.Background(), nil, nil, nil); err == nil {
		t.Fatal("ExecBatchCQL with nil config succeeded")
	}
}
