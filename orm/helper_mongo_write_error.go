package orm

import (
	"fmt"
	"net/http"

	"go.mongodb.org/mongo-driver/v2/mongo"
)

func (orm *ORM) setMongoWriteError(err error) error {
	if mongo.IsDuplicateKeyError(err) {
		return orm.setError(
			"A record with the same value for a primary key or unique column already exists.",
			err,
			http.StatusConflict,
		)
	}

	return orm.setError(
		"Check that MongoDB is reachable and the collection, document fields, BSON types, and write options are valid.",
		fmt.Errorf("MongoDB write: %w", err),
	)
}
