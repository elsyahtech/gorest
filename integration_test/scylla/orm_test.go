//go:build integration

package scylla_test

import (
	"context"
	"testing"
	"time"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
	"github.com/elsyahtech/gorest/database"
	"github.com/elsyahtech/gorest/integration_test/internaltest"
)

func TestScyllaServerVersion(t *testing.T) {
	app := internaltest.OpenApp(t, database.SCYLLA)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var version string
	err := app.Database().RawQueryScylla(ctx, func(ctx context.Context, session *gocql.Session) error {
		return session.Query("SELECT release_version FROM system.local").WithContext(ctx).Scan(&version)
	})
	if err != nil {
		t.Fatalf("query ScyllaDB server version failed: %v", err)
	}

	t.Logf("ScyllaDB server version: %s", version)
}

func TestORMCRUD(t *testing.T) {
	internaltest.ScyllaCRUD(t)
}
