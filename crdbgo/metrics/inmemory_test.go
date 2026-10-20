package metrics

import (
	"sync"
	"testing"
	"time"

	"github.com/felipeg48/cockroachdb-go-library/crdbgo"
)

// Compile-time interface checks.
var (
	_ crdbgo.MetricsRecorder = (*InMemoryRecorder)(nil)
	_ Snapshotter            = (*InMemoryRecorder)(nil)
)

func TestNewInMemoryRecorder_ZeroSnapshot(t *testing.T) {
	r := NewInMemoryRecorder()
	s := r.Snapshot()
	if s.TransactionCount != 0 || s.TransactionSuccessCount != 0 || s.TransactionFailureCount != 0 ||
		s.RetryCount != 0 || s.FollowerReadCount != 0 ||
		s.TotalTransactionDuration != 0 || s.TotalRetryDelay != 0 || s.TotalFollowerReadDuration != 0 {
		t.Fatalf("new recorder should have zero snapshot, got %+v", s)
	}
}

func TestRecordTransaction_Success(t *testing.T) {
	r := NewInMemoryRecorder()
	r.RecordTransaction(100*time.Millisecond, 0, true)

	s := r.Snapshot()
	if s.TransactionCount != 1 {
		t.Errorf("TransactionCount = %d, want 1", s.TransactionCount)
	}
	if s.TransactionSuccessCount != 1 {
		t.Errorf("TransactionSuccessCount = %d, want 1", s.TransactionSuccessCount)
	}
	if s.TransactionFailureCount != 0 {
		t.Errorf("TransactionFailureCount = %d, want 0", s.TransactionFailureCount)
	}
	if s.TotalTransactionDuration != 100*time.Millisecond {
		t.Errorf("TotalTransactionDuration = %v, want 100ms", s.TotalTransactionDuration)
	}
}

func TestRecordTransaction_Failure(t *testing.T) {
	r := NewInMemoryRecorder()
	r.RecordTransaction(50*time.Millisecond, 2, false)

	s := r.Snapshot()
	if s.TransactionCount != 1 {
		t.Errorf("TransactionCount = %d, want 1", s.TransactionCount)
	}
	if s.TransactionSuccessCount != 0 {
		t.Errorf("TransactionSuccessCount = %d, want 0", s.TransactionSuccessCount)
	}
	if s.TransactionFailureCount != 1 {
		t.Errorf("TransactionFailureCount = %d, want 1", s.TransactionFailureCount)
	}
	if s.RetryCount != 2 {
		t.Errorf("RetryCount = %d, want 2", s.RetryCount)
	}
}

func TestRecordTransaction_RetriesAccumulate(t *testing.T) {
	r := NewInMemoryRecorder()
	r.RecordTransaction(10*time.Millisecond, 3, true)
	r.RecordTransaction(20*time.Millisecond, 1, false)

	s := r.Snapshot()
	if s.RetryCount != 4 {
		t.Errorf("RetryCount = %d, want 4", s.RetryCount)
	}
	if s.TotalTransactionDuration != 30*time.Millisecond {
		t.Errorf("TotalTransactionDuration = %v, want 30ms", s.TotalTransactionDuration)
	}
}

func TestRecordTransaction_MultipleSuccessFailure(t *testing.T) {
	r := NewInMemoryRecorder()
	r.RecordTransaction(10*time.Millisecond, 0, true)
	r.RecordTransaction(20*time.Millisecond, 0, true)
	r.RecordTransaction(30*time.Millisecond, 0, false)

	s := r.Snapshot()
	if s.TransactionCount != 3 {
		t.Errorf("TransactionCount = %d, want 3", s.TransactionCount)
	}
	if s.TransactionSuccessCount != 2 {
		t.Errorf("TransactionSuccessCount = %d, want 2", s.TransactionSuccessCount)
	}
	if s.TransactionFailureCount != 1 {
		t.Errorf("TransactionFailureCount = %d, want 1", s.TransactionFailureCount)
	}
}

func TestRecordRetry(t *testing.T) {
	r := NewInMemoryRecorder()
	r.RecordRetry(50 * time.Millisecond)
	r.RecordRetry(100 * time.Millisecond)

	s := r.Snapshot()
	if s.RetryCount != 2 {
		t.Errorf("RetryCount = %d, want 2", s.RetryCount)
	}
	if s.TotalRetryDelay != 150*time.Millisecond {
		t.Errorf("TotalRetryDelay = %v, want 150ms", s.TotalRetryDelay)
	}
}

func TestRecordFollowerRead(t *testing.T) {
	r := NewInMemoryRecorder()
	r.RecordFollowerRead(5 * time.Millisecond)
	r.RecordFollowerRead(10 * time.Millisecond)
	r.RecordFollowerRead(15 * time.Millisecond)

	s := r.Snapshot()
	if s.FollowerReadCount != 3 {
		t.Errorf("FollowerReadCount = %d, want 3", s.FollowerReadCount)
	}
	if s.TotalFollowerReadDuration != 30*time.Millisecond {
		t.Errorf("TotalFollowerReadDuration = %v, want 30ms", s.TotalFollowerReadDuration)
	}
}

func TestSnapshot_ConsistentValues(t *testing.T) {
	r := NewInMemoryRecorder()
	r.RecordTransaction(100*time.Millisecond, 1, true)
	r.RecordRetry(25 * time.Millisecond)
	r.RecordFollowerRead(10 * time.Millisecond)

	s := r.Snapshot()

	if s.TransactionCount != 1 {
		t.Errorf("TransactionCount = %d, want 1", s.TransactionCount)
	}
	// RetryCount = 1 from RecordTransaction + 1 from RecordRetry
	if s.RetryCount != 2 {
		t.Errorf("RetryCount = %d, want 2", s.RetryCount)
	}
	if s.FollowerReadCount != 1 {
		t.Errorf("FollowerReadCount = %d, want 1", s.FollowerReadCount)
	}
	if s.TotalRetryDelay != 25*time.Millisecond {
		t.Errorf("TotalRetryDelay = %v, want 25ms", s.TotalRetryDelay)
	}
}

func TestReset(t *testing.T) {
	r := NewInMemoryRecorder()
	r.RecordTransaction(100*time.Millisecond, 3, true)
	r.RecordRetry(50 * time.Millisecond)
	r.RecordFollowerRead(10 * time.Millisecond)

	r.Reset()
	s := r.Snapshot()

	if s.TransactionCount != 0 || s.TransactionSuccessCount != 0 || s.TransactionFailureCount != 0 ||
		s.RetryCount != 0 || s.FollowerReadCount != 0 ||
		s.TotalTransactionDuration != 0 || s.TotalRetryDelay != 0 || s.TotalFollowerReadDuration != 0 {
		t.Fatalf("after Reset, snapshot should be zero, got %+v", s)
	}
}

func TestReset_ThenRecord(t *testing.T) {
	r := NewInMemoryRecorder()
	r.RecordTransaction(100*time.Millisecond, 5, true)
	r.Reset()
	r.RecordTransaction(50*time.Millisecond, 1, false)

	s := r.Snapshot()
	if s.TransactionCount != 1 {
		t.Errorf("TransactionCount = %d, want 1", s.TransactionCount)
	}
	if s.TransactionSuccessCount != 0 {
		t.Errorf("TransactionSuccessCount = %d, want 0", s.TransactionSuccessCount)
	}
	if s.TransactionFailureCount != 1 {
		t.Errorf("TransactionFailureCount = %d, want 1", s.TransactionFailureCount)
	}
	if s.RetryCount != 1 {
		t.Errorf("RetryCount = %d, want 1", s.RetryCount)
	}
	if s.TotalTransactionDuration != 50*time.Millisecond {
		t.Errorf("TotalTransactionDuration = %v, want 50ms", s.TotalTransactionDuration)
	}
}

func TestSnapshot_Immutability(t *testing.T) {
	r := NewInMemoryRecorder()
	r.RecordTransaction(100*time.Millisecond, 0, true)

	s1 := r.Snapshot()
	r.RecordTransaction(200*time.Millisecond, 0, true)
	s2 := r.Snapshot()

	if s1.TransactionCount != 1 {
		t.Errorf("s1.TransactionCount = %d, want 1", s1.TransactionCount)
	}
	if s2.TransactionCount != 2 {
		t.Errorf("s2.TransactionCount = %d, want 2", s2.TransactionCount)
	}
}

func TestConcurrentAccess(t *testing.T) {
	r := NewInMemoryRecorder()
	const goroutines = 100
	const opsPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < opsPerGoroutine; j++ {
				r.RecordTransaction(time.Millisecond, 1, j%2 == 0)
				r.RecordRetry(time.Millisecond)
				r.RecordFollowerRead(time.Millisecond)
			}
		}()
	}

	wg.Wait()
	s := r.Snapshot()

	expectedTxn := int64(goroutines * opsPerGoroutine)
	if s.TransactionCount != expectedTxn {
		t.Errorf("TransactionCount = %d, want %d", s.TransactionCount, expectedTxn)
	}
	if s.TransactionSuccessCount+s.TransactionFailureCount != expectedTxn {
		t.Errorf("success(%d) + failure(%d) != %d", s.TransactionSuccessCount, s.TransactionFailureCount, expectedTxn)
	}
	// RetryCount = 1 per RecordTransaction + 1 per RecordRetry = 2 * total ops
	expectedRetries := expectedTxn * 2
	if s.RetryCount != expectedRetries {
		t.Errorf("RetryCount = %d, want %d", s.RetryCount, expectedRetries)
	}
	if s.FollowerReadCount != expectedTxn {
		t.Errorf("FollowerReadCount = %d, want %d", s.FollowerReadCount, expectedTxn)
	}
}

func TestConcurrentAccessWithReset(t *testing.T) {
	r := NewInMemoryRecorder()
	const goroutines = 50

	var wg sync.WaitGroup
	wg.Add(goroutines + 1)

	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				r.RecordTransaction(time.Millisecond, 1, true)
				r.RecordRetry(time.Millisecond)
				r.RecordFollowerRead(time.Millisecond)
				_ = r.Snapshot()
			}
		}()
	}

	// One goroutine periodically resets — no panics or races expected.
	go func() {
		defer wg.Done()
		for j := 0; j < 20; j++ {
			r.Reset()
		}
	}()

	wg.Wait()
}
