package orm

import (
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/elsyahtech/gorest/database"
)

func TestGetPlaceholderUsesDriverSyntax(t *testing.T) {
	tests := []struct {
		driver string
		want   string
	}{
		{driver: database.MYSQL, want: "?"},
		{driver: database.SQLITE, want: "?"},
		{driver: database.POSTGRES, want: "$7"},
		{driver: database.SQLSERVER, want: "@p7"},
		{driver: database.ORACLE, want: ":7"},
	}

	for _, test := range tests {
		t.Run(test.driver, func(t *testing.T) {
			if got := getPlaceholder(test.driver, 7); got != test.want {
				t.Errorf("getPlaceholder(%q, 7) = %q, want %q", test.driver, got, test.want)
			}
		})
	}
}

func TestCreateValuesRemainBoundArguments(t *testing.T) {
	type testRow struct {
		ID      int    `gorest:"id, primary_key"`
		Payload string `gorest:"payload"`
	}

	const maliciousValue = `x'); DROP TABLE users; --`
	rows := []reflect.Value{
		reflect.ValueOf(testRow{ID: 7, Payload: maliciousValue}),
	}

	tests := []struct {
		driver string
		want   string
	}{
		{driver: database.MYSQL, want: "(?, ?)"},
		{driver: database.SQLITE, want: "(?, ?)"},
		{driver: database.POSTGRES, want: "($1, $2)"},
		{driver: database.SQLSERVER, want: "(@p1, @p2)"},
		{driver: database.ORACLE, want: "(:1, :2)"},
	}

	for _, test := range tests {
		t.Run(test.driver, func(t *testing.T) {
			built := buildValueArgCreateSQL(rows, []int{0, 1}, test.driver)
			statement := "INSERT INTO users (id, payload) VALUES " + strings.Join(built.placeholders, ", ")

			if statement != "INSERT INTO users (id, payload) VALUES "+test.want {
				t.Fatalf("statement = %q, want placeholders %q", statement, test.want)
			}
			if strings.Contains(statement, maliciousValue) {
				t.Fatal("user-controlled payload was interpolated into the SQL statement")
			}
			if len(built.valueArgs) != 2 || built.valueArgs[1] != maliciousValue {
				t.Fatalf("bound arguments = %#v, want payload preserved as argument", built.valueArgs)
			}
		})
	}
}

func TestFindWhereValidatesPlaceholderArgumentCountAndBindsValues(t *testing.T) {
	const maliciousValue = `x' OR 1=1 --`
	orm := &ORM{
		StringBuilder: &strings.Builder{},
		WhereClauses:  []string{"email = ?"},
		WhereArgs:     []any{maliciousValue},
	}

	result, err := orm.buildWhereClauseFindSQL(&reqBuildWhereClauseFindSQL{
		activeDriver: database.POSTGRES,
		isSlice:      true,
	})
	if err != nil {
		t.Fatalf("buildWhereClauseFindSQL() error = %v", err)
	}
	if got := orm.StringBuilder.String(); got != " WHERE email = $1" {
		t.Errorf("WHERE SQL = %q, want %q", got, " WHERE email = $1")
	}
	if len(result.valueArgs) != 1 || result.valueArgs[0] != maliciousValue {
		t.Errorf("bound arguments = %#v, want the input preserved as a bound value", result.valueArgs)
	}

	for _, test := range []struct {
		name string
		args []any
	}{
		{name: "missing argument"},
		{name: "unused argument", args: []any{"value", "extra"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			query := &ORM{
				StringBuilder: &strings.Builder{},
				WhereClauses:  []string{"email = ?"},
				WhereArgs:     test.args,
			}
			_, err := query.buildWhereClauseFindSQL(&reqBuildWhereClauseFindSQL{
				activeDriver: database.POSTGRES,
				isSlice:      true,
			})
			if err == nil {
				t.Fatal("expected placeholder/argument mismatch to be rejected")
			}
		})
	}
}

func TestWhereSQLFragmentIsTrustedAndArgumentsRemainBound(t *testing.T) {
	const rawFragment = "email = ? OR 1=1 --"
	const boundValue = "person@example.test"

	query := &ORM{
		StringBuilder: &strings.Builder{},
		WhereClauses:  []string{rawFragment},
		WhereArgs:     []any{boundValue},
	}
	result, err := query.buildWhereClauseFindSQL(&reqBuildWhereClauseFindSQL{
		activeDriver: database.POSTGRES,
		isSlice:      true,
	})
	if err != nil {
		t.Fatalf("buildWhereClauseFindSQL() error = %v", err)
	}

	if got, want := query.StringBuilder.String(), " WHERE email = $1 OR 1=1 --"; got != want {
		t.Fatalf("SQL fragment = %q, want %q", got, want)
	}
	if len(result.valueArgs) != 1 || result.valueArgs[0] != boundValue {
		t.Fatalf("bound arguments = %#v, want %#v", result.valueArgs, []any{boundValue})
	}
}

func TestHavingBindsValuesAndValidatesPlaceholderCount(t *testing.T) {
	const maliciousValue = `x' OR 1=1 --`
	query := &ORM{
		StringBuilder: &strings.Builder{},
		HavingClauses: []string{"COUNT(*) > ?"},
		HavingArgs:    []any{maliciousValue},
	}

	args, next, err := query.buildGroupByHavingFindSQL(database.POSTGRES, 4, []any{"active"})
	if err != nil {
		t.Fatalf("buildGroupByHavingFindSQL() error = %v", err)
	}
	if got, want := query.StringBuilder.String(), " HAVING COUNT(*) > $4"; got != want {
		t.Errorf("HAVING SQL = %q, want %q", got, want)
	}
	if !reflect.DeepEqual(args, []any{"active", maliciousValue}) {
		t.Errorf("bound arguments = %#v, want %#v", args, []any{"active", maliciousValue})
	}
	if next != 5 {
		t.Errorf("next placeholder = %d, want 5", next)
	}

	for _, test := range []struct {
		name string
		args []any
	}{
		{name: "missing argument"},
		{name: "unused argument", args: []any{"value", "extra"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			query := &ORM{
				StringBuilder: &strings.Builder{},
				HavingClauses: []string{"COUNT(*) > ?"},
				HavingArgs:    test.args,
			}
			if _, _, err := query.buildGroupByHavingFindSQL(database.POSTGRES, 1, nil); err == nil {
				t.Fatal("expected HAVING placeholder/argument mismatch to be rejected")
			}
		})
	}
}

func TestOrderByAcceptsTaggedIdentifiersAndRejectsAdversarialInput(t *testing.T) {
	type user struct {
		Email string `gorest:"email"`
	}
	modelType := reflect.TypeOf(user{})

	valid := &ORM{OrderByClauses: []string{"email DESC"}}
	got, err := valid.buildOrderByClause(modelType, "users")
	if err != nil {
		t.Fatalf("buildOrderByClause() rejected valid tagged identifier: %v", err)
	}
	if got != " ORDER BY email DESC" {
		t.Errorf("ORDER BY clause = %q, want %q", got, " ORDER BY email DESC")
	}

	for _, clause := range []string{
		"email; DROP TABLE users",
		"email --",
		"unmapped_column",
		"other_table.email",
		"email DESC, (SELECT 1)",
	} {
		t.Run(clause, func(t *testing.T) {
			query := &ORM{OrderByClauses: []string{clause}}
			if _, err := query.buildOrderByClause(modelType, "users"); err == nil {
				t.Fatalf("buildOrderByClause() accepted adversarial or untagged clause %q", clause)
			}
		})
	}
}

func TestCreateAndUpdateRejectInvalidPayloadAndMetadata(t *testing.T) {
	type noPrimaryKey struct {
		Name string `gorest:"name"`
	}
	type primaryKeyOnly struct {
		ID int `gorest:"id, primary_key"`
	}
	type updatable struct {
		ID   int    `gorest:"id, primary_key"`
		Name string `gorest:"name"`
	}

	orm := &ORM{}
	if _, err := orm.validateData([]struct {
		ID int `gorest:"id, primary_key"`
	}{}, fieldRequirement{
		requirePrimaryKey: true,
		requireNonEmpty:   true,
		validateRequired:  true,
	}, "Create"); err == nil {
		t.Fatal("Create validation accepted an empty slice")
	}
	if _, err := orm.validateData(noPrimaryKey{Name: "name"}, fieldRequirement{
		requirePrimaryKey: true,
		requireNonEmpty:   true,
		validateRequired:  true,
	}, "Create"); err == nil {
		t.Fatal("Create validation accepted a model without a primary key")
	}

	rows, err := orm.validateData(primaryKeyOnly{ID: 1}, fieldRequirement{
		requirePrimaryKey: true,
	}, "Update")
	if err != nil {
		t.Fatalf("Update validation unexpectedly failed: %v", err)
	}

	metadata := orm.extractColumnMetaData(rows, "Update")
	if len(metadata.primaryKeyIndex) != 1 {
		t.Fatalf("primary key indexes = %#v, want one primary key", metadata.primaryKeyIndex)
	}
	if !reflect.DeepEqual(metadata.columns, []string{"id"}) {
		t.Fatalf("extracted columns = %#v, want only the primary key", metadata.columns)
	}
	if err := orm.validateUpdateColumns(metadata); err == nil {
		t.Fatal("Update validation accepted a model with no non-primary-key columns")
	}

	validRows, err := orm.validateData(updatable{ID: 1, Name: "updated"}, fieldRequirement{
		requirePrimaryKey: true,
	}, "Update")
	if err != nil {
		t.Fatalf("valid Update validation unexpectedly failed: %v", err)
	}
	if err := orm.validateUpdateColumns(orm.extractColumnMetaData(validRows, "Update")); err != nil {
		t.Fatalf("Update validation rejected a non-primary-key column: %v", err)
	}
}

func TestDeletePayloadRequiresWhereOrPrimaryKey(t *testing.T) {
	orm := &ORM{StringBuilder: &strings.Builder{}}
	_, _, err := orm.buildDeletePayloadByDefault("", database.MYSQL, nil, columnMetaData{}, nil)
	if err == nil {
		t.Fatal("Delete payload builder accepted an empty WHERE clause without a primary key")
	}

	orm = &ORM{
		WhereClauses: []string{"status = ?"},
		WhereArgs:    []any{"active", "unused"},
	}
	_, _, err = orm.buildDeletePayloadByWhereClause(&reqBuildDeletePayloadByWhereClause{
		activeDriver: database.MYSQL,
	})
	if err == nil {
		t.Fatal("Delete payload builder accepted a placeholder/argument mismatch")
	}
}

func TestPrepareUpsertSQLValidatesDriverColumnsAndArguments(t *testing.T) {
	tests := []struct {
		name   string
		orm    *ORM
		meta   columnMetaData
		driver string
	}{
		{
			name:   "unsupported driver",
			orm:    &ORM{},
			driver: database.MONGO,
		},
		{
			name:   "unknown conflict column",
			orm:    &ORM{UpsertConflictCols: []string{"missing"}},
			driver: database.POSTGRES,
			meta: columnMetaData{
				columns:                []string{"id", "email"},
				primaryKeyColumns:      []string{"id"},
				allColumnPrimaryKeyIdx: map[string]int{"id": 0, "email": 1},
			},
		},
		{
			name: "mismatched condition arguments",
			orm: &ORM{
				UpsertConflictCols: []string{"email"},
				UpsertClauses:      []string{"status = ?"},
			},
			driver: database.POSTGRES,
			meta: columnMetaData{
				columns:                []string{"id", "email", "status"},
				primaryKeyColumns:      []string{"id"},
				allColumnPrimaryKeyIdx: map[string]int{"id": 0, "email": 1, "status": 2},
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.orm.prepareUpsertSQL(test.meta, test.driver); err == nil {
				t.Fatal("prepareUpsertSQL() accepted invalid upsert configuration")
			}
		})
	}
}

func TestPaginationBuildsDriverSpecificSQLAndArgs(t *testing.T) {
	tests := []struct {
		driver               string
		wantSQL              string
		initialArgs          []any
		wantArgs             []any
		wantPlaceholderStart int
	}{
		{
			driver:               database.MYSQL,
			wantSQL:              " LIMIT ? OFFSET ?",
			wantArgs:             []any{20, 40},
			wantPlaceholderStart: 1,
		},
		{
			driver:               database.SQLITE,
			wantSQL:              " LIMIT ? OFFSET ?",
			wantArgs:             []any{20, 40},
			wantPlaceholderStart: 1,
		},
		{
			driver:               database.POSTGRES,
			wantSQL:              " LIMIT $3 OFFSET $4",
			initialArgs:          []any{"status", "active"},
			wantArgs:             []any{"status", "active", 20, 40},
			wantPlaceholderStart: 3,
		},
		{
			driver:               database.SQLSERVER,
			wantSQL:              " ORDER BY (SELECT NULL) OFFSET @p3 ROWS FETCH NEXT @p4 ROWS ONLY",
			initialArgs:          []any{"status", "active"},
			wantArgs:             []any{"status", "active", 40, 20},
			wantPlaceholderStart: 3,
		},
		{
			driver:               database.ORACLE,
			wantSQL:              " OFFSET :3 ROWS FETCH NEXT :4 ROWS ONLY",
			initialArgs:          []any{"status", "active"},
			wantArgs:             []any{"status", "active", 40, 20},
			wantPlaceholderStart: 3,
		},
	}

	for _, test := range tests {
		t.Run(test.driver, func(t *testing.T) {
			query := &ORM{
				StringBuilder: &strings.Builder{},
				LimitVal:      20,
				OffsetVal:     40,
				limitSet:      true,
			}
			args := query.buildLimitOffsetFindSQL(&structBuildLimitOffsetFindSQL{
				activeDriver: test.driver,
				valueArgs:    append([]any(nil), test.initialArgs...),
				limitVal:     20,
				argCounter:   test.wantPlaceholderStart,
			})

			if got := query.StringBuilder.String(); got != test.wantSQL {
				t.Errorf("pagination SQL = %q, want %q", got, test.wantSQL)
			}
			if !reflect.DeepEqual(args, test.wantArgs) {
				t.Errorf("pagination arguments = %#v, want %#v", args, test.wantArgs)
			}
		})
	}
}

func TestPreloadRelationMapsForeignAndTargetColumns(t *testing.T) {
	type author struct {
		Key int `gorest:"author_key, primary_key"`
	}
	type book struct {
		AuthorID int    `gorest:"author_ref"`
		Author   author `gorest:"-" references:"author_key"`
	}

	orm := &ORM{}
	result, err := orm.findPreloadRelation(reflect.TypeOf(book{}), "Author")
	if err != nil {
		t.Fatalf("findPreloadRelation() error = %v", err)
	}
	if !result.found {
		t.Fatal("expected Author relation to be found")
	}
	if result.foreignKey != "author_ref" || result.targetKey != "author_key" {
		t.Errorf("relation keys = (%q, %q), want (%q, %q)", result.foreignKey, result.targetKey, "author_ref", "author_key")
	}
}

func TestPreloadAliasColumnsDoNotConflictWithParentColumns(t *testing.T) {
	type user struct {
		ID int `gorest:"id, primary_key"`
	}

	query := &ORM{Preloads: map[string][]any{"Author": nil}}
	prefixes := query.buildPreloadPrefixMapFindSQL(nil)
	if _, ok := prefixes["authors_"]; !ok {
		t.Fatalf("preload aliases = %#v, want authors_ prefix", prefixes)
	}

	if err := query.validateDuplicateColumnsFindSQL(
		[]string{"id", "authors_id"},
		prefixes,
		buildStructFieldMap(reflect.TypeOf(user{})),
	); err != nil {
		t.Fatalf("preload alias was treated as a duplicate parent column: %v", err)
	}

	if err := query.validateDuplicateColumnsFindSQL(
		[]string{"id", "id"},
		prefixes,
		buildStructFieldMap(reflect.TypeOf(user{})),
	); err == nil {
		t.Fatal("duplicate parent column aliases were not rejected")
	}
}

func TestIndependentORMQueriesAreSafeToBuildConcurrently(t *testing.T) {
	const workers = 16
	var wait sync.WaitGroup
	errors := make(chan string, workers)

	for worker := range workers {
		wait.Add(1)
		go func(worker int) {
			defer wait.Done()

			driver := database.POSTGRES
			if worker%2 == 0 {
				driver = database.SQLSERVER
			}

			query := &ORM{
				StringBuilder: &strings.Builder{},
				WhereClauses:  []string{"account_id = ?"},
				WhereArgs:     []any{worker},
			}
			where, err := query.buildWhereClauseFindSQL(&reqBuildWhereClauseFindSQL{
				activeDriver: driver,
				isSlice:      true,
			})
			if err != nil {
				errors <- err.Error()
				return
			}

			want := " WHERE account_id = $1"
			if driver == database.SQLSERVER {
				want = " WHERE account_id = @p1"
			}
			if got := query.StringBuilder.String(); got != want {
				errors <- got
				return
			}
			if len(where.valueArgs) != 1 || where.valueArgs[0] != worker {
				errors <- "query received another worker's arguments"
			}
		}(worker)
	}

	wait.Wait()
	close(errors)
	for err := range errors {
		t.Errorf("concurrent independent query build failed: %s", err)
	}
}
