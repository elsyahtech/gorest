package orm

import (
	"errors"
	"net/http"
	"reflect"
	"testing"

	"github.com/elsyahtech/gorest/database"
)

func TestQueryBuilderMethodsPreserveValidOptions(t *testing.T) {
	query := &ORM{}
	query.Select("id", "COUNT(*) AS total").
		Where("active = ? AND score >= ?", true, 10).
		Distinct("id").
		Join("INNER JOIN authors ON authors.id = books.author_id").
		AllowFiltering().
		Preload("Author", "active = ?", true).
		GroupBy("id", "score").
		Having("COUNT(*) > ?", 1).
		OrderBy("score", "desc").
		Limit(0).
		Offset(4).
		Upsert("id, email", "ID")

	if query.Error != nil {
		t.Fatalf("valid query builder options produced error: %v", query.Error)
	}
	if !reflect.DeepEqual(query.SelectedCols, []string{"id", "COUNT(*) AS total"}) {
		t.Fatalf("selected columns = %#v", query.SelectedCols)
	}
	if !reflect.DeepEqual(query.WhereClauses, []string{"active = ? AND score >= ?"}) ||
		!reflect.DeepEqual(query.WhereArgs, []any{true, 10}) {
		t.Fatalf("where clauses/arguments = %#v / %#v", query.WhereClauses, query.WhereArgs)
	}
	if !query.IsDistinct || !reflect.DeepEqual(query.DistinctCols, []string{"id"}) {
		t.Fatalf("distinct state = %t / %#v", query.IsDistinct, query.DistinctCols)
	}
	if !reflect.DeepEqual(query.JoinClauses, []string{"INNER JOIN authors ON authors.id = books.author_id"}) {
		t.Fatalf("join clauses = %#v", query.JoinClauses)
	}
	if !query.AllowFilteringFlag {
		t.Fatal("AllowFiltering() did not enable filtering")
	}
	if !reflect.DeepEqual(query.PreloadClauses, []string{"Author"}) ||
		!reflect.DeepEqual(query.Preloads["Author"], []any{"active = ?", true}) {
		t.Fatalf("preload state = %#v / %#v", query.PreloadClauses, query.Preloads)
	}
	if !reflect.DeepEqual(query.GroupByClauses, []string{"id", "score"}) ||
		!reflect.DeepEqual(query.HavingClauses, []string{"COUNT(*) > ?"}) ||
		!reflect.DeepEqual(query.HavingArgs, []any{1}) {
		t.Fatalf("group/having state = %#v / %#v / %#v", query.GroupByClauses, query.HavingClauses, query.HavingArgs)
	}
	if !reflect.DeepEqual(query.OrderByClauses, []string{"score DESC"}) {
		t.Fatalf("order by clauses = %#v", query.OrderByClauses)
	}
	if query.LimitVal != 0 || !query.limitSet || query.OffsetVal != 4 {
		t.Fatalf("pagination state = limit %d (set %t), offset %d", query.LimitVal, query.limitSet, query.OffsetVal)
	}
	if !query.IsUpsert || !reflect.DeepEqual(query.UpsertConflictCols, []string{"id", "email"}) {
		t.Fatalf("upsert state = %t / %#v", query.IsUpsert, query.UpsertConflictCols)
	}
}

func TestQueryBuilderRejectsInvalidOptionsWithoutKeepingThem(t *testing.T) {
	tests := []struct {
		name string
		call func(*ORM)
	}{
		{name: "select", call: func(query *ORM) { query.Select("id; DELETE FROM users") }},
		{name: "where", call: func(query *ORM) { query.Where("id = 1") }},
		{name: "distinct", call: func(query *ORM) { query.Distinct("id; DROP TABLE users") }},
		{name: "join", call: func(query *ORM) { query.Join("JOIN users ON 1=1; DELETE FROM users") }},
		{name: "negative limit", call: func(query *ORM) { query.Limit(-1) }},
		{name: "negative offset", call: func(query *ORM) { query.Offset(-1) }},
		{name: "group by", call: func(query *ORM) { query.GroupBy("id; DROP TABLE users") }},
		{name: "having", call: func(query *ORM) { query.Having("COUNT(*) > 0") }},
		{name: "order by direction", call: func(query *ORM) { query.OrderBy("id", "sideways") }},
		{name: "order by arity", call: func(query *ORM) { query.OrderBy("id", "ASC", "DESC") }},
		{name: "upsert type", call: func(query *ORM) { query.Upsert(42) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := &ORM{}
			test.call(query)

			if query.Error == nil {
				t.Fatal("invalid option unexpectedly succeeded")
			}
			if query.HTTPCode != http.StatusBadRequest && test.name != "upsert type" {
				t.Fatalf("HTTP code = %d, want %d", query.HTTPCode, http.StatusBadRequest)
			}
			if test.name == "where" && len(query.WhereClauses) != 0 {
				t.Fatalf("invalid WHERE clause was retained: %#v", query.WhereClauses)
			}
			if test.name == "join" && len(query.JoinClauses) != 0 {
				t.Fatalf("invalid JOIN clause was retained: %#v", query.JoinClauses)
			}
		})
	}
}

func TestNilQueryBuilderReceiversReturnErrors(t *testing.T) {
	var query *ORM
	tests := []struct {
		name string
		call func() *ORM
	}{
		{name: "Create", call: func() *ORM { return query.Create(struct{}{}) }},
		{name: "Update", call: func() *ORM { return query.Update(struct{}{}) }},
		{name: "Delete", call: func() *ORM { return query.Delete(struct{}{}) }},
		{name: "Find", call: func() *ORM { return query.Find(&struct{}{}) }},
		{name: "Select", call: func() *ORM { return query.Select("id") }},
		{name: "Where", call: func() *ORM { return query.Where("id = ?", 1) }},
		{name: "Distinct", call: func() *ORM { return query.Distinct("id") }},
		{name: "Join", call: func() *ORM { return query.Join("INNER JOIN users ON users.id = posts.user_id") }},
		{name: "AllowFiltering", call: func() *ORM { return query.AllowFiltering() }},
		{name: "Limit", call: func() *ORM { return query.Limit(1) }},
		{name: "Offset", call: func() *ORM { return query.Offset(1) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.call()
			if got == nil || got.Error == nil {
				t.Fatal("nil ORM receiver did not return an error")
			}
		})
	}
}

func TestCRUDEntryPointsRequireInitializedDatabase(t *testing.T) {
	tests := []struct {
		name string
		call func(*ORM) *ORM
	}{
		{name: "Create", call: func(query *ORM) *ORM { return query.Create(struct{}{}) }},
		{name: "Update", call: func(query *ORM) *ORM { return query.Update(struct{}{}) }},
		{name: "Delete", call: func(query *ORM) *ORM { return query.Delete(struct{}{}) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.call(&ORM{})
			if got.Error == nil {
				t.Fatal("operation without database unexpectedly succeeded")
			}
			if got.HTTPCode != http.StatusServiceUnavailable {
				t.Fatalf("HTTP code = %d, want %d", got.HTTPCode, http.StatusServiceUnavailable)
			}
		})
	}
}

func TestExecORMDispatchesDriversAndReportsErrors(t *testing.T) {
	type callbacks struct {
		sql, mongo, scylla int
	}
	const input = "payload"

	tests := []struct {
		name   string
		driver string
		want   string
		code   int
	}{
		{name: "mysql", driver: database.MYSQL, want: "sql", code: http.StatusOK},
		{name: "mongodb", driver: database.MONGO, want: "mongo", code: http.StatusOK},
		{name: "scylla", driver: database.SCYLLA, want: "scylla", code: http.StatusOK},
		{name: "unsupported", driver: "unknown", code: http.StatusNotImplemented},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			query := &ORM{Database: &database.Database{}, DatabaseConfig: &database.Config{Driver: test.driver}}
			var calls callbacks
			run := func(count *int) func(any) error {
				return func(value any) error {
					*count++
					if value != input {
						t.Errorf("callback received %v, want payload", value)
					}
					return nil
				}
			}

			query.execOrm(
				input,
				"Test",
				run(&calls.sql),
				run(&calls.mongo),
				run(&calls.scylla),
			)

			if query.HTTPCode != test.code {
				t.Fatalf("HTTP code = %d, want %d (error %v)", query.HTTPCode, test.code, query.Error)
			}
			got := ""
			switch {
			case calls.sql > 0:
				got = "sql"
			case calls.mongo > 0:
				got = "mongo"
			case calls.scylla > 0:
				got = "scylla"
			}
			if got != test.want {
				t.Fatalf("dispatched %q, want %q", got, test.want)
			}
			if test.want == "" && query.Error == nil {
				t.Fatal("unsupported driver did not return an error")
			}
		})
	}
}

func TestExecORMRejectsMissingDatabaseAndPayload(t *testing.T) {
	callback := func(any) error { t.Fatal("callback should not run"); return nil }
	tests := []struct {
		name  string
		query *ORM
		data  any
		code  int
	}{
		{name: "missing database", query: &ORM{}, data: "payload", code: http.StatusServiceUnavailable},
		{
			name: "nil payload",
			query: &ORM{
				Database:       &database.Database{},
				DatabaseConfig: &database.Config{Driver: database.SQLITE},
			},
			code: http.StatusBadRequest,
		},
		{
			name: "existing error",
			query: &ORM{
				Error: errors.New("already failed"),
			},
			data: "payload",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			test.query.execOrm(test.data, "Test", callback, callback, callback)
			if test.query.Error == nil {
				t.Fatal("expected ORM error")
			}
			if test.code != 0 && test.query.HTTPCode != test.code {
				t.Fatalf("HTTP code = %d, want %d", test.query.HTTPCode, test.code)
			}
		})
	}
}
