//go:build integration

package internaltest

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
	gorest "github.com/elsyahtech/gorest"
	"github.com/elsyahtech/gorest/database"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

func MongoORMCoverage(t *testing.T) {
	t.Helper()
	app := OpenApp(t, database.MONGO)
	collectionName := "audit_" + uniqueIdentifier(t, "")
	collection, message, code, err := app.Database().MongoCollection(collectionName)
	if err != nil {
		t.Fatalf("create collection handle (HTTP %d, %s): %v", code, message, err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := collection.Drop(ctx); err != nil {
			t.Errorf("drop integration collection %s: %v", collectionName, err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err = collection.Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	if err != nil {
		t.Fatalf("create unique email index: %v", err)
	}

	type document struct {
		ID    string `gorest:"_id, primary_key"`
		Email string `gorest:"email"`
		Name  string `gorest:"name"`
	}
	initial := document{ID: "first", Email: "test@example.test", Name: "before"}
	RequireNoORMError(t, app.Database().Table(collectionName).Create(&initial))
	update := document{ID: "second", Email: initial.Email, Name: "after"}
	RequireNoORMError(t, app.Database().Table(collectionName).Upsert("email").Create(&update))

	var loaded document
	RequireNoORMError(t, app.Database().Table(collectionName).
		Where("email = ?", initial.Email).
		Find(&loaded))
	if loaded.ID != initial.ID || loaded.Name != "after" {
		t.Fatalf("alternate-key Upsert result = %#v, want original _id with updated name", loaded)
	}

	type applicationDocument struct {
		ID    string `gorest:"id, primary_key"`
		Email string `gorest:"email"`
		Name  string `gorest:"name"`
	}
	initialApplicationDoc := applicationDocument{
		ID: "application-id-first", Email: "application-id@example.test", Name: "before",
	}
	RequireNoORMError(t, app.Database().Table(collectionName).Create(&initialApplicationDoc))
	updatedApplicationDoc := applicationDocument{
		ID: "application-id-second", Email: initialApplicationDoc.Email, Name: "after",
	}
	RequireNoORMError(t, app.Database().Table(collectionName).Upsert("email").Create(&updatedApplicationDoc))

	var loadedApplicationDoc applicationDocument
	RequireNoORMError(t, app.Database().Table(collectionName).
		Where("email = ?", initialApplicationDoc.Email).
		Find(&loadedApplicationDoc))
	if loadedApplicationDoc.ID != initialApplicationDoc.ID || loadedApplicationDoc.Name != "after" {
		t.Fatalf("alternate-key Upsert result = %#v, want original application id %q with updated name",
			loadedApplicationDoc, initialApplicationDoc.ID)
	}

	if query := app.Database().Table(collectionName).Upsert("email = ? AND name = ?", initial.Email).
		Create(&document{ID: "third", Email: initial.Email, Name: "invalid"}); query.Error == nil {
		t.Fatal("Upsert() with a placeholder/argument mismatch unexpectedly succeeded")
	}
	if query := app.Database().Table(collectionName).Update(&document{Name: "unfiltered"}); query.Error == nil {
		t.Fatal("unfiltered Update() unexpectedly succeeded")
	}
	if query := app.Database().Table(collectionName).Delete(&document{}); query.Error == nil {
		t.Fatal("unfiltered Delete() unexpectedly succeeded")
	}

	secondApplicationDoc := applicationDocument{
		ID: "application-id-third", Email: "application-id-third@example.test", Name: "third",
	}
	RequireNoORMError(t, app.Database().Table(collectionName).Create(&secondApplicationDoc))

	singleKeyRows := []applicationDocument{initialApplicationDoc, secondApplicationDoc}
	query := app.Database().Table(collectionName).Delete(&singleKeyRows)
	RequireNoORMError(t, query)
	if query.RowsAffected != int64(len(singleKeyRows)) {
		t.Fatalf("MongoDB single-key bulk delete affected %d rows, want %d", query.RowsAffected, len(singleKeyRows))
	}

	compositeCollection := "audit_composite_" + uniqueIdentifier(t, "")
	compositeDocs, _, _, err := app.Database().MongoCollection(compositeCollection)
	if err != nil {
		t.Fatalf("get MongoDB composite-key collection: %v", err)
	}
	t.Cleanup(func() {
		if err := compositeDocs.Drop(context.Background()); err != nil {
			t.Errorf("drop MongoDB composite-key collection: %v", err)
		}
	})
	type compositeDocument struct {
		TenantID string `gorest:"tenant_id, primary_key"`
		ItemID   string `gorest:"item_id, primary_key"`
		Name     string `gorest:"name"`
	}
	compositeRows := []compositeDocument{
		{TenantID: "tenant", ItemID: "one", Name: "one"},
		{TenantID: "tenant", ItemID: "two", Name: "two"},
	}
	RequireNoORMError(t, app.Database().Table(compositeCollection).Create(&compositeRows))
	query = app.Database().Table(compositeCollection).Delete(&compositeRows)
	RequireNoORMError(t, query)
	if query.RowsAffected != int64(len(compositeRows)) {
		t.Fatalf("MongoDB composite-key bulk delete affected %d rows, want %d", query.RowsAffected, len(compositeRows))
	}

	mongoPreloadCoverage(t, app)
}

func ScyllaORMCoverage(t *testing.T) {
	t.Helper()
	app := OpenApp(t, database.SCYLLA)
	table := "audit_scylla_" + fmt.Sprintf("%x", time.Now().UnixNano())[:8]
	ExecCQL(t, app, "CREATE TABLE "+table+
		" (tenant_id text, item_id text, name text, optional_text text, PRIMARY KEY (tenant_id, item_id))")
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		message, code, err := app.Database().ExecCQL(ctx, "DROP TABLE "+table)
		if err != nil {
			t.Errorf("drop integration table %s (HTTP %d, %s): %v", table, code, message, err)
		}
	})

	type record struct {
		TenantID     string  `gorest:"tenant_id, primary_key"`
		ItemID       string  `gorest:"item_id, primary_key"`
		Name         string  `gorest:"name"`
		OptionalText *string `gorest:"optional_text"`
	}
	original := record{TenantID: "tenant", ItemID: "item", Name: "before"}
	RequireNoORMError(t, app.Database().Table(table).Create(&original))

	if query := app.Database().Table(table).Upsert("tenant_id").Create(&record{
		TenantID: "tenant", ItemID: "item", Name: "invalid conflict target",
	}); query.Error == nil {
		t.Fatal("Upsert() with only part of the composite key unexpectedly succeeded")
	}
	if query := app.Database().Table(table).Upsert("tenant_id = ?", "tenant").Create(&record{
		TenantID: "tenant", ItemID: "item", Name: "conditional",
	}); query.Error == nil {
		t.Fatal("conditional ScyllaDB Upsert() unexpectedly succeeded")
	}

	updated := record{TenantID: "tenant", ItemID: "item", Name: "after"}
	RequireNoORMError(t, app.Database().Table(table).Upsert("tenant_id", "item_id").Create(&updated))

	secondRecord := record{TenantID: "tenant", ItemID: "item-2", Name: "before second"}
	RequireNoORMError(t, app.Database().Table(table).Create(&secondRecord))

	bulkUpdates := []record{
		{TenantID: "tenant", ItemID: updated.ItemID, Name: "bulk first"},
		{TenantID: "tenant", ItemID: secondRecord.ItemID, Name: "bulk second"},
	}
	query := app.Database().Table(table).Update(&bulkUpdates)
	RequireNoORMError(t, query)
	if query.RowsAffected != int64(len(bulkUpdates)) {
		t.Fatalf("ScyllaDB bulk update affected %d rows, want %d", query.RowsAffected, len(bulkUpdates))
	}

	missingUpdate := []record{{TenantID: "tenant", ItemID: "missing", Name: "missing"}}
	query = app.Database().Table(table).Update(&missingUpdate)
	if query.Error == nil || query.HTTPCode != 404 {
		t.Fatalf("ScyllaDB bulk update with missing key = HTTP %d, error %v; want HTTP 404", query.HTTPCode, query.Error)
	}

	upsert := app.Database().Table(table).Upsert("tenant_id", "item_id")
	upsert.UpsertUpdateCols = []string{"name"}
	RequireNoORMError(t, upsert.Create(&record{
		TenantID: updated.TenantID,
		ItemID:   updated.ItemID,
		Name:     "upsert update",
	}))

	ctxRaw, cancelRaw := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelRaw()
	var rawOptionalText *string
	_, _, err := app.Database().RawQueryScylla(ctxRaw, func(ctx context.Context, session *gocql.Session) error {
		return session.Query(
			"SELECT optional_text FROM "+table+" WHERE tenant_id = ? AND item_id = ?",
			updated.TenantID,
			updated.ItemID,
		).WithContext(ctx).Scan(&rawOptionalText)
	})
	if err != nil {
		t.Fatalf("read ScyllaDB optional_text directly: %v", err)
	}
	if rawOptionalText != nil {
		t.Fatalf("ScyllaDB stored optional_text = %q, want NULL", *rawOptionalText)
	}

	var loaded record
	RequireNoORMError(t, app.Database().Table(table).
		Where("tenant_id = ? AND item_id = ?", updated.TenantID, updated.ItemID).
		Find(&loaded))
	if loaded.Name != "upsert update" || loaded.OptionalText != nil {
		var optionalText any
		if loaded.OptionalText != nil {
			optionalText = *loaded.OptionalText
		}
		t.Fatalf("ScyllaDB composite-key Upsert/nullable mapping = %#v (optional_text=%#v)", loaded, optionalText)
	}
	if query := app.Database().Table(table).Update(&record{Name: "unfiltered"}); query.Error == nil {
		t.Fatal("unfiltered Update() unexpectedly succeeded")
	}
	if query := app.Database().Table(table).Delete(&record{}); query.Error == nil {
		t.Fatal("unfiltered Delete() unexpectedly succeeded")
	}

	preservedRecord := record{TenantID: "other", ItemID: "keep", Name: "preserve"}
	RequireNoORMError(t, app.Database().Table(table).Create(&preservedRecord))
	bulkDeletes := []record{updated, secondRecord}
	query = app.Database().Table(table).Delete(&bulkDeletes)
	RequireNoORMError(t, query)
	if query.RowsAffected != int64(len(bulkDeletes)) {
		t.Fatalf("ScyllaDB composite-key bulk delete affected %d rows, want %d",
			query.RowsAffected, len(bulkDeletes))
	}

	var remainingRecords []record
	RequireNoORMError(t, app.Database().Table(table).Find(&remainingRecords))
	if len(remainingRecords) != 1 || remainingRecords[0].TenantID != preservedRecord.TenantID ||
		remainingRecords[0].ItemID != preservedRecord.ItemID || remainingRecords[0].Name != preservedRecord.Name {
		t.Fatalf("ScyllaDB bulk delete left unexpected records: %#v", remainingRecords)
	}

	scyllaPreloadCoverage(t, app)
}

type preloadCoverageMongoAuthor struct {
	ID     string `gorest:"_id, primary_key"`
	Name   string `gorest:"name"`
	Active bool   `gorest:"active"`
}

type preloadCoverageMongoPost struct {
	ID     string `gorest:"_id, primary_key"`
	UserID string `gorest:"user_id"`
	Title  string `gorest:"title"`
	Status string `gorest:"status"`
}

type preloadCoverageMongoUser struct {
	ID                  string                      `gorest:"_id, primary_key"`
	ORMCoverageAuthorID string                      `gorest:"author_id"`
	Name                string                      `gorest:"name"`
	ORMCoverageAuthor   *preloadCoverageMongoAuthor `gorest:"-" references:"id"`
	ORMCoveragePost     []preloadCoverageMongoPost  `gorest:"-" references:"UserID" orderby:"title DESC"`
}

func mongoPreloadCoverage(t *testing.T, app *gorest.App) {
	t.Helper()

	const (
		authorCollection = "ormcoverageauthors"
		postCollection   = "ormcoverageposts"
	)

	parentCollection := "ormcoverageusers_" + fmt.Sprintf("%x", time.Now().UnixNano())
	explicitCollection := parentCollection + "_explicit"
	authorID := "author-" + fmt.Sprintf("%x", time.Now().UnixNano())
	userID := "user-" + fmt.Sprintf("%x", time.Now().UnixNano())
	secondUserID := userID + "-second"

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	message, status, err := app.Database().MongoCreateCollection(ctx, explicitCollection)
	if err != nil || status != 200 || message == "" {
		t.Fatalf("create MongoDB collection through DBSession = (%q, %d, %v)", message, status, err)
	}
	if _, status, err = app.Database().MongoCreateCollection(ctx, ""); err == nil || status != http.StatusBadRequest {
		t.Fatalf("empty MongoDB collection = (%d, %v), want bad request", status, err)
	}

	explicitDocs, _, _, err := app.Database().MongoCollection(explicitCollection)
	if err != nil {
		t.Fatalf("get explicitly created MongoDB collection: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cleanupCancel()
		if err := explicitDocs.Drop(cleanupCtx); err != nil {
			t.Errorf("drop explicitly created MongoDB collection: %v", err)
		}
	})

	authorDocs, _, _, err := app.Database().MongoCollection(authorCollection)
	if err != nil {
		t.Fatalf("get MongoDB preload author collection: %v", err)
	}

	postDocs, _, _, err := app.Database().MongoCollection(postCollection)
	if err != nil {
		t.Fatalf("get MongoDB preload post collection: %v", err)
	}

	parentDocs, _, _, err := app.Database().MongoCollection(parentCollection)
	if err != nil {
		t.Fatalf("get MongoDB preload parent collection: %v", err)
	}

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		for _, cleanup := range []struct {
			collection *mongo.Collection
			filter     bson.M
		}{
			{collection: authorDocs, filter: bson.M{"_id": authorID}},
			{collection: postDocs, filter: bson.M{"user_id": bson.M{"$in": []string{userID, secondUserID}}}},
			{collection: parentDocs, filter: bson.M{}},
		} {
			if _, err := cleanup.collection.DeleteMany(ctx, cleanup.filter); err != nil {
				t.Errorf("clean up MongoDB preload documents: %v", err)
			}
		}
	})

	author := preloadCoverageMongoAuthor{ID: authorID, Name: "Integration Author", Active: true}

	encodedAuthor, message, status, err := app.Database().EncodeMongoCollection(author)
	if err != nil || status != 200 || message != "" ||
		encodedAuthor["_id"] != author.ID || encodedAuthor["active"] != author.Active {
		t.Fatalf("encode MongoDB document through DBSession = (%#v, %q, %d, %v)", encodedAuthor, message, status, err)
	}

	RequireNoORMError(t, app.Database().Table(authorCollection).Create(&author))

	users := []preloadCoverageMongoUser{
		{ID: userID, ORMCoverageAuthorID: authorID},
		{ID: secondUserID, ORMCoverageAuthorID: authorID},
	}
	RequireNoORMError(t, app.Database().Table(parentCollection).Create(&users))

	posts := []preloadCoverageMongoPost{
		{ID: userID + "-a", UserID: userID, Title: "Alpha", Status: "published"},
		{ID: userID + "-z", UserID: userID, Title: "Zulu", Status: "published"},
		{ID: userID + "-draft", UserID: userID, Title: "Draft", Status: "draft"},
		{ID: secondUserID + "-only", UserID: secondUserID, Title: "Second", Status: "published"},
	}
	RequireNoORMError(t, app.Database().Table(postCollection).Create(&posts))

	var loaded []preloadCoverageMongoUser
	query := app.Database().Table(parentCollection).
		Select("_id").
		Preload("ORMCoverageAuthor", bson.M{"active": true}).
		Preload("ORMCoveragePost", "ormcoverageposts.status = ?", "published").
		Find(&loaded)
	RequireNoORMError(t, query)

	cursor, err := parentDocs.Find(ctx, bson.M{"_id": bson.M{"$in": []string{userID, secondUserID}}})
	if err != nil {
		t.Fatalf("query MongoDB preload parents for cursor decoding: %v", err)
	}

	var decoded []preloadCoverageMongoUser
	message, status, err = app.Database().DecodeMongoCursor(ctx, cursor, &decoded)
	if err != nil || status != 200 || message != "" || len(decoded) != 2 {
		t.Fatalf("decode MongoDB cursor through DBSession = (%d rows, %q, %d, %v)", len(decoded), message, status, err)
	}

	bulkUpdates := []preloadCoverageMongoUser{
		{ID: userID, ORMCoverageAuthorID: authorID, Name: "updated first"},
		{ID: secondUserID, ORMCoverageAuthorID: authorID, Name: "updated second"},
	}
	query = app.Database().Table(parentCollection).Update(&bulkUpdates)
	RequireNoORMError(t, query)
	if query.RowsAffected != int64(len(bulkUpdates)) {
		t.Fatalf("MongoDB bulk update affected %d rows, want %d", query.RowsAffected, len(bulkUpdates))
	}

	missingUpdate := []preloadCoverageMongoUser{
		{ID: userID, ORMCoverageAuthorID: authorID, Name: "must not update"},
		{ID: userID + "-missing", ORMCoverageAuthorID: authorID, Name: "missing"},
	}
	query = app.Database().Table(parentCollection).Update(&missingUpdate)
	if query.Error == nil || query.HTTPCode != 404 {
		t.Fatalf("MongoDB bulk update with missing key = HTTP %d, error %v; want HTTP 404", query.HTTPCode, query.Error)
	}

	loadedByID := make(map[string]preloadCoverageMongoUser, len(loaded))
	for _, user := range loaded {
		loadedByID[user.ID] = user
	}

	for _, id := range []string{userID, secondUserID} {
		user, ok := loadedByID[id]
		if !ok {
			t.Fatalf("MongoDB preload did not return parent %q: %#v", id, loaded)
		}

		if user.ORMCoverageAuthor == nil || user.ORMCoverageAuthor.ID != authorID {
			t.Fatalf("MongoDB belongs-to preload for %q = %#v", id, user.ORMCoverageAuthor)
		}
	}

	first := loadedByID[userID].ORMCoveragePost
	if len(first) != 2 || first[0].Title != "Zulu" || first[1].Title != "Alpha" {
		t.Fatalf("MongoDB has-many preload/filter/order = %#v", first)
	}

	second := loadedByID[secondUserID].ORMCoveragePost
	if len(second) != 1 || second[0].Title != "Second" {
		t.Fatalf("MongoDB second parent preload = %#v", second)
	}
}

type preloadCoverageScyllaAuthor struct {
	ID     string `gorest:"id, primary_key"`
	Name   string `gorest:"name"`
	Active bool   `gorest:"active"`
}

type preloadCoverageScyllaPost struct {
	ID     string `gorest:"id, primary_key"`
	UserID string `gorest:"user_id"`
	Title  string `gorest:"title"`
	Status string `gorest:"status"`
}

type preloadCoverageScyllaUser struct {
	ID                  string                       `gorest:"id, primary_key"`
	ORMCoverageAuthorID string                       `gorest:"author_id"`
	ORMCoverageAuthor   *preloadCoverageScyllaAuthor `gorest:"-" references:"id"`
	ORMCoveragePost     []preloadCoverageScyllaPost  `gorest:"-" references:"UserID" orderby:"title DESC"`
}

func scyllaPreloadCoverage(t *testing.T, app *gorest.App) {
	t.Helper()

	const (
		authorTable = "ormcoverageauthors"
		postTable   = "ormcoverageposts"
	)

	parentTable := "ormcoverageusers_" + fmt.Sprintf("%x", time.Now().UnixNano())
	authorID := "author-" + fmt.Sprintf("%x", time.Now().UnixNano())
	userID := "user-" + fmt.Sprintf("%x", time.Now().UnixNano())
	secondUserID := userID + "-second"

	ExecCQL(t, app, "CREATE TABLE IF NOT EXISTS "+authorTable+
		" (id text PRIMARY KEY, name text, active boolean)")
	ExecCQL(t, app, "CREATE TABLE IF NOT EXISTS "+postTable+
		" (user_id text, id text, title text, status text, PRIMARY KEY (user_id, id))")
	ExecCQL(t, app, "CREATE TABLE "+parentTable+
		" (id text PRIMARY KEY, author_id text)")

	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		for _, statement := range []struct {
			query string
			args  []any
		}{
			{query: "DELETE FROM " + authorTable + " WHERE id = ?", args: []any{authorID}},
			{query: "DELETE FROM " + postTable + " WHERE user_id = ?", args: []any{userID}},
			{query: "DELETE FROM " + postTable + " WHERE user_id = ?", args: []any{secondUserID}},
			{query: "DROP TABLE " + parentTable},
		} {
			if _, _, err := app.Database().ExecCQL(ctx, statement.query, statement.args...); err != nil {
				t.Errorf("clean up ScyllaDB preload data: %v", err)
			}
		}
	})

	author := preloadCoverageScyllaAuthor{ID: authorID, Name: "Integration Author", Active: true}
	RequireNoORMError(t, app.Database().Table(authorTable).Create(&author))

	users := []preloadCoverageScyllaUser{
		{ID: userID, ORMCoverageAuthorID: authorID},
		{ID: secondUserID, ORMCoverageAuthorID: authorID},
	}
	RequireNoORMError(t, app.Database().Table(parentTable).Create(&users))

	posts := []preloadCoverageScyllaPost{
		{ID: userID + "-a", UserID: userID, Title: "Alpha", Status: "published"},
		{ID: userID + "-z", UserID: userID, Title: "Zulu", Status: "published"},
		{ID: userID + "-draft", UserID: userID, Title: "Draft", Status: "draft"},
		{ID: secondUserID + "-only", UserID: secondUserID, Title: "Second", Status: "published"},
	}
	RequireNoORMError(t, app.Database().Table(postTable).Create(&posts))

	var loaded preloadCoverageScyllaUser
	query := app.Database().Table(parentTable).
		Select(parentTable+".id").
		Where(parentTable+".id = ?", userID).
		AllowFiltering().
		Preload("ORMCoverageAuthor", "ormcoverageauthors.active = ?", true).
		Preload("ORMCoveragePost", "ormcoverageposts.status = ?", "published").
		Find(&loaded)
	RequireNoORMError(t, query)

	if loaded.ID != userID || loaded.ORMCoverageAuthor == nil || loaded.ORMCoverageAuthor.ID != authorID {
		t.Fatalf("ScyllaDB belongs-to preload = %#v", loaded)
	}

	if len(loaded.ORMCoveragePost) != 2 ||
		loaded.ORMCoveragePost[0].Title != "Zulu" ||
		loaded.ORMCoveragePost[1].Title != "Alpha" {
		t.Fatalf("ScyllaDB has-many preload/filter/order = %#v", loaded.ORMCoveragePost)
	}
}
