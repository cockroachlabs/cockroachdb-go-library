package main

import (
	"testing"
	"time"

	"github.com/cockroachlabs/cockroachdb-go-library/crdbgo/metrics"
)

// TestInMemoryRecorder_CountsTransactionsAndRetries exercises the recorder the
// metered pool uses — no database, no container. It mirrors what the pool does
// internally: record a successful transaction that took two retries, then read
// the snapshot back.
func TestInMemoryRecorder_CountsTransactionsAndRetries(t *testing.T) {
	rec := metrics.NewInMemoryRecorder()

	// A single successful transaction that the pool retried twice before committing.
	rec.RecordTransaction(5*time.Millisecond, 2, true)

	snap := rec.Snapshot()
	if snap.TransactionCount != 1 {
		t.Fatalf("TransactionCount = %d, want 1", snap.TransactionCount)
	}
	if snap.TransactionSuccessCount != 1 {
		t.Fatalf("TransactionSuccessCount = %d, want 1", snap.TransactionSuccessCount)
	}
	if snap.RetryCount != 2 {
		t.Fatalf("RetryCount = %d, want 2", snap.RetryCount)
	}
}
