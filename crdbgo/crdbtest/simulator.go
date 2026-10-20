package crdbtest

import (
	"fmt"
	"strings"
	"sync"
	"testing"
)

// SimulatedRetryError mimics a CockroachDB serialization failure (SQLSTATE 40001)
// for unit tests that need to exercise retry logic without a live database.
type SimulatedRetryError struct {
	Attempt int
}

// Error returns a descriptive message including the attempt number and SQLSTATE.
func (e *SimulatedRetryError) Error() string {
	return fmt.Sprintf("crdbtest: simulated retry error (attempt %d, SQLSTATE 40001)", e.Attempt)
}

// SQLState returns "40001" (serialization failure), satisfying the sqlStateError
// interface used by [crdbgo.IsRetryable].
func (e *SimulatedRetryError) SQLState() string {
	return "40001"
}

// RetrySimulator simulates CockroachDB serialization failures for a configurable
// number of attempts, then succeeds. Thread-safe for use with [ConcurrencyRunner].
type RetrySimulator struct {
	failCount    int
	currentCount int
	mu           sync.Mutex
}

// NewRetrySimulator creates a simulator that returns [SimulatedRetryError] for
// the first failCount calls, then delegates to the provided function.
func NewRetrySimulator(failCount int) *RetrySimulator {
	return &RetrySimulator{failCount: failCount}
}

// Execute returns a SimulatedRetryError for the first failCount invocations,
// then delegates to fn.
func (s *RetrySimulator) Execute(fn func() error) error {
	s.mu.Lock()
	count := s.currentCount
	s.currentCount++
	s.mu.Unlock()

	if count < s.failCount {
		return &SimulatedRetryError{Attempt: count}
	}
	return fn()
}

// Reset zeroes the internal counter so the simulator can be reused.
func (s *RetrySimulator) Reset() {
	s.mu.Lock()
	s.currentCount = 0
	s.mu.Unlock()
}

// FollowerReadSimulator provides test helpers for verifying that queries include
// CockroachDB's AS OF SYSTEM TIME follower_read_timestamp() clause.
type FollowerReadSimulator struct{}

// NewFollowerReadSimulator creates a new follower read test simulator.
func NewFollowerReadSimulator() *FollowerReadSimulator {
	return &FollowerReadSimulator{}
}

// SimulateRead appends AS OF SYSTEM TIME follower_read_timestamp() to the query,
// simulating what the adapter follower read methods do.
func (f *FollowerReadSimulator) SimulateRead(query string) string {
	return query + " AS OF SYSTEM TIME follower_read_timestamp()"
}

// AssertFollowerRead fails the test if query does not contain an AS OF SYSTEM
// TIME clause.
func (f *FollowerReadSimulator) AssertFollowerRead(t *testing.T, query string) {
	t.Helper()
	if !strings.Contains(query, "AS OF SYSTEM TIME") {
		t.Errorf("expected query to contain AS OF SYSTEM TIME clause, got: %s", query)
	}
}
