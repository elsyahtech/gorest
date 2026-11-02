package dbmetrics

import (
	"sync"
	"sync/atomic"
	"time"
)

// SlowQueryThreshold is the minimum operation duration counted as slow.
const SlowQueryThreshold = time.Second

// Snapshot contains a consistent view of cumulative driver counters.
type Snapshot struct {
	QueryTotal               int64
	QuerySlowTotal           int64
	QueryErrorTotal          int64
	TransactionCommitTotal   int64
	TransactionRollbackTotal int64
	PoolActive               int64
	PoolIdle                 int64
	PoolMax                  int64
	QueryAvailable           bool
	TransactionAvailable     bool
	PoolConnectionsAvailable bool
	PoolMaxAvailable         bool
}

// Stats stores application-scoped database driver metrics.
type Stats struct {
	mongoCommands map[int64]time.Time
	mongoPools    map[string]*mongoPool
	mongoMu       sync.Mutex

	queryTotal               atomic.Int64
	querySlowTotal           atomic.Int64
	queryErrorTotal          atomic.Int64
	transactionCommitTotal   atomic.Int64
	transactionRollbackTotal atomic.Int64
	queryAvailable           atomic.Bool
	transactionAvailable     atomic.Bool
}

type mongoPool struct {
	connections map[int64]bool
	max         int64
}

// New creates a metrics collector for one configured database client.
func New() *Stats {
	return &Stats{
		mongoCommands: make(map[int64]time.Time),
		mongoPools:    make(map[string]*mongoPool),
	}
}

// EnableQueries marks query counters as available for this client.
func (s *Stats) EnableQueries() {
	s.queryAvailable.Store(true)
}

func (s *Stats) enableTransactions() {
	s.transactionAvailable.Store(true)
}

func (s *Stats) startQuery() time.Time {
	s.queryTotal.Add(1)
	return time.Now()
}

func (s *Stats) skipQuery() {
	s.queryTotal.Add(-1)
}

func (s *Stats) finishQuery(start time.Time, err error) {
	if err != nil {
		s.queryErrorTotal.Add(1)
	}

	if time.Since(start) >= SlowQueryThreshold {
		s.querySlowTotal.Add(1)
	}
}

func (s *Stats) recordCommit() {
	s.transactionCommitTotal.Add(1)
}

func (s *Stats) recordRollback() {
	s.transactionRollbackTotal.Add(1)
}

func (s *Stats) Snapshot() Snapshot {
	result := Snapshot{
		QueryTotal:               s.queryTotal.Load(),
		QuerySlowTotal:           s.querySlowTotal.Load(),
		QueryErrorTotal:          s.queryErrorTotal.Load(),
		TransactionCommitTotal:   s.transactionCommitTotal.Load(),
		TransactionRollbackTotal: s.transactionRollbackTotal.Load(),
		QueryAvailable:           s.queryAvailable.Load(),
		TransactionAvailable:     s.transactionAvailable.Load(),
	}

	s.mongoMu.Lock()
	defer s.mongoMu.Unlock()

	if len(s.mongoPools) == 0 {
		return result
	}

	result.PoolConnectionsAvailable = true
	result.PoolMaxAvailable = true

	for _, pool := range s.mongoPools {
		if pool.max <= 0 {
			result.PoolMaxAvailable = false
		}

		result.PoolMax += pool.max
		for _, inUse := range pool.connections {
			if inUse {
				result.PoolActive++
			} else {
				result.PoolIdle++
			}
		}
	}

	return result
}
