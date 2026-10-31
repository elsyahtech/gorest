package orm

import (
	"context"
	"reflect"
	"strings"
	"testing"

	"github.com/elsyahtech/gorest/database"
)

func TestSQLPreloadPlanningAndJoinConflicts(t *testing.T) {
	type department struct {
		ID   int    `gorest:"id, primary_key"`
		Name string `gorest:"name"`
	}
	type user struct {
		ID           int         `gorest:"id, primary_key"`
		DepartmentID int         `gorest:"department_id"`
		Department   *department `gorest:"-" references:"ID"`
	}
	query := &ORM{
		Preloads: map[string][]any{
			"Department": {"departments.active = ?", true},
		},
	}

	plan, err := query.buildSQLPreloads(reflect.TypeFor[user](), "users", database.POSTGRES)
	if err != nil {
		t.Fatalf("buildSQLPreloads() error = %v", err)
	}
	if len(plan.dynamicJoins) != 1 ||
		!strings.Contains(plan.dynamicJoins[0], "LEFT JOIN departments ON departments.id = users.department_id") ||
		!strings.Contains(plan.dynamicJoins[0], "departments.active = $1") {
		t.Fatalf("dynamic SQL preload join = %#v", plan.dynamicJoins)
	}
	if !reflect.DeepEqual(plan.joinArgs, []any{true}) {
		t.Fatalf("SQL preload args = %#v", plan.joinArgs)
	}
	if !reflect.DeepEqual(plan.columnsToSelect, []string{
		"users.*",
		"departments.id AS departments_id",
		"departments.name AS departments_name",
	}) {
		t.Fatalf("SQL preload columns = %#v", plan.columnsToSelect)
	}

	conflict := &ORM{
		Preloads:    map[string][]any{"Department": nil},
		JoinClauses: []string{"LEFT JOIN departments d ON d.id = users.department_id"},
	}
	if _, err := conflict.buildSQLPreloads(reflect.TypeFor[user](), "users", database.SQLITE); err == nil {
		t.Fatal("buildSQLPreloads() accepted a manual join that duplicates a preload")
	}
	if err := conflict.verifJoinClausesPreloadFindSQL("departments", "Department"); err == nil {
		t.Fatal("verifJoinClausesPreloadFindSQL() accepted a conflicting join")
	}

	if _, err := (&ORM{Preloads: map[string][]any{"Missing": nil}}).
		buildSQLPreloads(reflect.TypeFor[user](), "users", database.SQLITE); err == nil {
		t.Fatal("buildSQLPreloads() accepted an unknown relation")
	}
	if _, err := (&ORM{}).findPreloadRelationFindSQL(reflect.TypeFor[user](), "Missing"); err == nil {
		t.Fatal("findPreloadRelationFindSQL() accepted an unknown relation")
	}
}

func TestSQLFindSelectAndPreloadHelpers(t *testing.T) {
	query := &ORM{}
	if got := query.buildSelectColumnFindSQL("users"); !reflect.DeepEqual(got, []string{"users.*"}) {
		t.Fatalf("default selected columns = %#v", got)
	}

	query.SelectedCols = []string{" * ", "users.name"}
	if got := query.buildSelectColumnFindSQL("users"); !reflect.DeepEqual(got, []string{"users.*", "users.name"}) {
		t.Fatalf("explicit selected columns = %#v", got)
	}

	query.StringBuilder = &strings.Builder{}
	query.IsDistinct = true
	query.JoinClauses = []string{"INNER JOIN teams ON teams.id = users.team_id"}
	query.buildQueryStringFindSQL(
		[]string{"users.id", "teams.name"},
		"users",
		[]string{" LEFT JOIN departments ON departments.id = users.department_id"},
	)
	want := "SELECT DISTINCT users.id, teams.name FROM users INNER JOIN teams ON teams.id = users.team_id LEFT JOIN departments ON departments.id = users.department_id"
	if got := query.StringBuilder.String(); got != want {
		t.Fatalf("SQL query = %q, want %q", got, want)
	}

	if err := query.loadFindSQLPreloadData(context.Background(), database.SQLITE, nil, false, nil, nil); err != nil {
		t.Fatalf("loadFindSQLPreloadData() without has-many relations: %v", err)
	}
}
