// Package circuitbreaker provides a circuit breaker for CockroachDB operations.
//
// The circuit breaker prevents cascading failures by tracking consecutive errors
// and temporarily blocking operations when the failure threshold is reached.
// It follows the standard three-state model:
//
//   - Closed: Normal operation. Failures are counted.
//   - Open: All operations are blocked with CircuitBreakerOpenError.
//   - HalfOpen: After the break duration expires, one probe operation is allowed.
//     Success closes the circuit; failure reopens it.
//
// The circuit breaker implements crdbgo.Middleware, so it composes with the
// existing middleware chain:
//
//	cb := circuitbreaker.New(
//	    circuitbreaker.WithFailureThreshold(5),
//	    circuitbreaker.WithBreakDuration(30 * time.Second),
//	)
//
//	chain := crdbgo.Chain(cb.Middleware())
//	err := chain(myTxFunc)(ctx)
//
// It is safe for concurrent use from multiple goroutines.
package circuitbreaker
