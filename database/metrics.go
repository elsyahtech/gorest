package database

// OperationalMetrics reports current pool usage and cumulative database-client counters.
func (db *Database) OperationalMetrics() OperationalMetrics {
	var result OperationalMetrics
	if db == nil {
		return result
	}

	if db.SQL != nil {
		stats := db.SQL.Stats()
		result.ConnectionActive = metricInt64(int64(stats.InUse))
		result.ConnectionIdle = metricInt64(int64(stats.Idle))
		result.ConnectionWaitCount = metricInt64(stats.WaitCount)

		waitDuration := stats.WaitDuration.String()

		result.ConnectionWaitDuration = &waitDuration
		if maxOpen := stats.MaxOpenConnections; maxOpen > 0 {
			result.ConnectionMax = metricInt64(int64(maxOpen))
			usage := float64(stats.InUse) / float64(maxOpen) * 100
			result.PoolUsagePercent = &usage
		}
	}

	if db.metrics == nil {
		return result
	}

	snapshot := db.metrics.Snapshot()
	if snapshot.QueryAvailable {
		result.QueryTotal = metricInt64(snapshot.QueryTotal)
		result.QuerySlowTotal = metricInt64(snapshot.QuerySlowTotal)
		result.QueryErrorTotal = metricInt64(snapshot.QueryErrorTotal)
	}

	if snapshot.TransactionAvailable {
		result.TransactionCommitTotal = metricInt64(snapshot.TransactionCommitTotal)
		result.TransactionRollbackTotal = metricInt64(snapshot.TransactionRollbackTotal)
	}

	if snapshot.PoolConnectionsAvailable && db.SQL == nil {
		result.ConnectionActive = metricInt64(snapshot.PoolActive)
		result.ConnectionIdle = metricInt64(snapshot.PoolIdle)
	}

	if snapshot.PoolMaxAvailable && db.SQL == nil {
		result.ConnectionMax = metricInt64(snapshot.PoolMax)
		if snapshot.PoolMax > 0 {
			usage := float64(snapshot.PoolActive) / float64(snapshot.PoolMax) * 100
			result.PoolUsagePercent = &usage
		}
	}

	return result
}

func metricInt64(value int64) *int64 {
	return &value
}
