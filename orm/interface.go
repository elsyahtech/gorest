package orm

import (
	"context"
	"database/sql"
	"strings"

	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/log"
)

type ORM struct {
	Error              error
	Result             any
	Context            context.Context
	ReturnDest         any
	Database           *database.Database
	Log                *log.Log
	Tx                 *sql.Tx
	StringBuilder      *strings.Builder
	DatabaseConfig     *database.Config
	Table              *string
	Preloads           map[string][]any
	Message            string
	LastInsertId       string
	UpsertConflictCols []string
	WhereClauses       []string
	SelectedCols       []string
	DistinctCols       []string
	UpsertUpdateCols   []string
	UpsertClauses      []string
	UpsertArgs         []any
	PreloadClauses     []string
	WhereArgs          []any
	JoinClauses        []string
	OrderByClauses     []string
	ReturnCols         []string
	RowsAffected       int64
	OffsetVal          int
	LimitVal           int
	IsUpsert           bool
	AllowFilteringFlag bool
	IsDistinct         bool
	IsReturn           bool
}

type sqlPreloadPlan struct {
	preloadReferences map[string]string
	hasManyNames      map[string]struct{}
	dynamicJoins      []string
	hasManyRels       []*hasManyRelation
	joinArgs          []any
	columnsToSelect   []string
	joinArgCounter    int
}
