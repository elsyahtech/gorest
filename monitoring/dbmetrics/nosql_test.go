package dbmetrics

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.mongodb.org/mongo-driver/v2/event"
)

func TestMongoMonitorsRecordCommandsTransactionsAndPoolState(t *testing.T) {
	stats := New()
	commandMonitor := stats.MongoCommandMonitor()
	ctx := context.Background()

	commandMonitor.Started(ctx, &event.CommandStartedEvent{
		CommandName: "find",
		RequestID:   1,
	})
	commandMonitor.Succeeded(ctx, &event.CommandSucceededEvent{
		CommandFinishedEvent: event.CommandFinishedEvent{
			CommandName: "find",
			RequestID:   1,
			Duration:    SlowQueryThreshold,
		},
	})
	commandMonitor.Started(ctx, &event.CommandStartedEvent{
		CommandName: "insert",
		RequestID:   2,
	})
	commandMonitor.Failed(ctx, &event.CommandFailedEvent{
		CommandFinishedEvent: event.CommandFinishedEvent{
			CommandName: "insert",
			RequestID:   2,
			Duration:    time.Millisecond,
		},
		Failure: errors.New("write failed"),
	})
	commandMonitor.Started(ctx, &event.CommandStartedEvent{
		CommandName: "commitTransaction",
		RequestID:   3,
	})
	commandMonitor.Succeeded(ctx, &event.CommandSucceededEvent{
		CommandFinishedEvent: event.CommandFinishedEvent{
			CommandName: "commitTransaction",
			RequestID:   3,
		},
	})

	poolMonitor := stats.MongoPoolMonitor()
	poolMonitor.Event(&event.PoolEvent{
		Type:        event.ConnectionPoolCreated,
		Address:     "db.example:27017",
		PoolOptions: &event.MonitorPoolOptions{MaxPoolSize: 8},
	})
	poolMonitor.Event(&event.PoolEvent{
		Type:         event.ConnectionReady,
		Address:      "db.example:27017",
		ConnectionID: 1,
	})
	poolMonitor.Event(&event.PoolEvent{
		Type:         event.ConnectionCheckedOut,
		Address:      "db.example:27017",
		ConnectionID: 1,
	})

	snapshot := stats.Snapshot()
	if !snapshot.QueryAvailable || !snapshot.TransactionAvailable {
		t.Fatal("MongoDB command monitor did not mark counters available")
	}
	if snapshot.QueryTotal != 3 || snapshot.QuerySlowTotal != 1 || snapshot.QueryErrorTotal != 1 {
		t.Errorf("MongoDB command counters = (%d, %d, %d), want (3, 1, 1)",
			snapshot.QueryTotal, snapshot.QuerySlowTotal, snapshot.QueryErrorTotal)
	}
	if snapshot.TransactionCommitTotal != 1 || snapshot.TransactionRollbackTotal != 0 {
		t.Errorf("MongoDB transaction counters = (%d, %d), want (1, 0)",
			snapshot.TransactionCommitTotal, snapshot.TransactionRollbackTotal)
	}
	if !snapshot.PoolConnectionsAvailable || !snapshot.PoolMaxAvailable ||
		snapshot.PoolActive != 1 || snapshot.PoolIdle != 0 || snapshot.PoolMax != 8 {
		t.Errorf("MongoDB pool snapshot = %#v; want active=1 idle=0 max=8", snapshot)
	}
}
