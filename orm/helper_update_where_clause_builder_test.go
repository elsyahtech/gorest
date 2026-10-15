package orm

import (
	"reflect"
	"strings"
	"testing"
)

func TestBuildSingleWhereClauseRejectsUnusedWhereArgs(t *testing.T) {
	var row reflect.Value
	orm := &ORM{
		WhereClauses: []string{"status = ?"},
		WhereArgs:    []any{"active", "unused"},
	}

	_, err := orm.buildSingleWhereClause(
		&row,
		&resultBuildUpdatePayload{values: []any{"updated"}},
		&columnMetaData{},
		"mysql",
	)
	if err == nil || !strings.Contains(err.Error(), "mismatched where clause placeholders and arguments") {
		t.Fatalf("expected mismatched WHERE argument error, got %v", err)
	}
}

func TestBuildSingleWhereClauseAcceptsExactWhereArgs(t *testing.T) {
	var row reflect.Value
	orm := &ORM{
		WhereClauses: []string{"status = ?"},
		WhereArgs:    []any{"active"},
	}

	result, err := orm.buildSingleWhereClause(
		&row,
		&resultBuildUpdatePayload{values: []any{"updated"}},
		&columnMetaData{},
		"mysql",
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.args) != 2 || result.args[0] != "updated" || result.args[1] != "active" {
		t.Fatalf("unexpected arguments: %#v", result.args)
	}
}

func TestBuildBulkWhereClauseRejectsUnusedWhereArgs(t *testing.T) {
	row := reflect.ValueOf(struct {
		ID int
	}{ID: 1})
	orm := &ORM{
		WhereClauses: []string{"status = ?"},
		WhereArgs:    []any{"active", "unused"},
	}
	query := &resBuildQueryUpdateSQL{argCounter: 1}

	_, err := orm.buildBulkWhereClause(&reqBuildBulkWhereClause{
		query:             query,
		driver:            "mysql",
		primaryKeyIndex:   []int{0},
		primaryKeyColumns: []string{"id"},
		rows:              []reflect.Value{row},
	})
	if err == nil || !strings.Contains(err.Error(), "mismatched where clause placeholders and arguments") {
		t.Fatalf("expected mismatched WHERE argument error, got %v", err)
	}
}

func TestBuildBulkWhereClauseAcceptsExactWhereArgs(t *testing.T) {
	row := reflect.ValueOf(struct {
		ID int
	}{ID: 1})
	orm := &ORM{
		WhereClauses: []string{"status = ?"},
		WhereArgs:    []any{"active"},
	}
	query := &resBuildQueryUpdateSQL{argCounter: 1}

	_, err := orm.buildBulkWhereClause(&reqBuildBulkWhereClause{
		query:             query,
		driver:            "mysql",
		primaryKeyIndex:   []int{0},
		primaryKeyColumns: []string{"id"},
		rows:              []reflect.Value{row},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(query.args) != 2 || query.args[0] != 1 || query.args[1] != "active" {
		t.Fatalf("unexpected arguments: %#v", query.args)
	}
}
