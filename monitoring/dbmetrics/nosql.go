//nolint:wsl_v5 // Driver event hooks are adapters that preserve native event semantics.
package dbmetrics

import (
	"context"
	"strings"
	"time"

	gocql "github.com/apache/cassandra-gocql-driver/v2"
	"go.mongodb.org/mongo-driver/v2/event"
)

// MongoCommandMonitor counts application commands and transaction outcomes.
func (s *Stats) MongoCommandMonitor() *event.CommandMonitor {
	s.EnableQueries()
	s.enableTransactions()
	return &event.CommandMonitor{
		Started: func(_ context.Context, command *event.CommandStartedEvent) {
			if isMongoInternalCommand(command.CommandName) {
				return
			}
			s.queryTotal.Add(1)
			s.mongoMu.Lock()
			s.mongoCommands[command.RequestID] = time.Now()
			s.mongoMu.Unlock()
		},
		Succeeded: func(_ context.Context, command *event.CommandSucceededEvent) {
			s.finishMongoCommand(command.RequestID, command.Duration, nil)

			switch strings.ToLower(command.CommandName) {
			case "committransaction":
				s.recordCommit()
			case "aborttransaction":
				s.recordRollback()
			default:
				return
			}
		},
		Failed: func(_ context.Context, command *event.CommandFailedEvent) {
			s.finishMongoCommand(command.RequestID, command.Duration, command.Failure)
		},
	}
}

// MongoPoolMonitor tracks active, idle, and configured maximum pool sizes.
func (s *Stats) MongoPoolMonitor() *event.PoolMonitor {
	return &event.PoolMonitor{
		Event: func(poolEvent *event.PoolEvent) {
			s.recordMongoPoolEvent(poolEvent)
		},
	}
}

func (s *Stats) recordMongoPoolEvent(poolEvent *event.PoolEvent) {
	s.mongoMu.Lock()
	defer s.mongoMu.Unlock()

	pool := s.mongoPools[poolEvent.Address]
	if pool == nil {
		pool = &mongoPool{connections: make(map[int64]bool)}
		s.mongoPools[poolEvent.Address] = pool
	}
	if poolEvent.PoolOptions != nil && poolEvent.PoolOptions.MaxPoolSize > 0 {
		pool.max = int64(poolEvent.PoolOptions.MaxPoolSize)
	}

	switch poolEvent.Type {
	case event.ConnectionReady, event.ConnectionCheckedIn:
		pool.connections[poolEvent.ConnectionID] = false
	case event.ConnectionCheckedOut:
		pool.connections[poolEvent.ConnectionID] = true
	case event.ConnectionClosed:
		delete(pool.connections, poolEvent.ConnectionID)
	case event.ConnectionPoolClosed:
		delete(s.mongoPools, poolEvent.Address)
	default:
		return
	}
}

func (s *Stats) finishMongoCommand(requestID int64, duration time.Duration, err error) {
	s.mongoMu.Lock()
	start, found := s.mongoCommands[requestID]
	delete(s.mongoCommands, requestID)
	s.mongoMu.Unlock()

	if !found {
		return
	}
	if err != nil {
		s.queryErrorTotal.Add(1)
	}
	elapsed := duration
	if elapsed <= 0 {
		elapsed = time.Since(start)
	}
	if elapsed >= SlowQueryThreshold {
		s.querySlowTotal.Add(1)
	}
}

func isMongoInternalCommand(command string) bool {
	switch strings.ToLower(command) {
	case "hello", "ismaster", "saslstart", "saslcontinue", "authenticate", "getnonce":
		return true
	default:
		return false
	}
}

// ObserveQuery records a query reported by the ScyllaDB driver.
func (s *Stats) ObserveQuery(_ context.Context, query gocql.ObservedQuery) {
	s.recordObservedQuery(query.Start, query.End, query.Err)
}

// ObserveBatch records a batch reported by the ScyllaDB driver.
func (s *Stats) ObserveBatch(_ context.Context, batch gocql.ObservedBatch) {
	s.recordObservedQuery(batch.Start, batch.End, batch.Err)
}

func (s *Stats) recordObservedQuery(start, end time.Time, err error) {
	s.queryTotal.Add(1)
	if err != nil {
		s.queryErrorTotal.Add(1)
	}
	if end.Sub(start) >= SlowQueryThreshold {
		s.querySlowTotal.Add(1)
	}
}
