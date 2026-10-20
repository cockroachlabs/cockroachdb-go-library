// Package crdbtest provides testing utilities for CockroachDB Go applications.
//
// It includes:
//   - [Harness]: test setup with schema execution and cleanup
//   - [ConcurrencyRunner]: barrier-synchronized concurrent execution for contention testing
//   - [RetrySimulator]: simulates SQLSTATE 40001 errors without a live database
//   - [FollowerReadSimulator]: validates AS OF SYSTEM TIME clause injection
package crdbtest
