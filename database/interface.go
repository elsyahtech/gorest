package database

import (
	"database/sql"
	"time"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
	"github.com/elsyahtech/gorest/monitoring/dbmetrics"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type Database struct {
	SQL     *sql.DB
	Tx      *sql.Tx
	Mongo   *mongo.Client
	Scylla  *gocql.Session
	metrics *dbmetrics.Stats
}

// OperationalMetrics contains metrics scoped to this application's database client.
type OperationalMetrics struct {
	ConnectionActive         *int64
	ConnectionIdle           *int64
	ConnectionMax            *int64
	ConnectionWaitCount      *int64
	ConnectionWaitDuration   *string
	PoolUsagePercent         *float64
	QueryTotal               *int64
	QuerySlowTotal           *int64
	QueryErrorTotal          *int64
	TransactionCommitTotal   *int64
	TransactionRollbackTotal *int64
}

// migration purposes.
type File struct {
	CreatedAt           time.Time
	Name                string
	Path                string
	Content             string
	CompensationContent string
	Number              int
}

type databaseConnector func(*Config, string) string
