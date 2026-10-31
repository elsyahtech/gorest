package database

import (
	"context"
	"testing"
)

func TestMongoMigrationHelpersHandleInvalidInputsAndUnavailableClient(t *testing.T) {
	ctx := context.Background()
	config := &Config{Driver: MONGO, Name: "test"}
	db := &Database{}

	if message, err := createMongoHistoryTable(ctx, db, config, "gorest_migration", nil); err == nil || message == "" {
		t.Fatalf("createMongoHistoryTable() = (%q, %v), want an unavailable-client error", message, err)
	}
	if message, err := createMongoHistoryTable(ctx, db, config, "", nil); err == nil || message == "" {
		t.Fatalf("createMongoHistoryTable(empty collection) = (%q, %v), want validation error", message, err)
	}
	if _, _, err := getMongoRecords(ctx, db, config, "gorest_migration", "migration_name"); err == nil {
		t.Fatal("getMongoRecords() succeeded without a Mongo client")
	}

	for _, test := range []struct {
		name    string
		content string
		wantErr bool
	}{
		{name: "invalid JSON", content: "{", wantErr: true},
		{name: "missing collection", content: `{}`, wantErr: true},
		{name: "valid collection", content: `{"collection":"records"}`, wantErr: true},
	} {
		t.Run("migration/"+test.name, func(t *testing.T) {
			message, err := mongoDataMigrationExec(ctx, db, config, File{Name: "001.json", Content: test.content})
			if (err != nil) != test.wantErr {
				t.Fatalf("mongoDataMigrationExec() error = %v, wantErr %t (message %q)", err, test.wantErr, message)
			}
			if test.wantErr && message == "" {
				t.Fatal("mongoDataMigrationExec() returned an empty error message")
			}
		})
	}

	for _, test := range []struct {
		name    string
		content string
		wantErr bool
	}{
		{name: "invalid JSON", content: "{", wantErr: true},
		{name: "missing collection type", content: `{"collection":1,"data":[]}`, wantErr: false},
		{name: "empty data", content: `{"collection":"records","data":[]}`, wantErr: true},
		{name: "valid documents", content: `{"collection":"records","data":[{"id":1}]}`, wantErr: true},
	} {
		t.Run("seeder/"+test.name, func(t *testing.T) {
			message, err := mongoDataSeederExec(ctx, db, config, File{Name: "001.json", Content: test.content})
			if (err != nil) != test.wantErr {
				t.Fatalf("mongoDataSeederExec() error = %v, wantErr %t (message %q)", err, test.wantErr, message)
			}
			if test.wantErr && message == "" {
				t.Fatal("mongoDataSeederExec() returned an empty error message")
			}
		})
	}
}

func TestScyllaMigrationHelpersHandleUnavailableSession(t *testing.T) {
	ctx := context.Background()
	config := &Config{Driver: SCYLLA}
	db := &Database{}

	if message, err := createScylaHistoryTable(ctx, db, config, "gorest_migration", "migration_name"); err == nil || message == "" {
		t.Fatalf("createScylaHistoryTable() = (%q, %v), want an unavailable-session error", message, err)
	}
	if _, _, err := getScyllaRecords(ctx, db, config, "gorest_migration", "migration_name"); err == nil {
		t.Fatal("getScyllaRecords() succeeded without a Scylla session")
	}
	if message, err := scyllaDataMigrationExec(ctx, db, config, File{Name: "001.cql", Content: "CREATE TABLE records (id int PRIMARY KEY);"}, "gorest_migration", "migration_name"); err == nil || message == "" {
		t.Fatalf("scyllaDataMigrationExec(statement failure) = (%q, %v), want an unavailable-session error", message, err)
	}
	if message, err := scyllaDataMigrationExec(ctx, db, config, File{Name: "001.cql", Content: " ; "}, "gorest_migration", "migration_name"); err == nil || message == "" {
		t.Fatalf("scyllaDataMigrationExec(history failure) = (%q, %v), want an unavailable-session error", message, err)
	}
}
