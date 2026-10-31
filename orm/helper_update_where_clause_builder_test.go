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

func TestBuildBulkWhereClauseAllowsUnwrappedSingleRowKey(t *testing.T) {
	row := reflect.ValueOf(struct {
		TenantID string
		ItemID   string
	}{TenantID: "tenant-1", ItemID: "item-1"})
	query := &resBuildQueryUpdateSQL{argCounter: 1}

	clauses, err := (&ORM{}).buildBulkWhereClause(&reqBuildBulkWhereClause{
		query:                     query,
		driver:                    "scylla",
		primaryKeyIndex:           []int{0, 1},
		primaryKeyColumns:         []string{"tenant_id", "item_id"},
		rows:                      []reflect.Value{row},
		singleRowPrimaryKeyClause: true,
	})
	if err != nil {
		t.Fatalf("build single-row ScyllaDB WHERE clause: %v", err)
	}
	if len(clauses) != 1 || clauses[0] != "tenant_id = ? AND item_id = ?" {
		t.Fatalf("ScyllaDB WHERE clauses = %#v, want an unwrapped composite-key predicate", clauses)
	}
	if !reflect.DeepEqual(query.args, []any{"tenant-1", "item-1"}) {
		t.Fatalf("ScyllaDB WHERE args = %#v", query.args)
	}
}
