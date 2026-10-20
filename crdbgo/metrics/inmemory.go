package metrics

import (
	"sync/atomic"
	"time"
)

// InMemoryRecorder implements [crdbgo.MetricsRecorder] using atomic counters.
// It is thread-safe and suitable for testing and development. Use [Snapshot]
// to read accumulated metrics, and [Reset] to zero all counters.
type InMemoryRecorder struct {
	transactionCount          atomic.Int64
	transactionSuccessCount   atomic.Int64
	transactionFailureCount   atomic.Int64
	retryCount                atomic.Int64
	followerReadCount         atomic.Int64
	totalTransactionDuration  atomic.Int64 // nanoseconds
	totalRetryDelay           atomic.Int64 // nanoseconds
	totalFollowerReadDuration atomic.Int64 // nanoseconds
}

// NewInMemoryRecorder creates a new in-memory metrics recorder with all
// counters initialized to zero.
func NewInMemoryRecorder() *InMemoryRecorder {
	return &InMemoryRecorder{}
}

// RecordTransaction records a completed transaction with its duration, retry
// count, and success/failure outcome.
func (r *InMemoryRecorder) RecordTransaction(duration time.Duration, retries int, success bool) {
	r.transactionCount.Add(1)
	if success {
		r.transactionSuccessCount.Add(1)
	} else {
		r.transactionFailureCount.Add(1)
	}
	r.totalTransactionDuration.Add(int64(duration))
	r.retryCount.Add(int64(retries))
}

// RecordRetry records a single retry attempt and its backoff delay.
func (r *InMemoryRecorder) RecordRetry(delay time.Duration) {
	r.retryCount.Add(1)
	r.totalRetryDelay.Add(int64(delay))
}

// RecordFollowerRead records a follower read query and its latency.
func (r *InMemoryRecorder) RecordFollowerRead(duration time.Duration) {
	r.followerReadCount.Add(1)
	r.totalFollowerReadDuration.Add(int64(duration))
}

// Snapshot returns a point-in-time copy of all accumulated metrics.
func (r *InMemoryRecorder) Snapshot() Snapshot {
	return Snapshot{
		TransactionCount:          r.transactionCount.Load(),
		TransactionSuccessCount:   r.transactionSuccessCount.Load(),
		TransactionFailureCount:   r.transactionFailureCount.Load(),
		RetryCount:                r.retryCount.Load(),
		FollowerReadCount:         r.followerReadCount.Load(),
		TotalTransactionDuration:  time.Duration(r.totalTransactionDuration.Load()),
		TotalRetryDelay:           time.Duration(r.totalRetryDelay.Load()),
		TotalFollowerReadDuration: time.Duration(r.totalFollowerReadDuration.Load()),
	}
}

// Reset zeroes all counters. Useful between test cases.
func (r *InMemoryRecorder) Reset() {
	r.transactionCount.Store(0)
	r.transactionSuccessCount.Store(0)
	r.transactionFailureCount.Store(0)
	r.retryCount.Store(0)
	r.followerReadCount.Store(0)
	r.totalTransactionDuration.Store(0)
	r.totalRetryDelay.Store(0)
	r.totalFollowerReadDuration.Store(0)
}
