package orm

import (
	"testing"

	"github.com/elsyahtech/gorest/database"
)

func TestBackendEntryPointsRejectUnavailableDatabaseServices(t *testing.T) {
	type record struct {
		ID   int    `gorest:"id, primary_key"`
		Name string `gorest:"name"`
	}

	tests := []struct {
		name   string
		driver string
		call   func(*ORM, any) error
	}{
		{name: "Mongo create", driver: database.MONGO, call: (*ORM).createMongo},
		{name: "Scylla create", driver: database.SCYLLA, call: (*ORM).createScylla},
		{name: "Mongo find", driver: database.MONGO, call: (*ORM).findMongo},
		{name: "Scylla find", driver: database.SCYLLA, call: (*ORM).findScylla},
		{name: "Mongo update", driver: database.MONGO, call: (*ORM).updateMongo},
		{name: "Scylla update", driver: database.SCYLLA, call: (*ORM).updateScylla},
		{name: "Mongo delete", driver: database.MONGO, call: (*ORM).deleteMongo},
		{name: "Scylla delete", driver: database.SCYLLA, call: (*ORM).deleteScylla},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			table := "records"
			query := &ORM{
				Database:       &database.Database{},
				DatabaseConfig: &database.Config{Driver: test.driver},
				Table:          &table,
			}
			err := test.call(query, &record{ID: 1, Name: "test"})
			if err == nil || query.Error == nil {
				t.Fatalf("entry point error = %v, ORM.Error = %v; want unavailable database failure", err, query.Error)
			}
		})
	}
}
