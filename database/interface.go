package database

import (
	"database/sql"
	"time"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type Database struct {
	SQL    *sql.DB
	Tx     *sql.Tx
	Mongo  *mongo.Client
	Scylla *gocql.Session
}

// migration purposes.
type File struct {
	CreatedAt time.Time
	Name      string
	Path      string
	Content   string
	Number    int
}

type databaseConnector func(*Config, string) string
