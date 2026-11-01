package monitoring

import (
	"context"
	"errors"
	"fmt"
	"math"

	"github.com/elsyahtech/gorest/database"
	"go.mongodb.org/mongo-driver/v2/bson"
)

func queryMongoStorage(ctx context.Context, databaseConn *database.Database, databaseName string) (int64, error) {
	if databaseName == "" {
		return 0, errors.New("configured MongoDB database name is empty")
	}

	if databaseConn.Mongo == nil {
		return 0, errors.New("MongoDB connection is unavailable")
	}

	var stats struct {
		StorageSize int64 `bson:"storageSize"`
		IndexSize   int64 `bson:"indexSize"`
	}

	result := databaseConn.Mongo.Database(databaseName).RunCommand(ctx, bson.D{{Key: "dbStats", Value: 1}})

	if err := result.Decode(&stats); err != nil {
		return 0, fmt.Errorf("decode MongoDB storage statistics: %w", err)
	}

	if stats.StorageSize < 0 || stats.IndexSize < 0 || stats.StorageSize > math.MaxInt64-stats.IndexSize {
		return 0, errors.New("MongoDB returned invalid storage statistics")
	}

	return stats.StorageSize + stats.IndexSize, nil
}

func queryScyllaStorage(ctx context.Context, databaseConn *database.Database, keyspace string) (int64, error) {
	if keyspace == "" {
		return 0, errors.New("configured ScyllaDB keyspace is empty")
	}

	if databaseConn.Scylla == nil {
		return 0, errors.New("ScyllaDB connection is unavailable")
	}

	iter := databaseConn.Scylla.Query(
		"SELECT mean_partition_size, partitions_count FROM system.size_estimates WHERE keyspace_name = ?",
		keyspace,
	).IterContext(ctx)
	if iter == nil {
		return 0, errors.New("ScyllaDB returned no storage estimate iterator")
	}

	var (
		usedBytes         int64
		meanPartitionSize int64
		partitionsCount   int64
	)

	hasEstimate := false

	for iter.Scan(&meanPartitionSize, &partitionsCount) {
		hasEstimate = true

		if meanPartitionSize < 0 || partitionsCount < 0 ||
			(meanPartitionSize != 0 && partitionsCount > math.MaxInt64/meanPartitionSize) {
			estimateErr := errors.New("ScyllaDB returned invalid storage estimates")
			if err := iter.Close(); err != nil {
				estimateErr = errors.Join(estimateErr, fmt.Errorf("close ScyllaDB storage iterator: %w", err))
			}

			return 0, estimateErr
		}

		estimate := meanPartitionSize * partitionsCount
		if usedBytes > math.MaxInt64-estimate {
			estimateErr := errors.New("ScyllaDB storage estimate overflow")
			if err := iter.Close(); err != nil {
				estimateErr = errors.Join(estimateErr, fmt.Errorf("close ScyllaDB storage iterator: %w", err))
			}

			return 0, estimateErr
		}

		usedBytes += estimate
	}

	if err := iter.Close(); err != nil {
		return 0, fmt.Errorf("close ScyllaDB storage iterator: %w", err)
	}

	if !hasEstimate {
		return 0, errors.New("ScyllaDB has no storage estimates for the configured keyspace")
	}

	return usedBytes, nil
}
