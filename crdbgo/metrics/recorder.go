package metrics

import "time"

// Snapshot holds a point-in-time copy of all accumulated metrics counters.
// Obtain one from [InMemoryRecorder.Snapshot].
type Snapshot struct {
	TransactionCount          int64
	TransactionSuccessCount   int64
	TransactionFailureCount   int64
	RetryCount                int64
	FollowerReadCount         int64
	TotalTransactionDuration  time.Duration
	TotalRetryDelay           time.Duration
	TotalFollowerReadDuration time.Duration
}

// Snapshotter is implemented by recorders that support point-in-time metric snapshots.
type Snapshotter interface {
	Snapshot() Snapshot
}
