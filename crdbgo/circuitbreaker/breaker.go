package circuitbreaker

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/felipeg48/cockroachdb-go-library/crdbgo"
)

// State represents the circuit breaker's current state.
type State int

const (
	// Closed is the normal operating state. Operations are allowed and failures
	// are counted toward the threshold.
	Closed State = iota

	// Open means the circuit breaker has tripped. All operations are blocked
	// until the break duration expires.
	Open

	// HalfOpen allows a single probe operation after the break duration expires.
	// If the probe succeeds the circuit closes; if it fails the circuit reopens.
	HalfOpen
)

// String returns the human-readable state name.
func (s State) String() string {
	switch s {
	case Closed:
		return "closed"
	case Open:
		return "open"
	case HalfOpen:
		return "half-open"
	default:
		return fmt.Sprintf("unknown(%d)", int(s))
	}
}

// CircuitBreakerOpenError is returned when the circuit breaker is open and
// blocking operations.
type CircuitBreakerOpenError struct {
	// Since is when the circuit breaker opened.
	Since time.Time
	// Duration is how long the circuit breaker will remain open.
	Duration time.Duration
}

// Error returns a human-readable message including how long ago the circuit
// opened and how much time remains until the half-open probe.
func (e *CircuitBreakerOpenError) Error() string {
	remaining := time.Until(e.Since.Add(e.Duration))
	if remaining < 0 {
		remaining = 0
	}
	return fmt.Sprintf("circuit breaker is open (opened %s ago, %s remaining)",
		time.Since(e.Since).Truncate(time.Millisecond),
		remaining.Truncate(time.Millisecond))
}

// StateChangeFunc is called when the circuit breaker transitions between states.
// It receives the previous state and the new state.
type StateChangeFunc func(from, to State)

// CircuitBreaker tracks consecutive failures and blocks operations when the
// failure threshold is reached. It is safe for concurrent use.
type CircuitBreaker struct {
	mu sync.Mutex

	state        State
	failureCount int
	openedAt     time.Time

	failureThreshold int
	breakDuration    time.Duration
	onStateChange    StateChangeFunc

	// now is a replaceable clock for testing.
	now func() time.Time
}

// Option is a functional option that configures a [CircuitBreaker]. Pass
// options to [New] to customize thresholds and callbacks.
type Option func(*CircuitBreaker)

// WithFailureThreshold sets the number of consecutive failures required to
// trip the circuit breaker from Closed to Open. Default is 5.
func WithFailureThreshold(n int) Option {
	return func(cb *CircuitBreaker) {
		if n > 0 {
			cb.failureThreshold = n
		}
	}
}

// WithBreakDuration sets how long the circuit breaker stays Open before
// transitioning to HalfOpen. Default is 30 seconds.
func WithBreakDuration(d time.Duration) Option {
	return func(cb *CircuitBreaker) {
		if d > 0 {
			cb.breakDuration = d
		}
	}
}

// WithOnStateChange registers a callback invoked on every state transition.
// The callback is called while the circuit breaker lock is held, so it must
// not call back into the circuit breaker.
func WithOnStateChange(fn StateChangeFunc) Option {
	return func(cb *CircuitBreaker) {
		cb.onStateChange = fn
	}
}

// New creates a CircuitBreaker with the given options. Defaults:
//   - FailureThreshold: 5
//   - BreakDuration: 30s
func New(opts ...Option) *CircuitBreaker {
	cb := &CircuitBreaker{
		state:            Closed,
		failureThreshold: 5,
		breakDuration:    30 * time.Second,
		now:              time.Now,
	}
	for _, opt := range opts {
		opt(cb)
	}
	return cb
}

// State returns the circuit breaker's current state.
func (cb *CircuitBreaker) State() State {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.state
}

// FailureCount returns the current consecutive failure count.
func (cb *CircuitBreaker) FailureCount() int {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return cb.failureCount
}

// Reset forces the circuit breaker back to the Closed state with zero failures.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	old := cb.state
	cb.state = Closed
	cb.failureCount = 0
	if old != Closed && cb.onStateChange != nil {
		cb.onStateChange(old, Closed)
	}
}

// Execute runs fn if the circuit allows it, tracking success/failure.
// Returns CircuitBreakerOpenError if the circuit is open.
func (cb *CircuitBreaker) Execute(ctx context.Context, fn func(context.Context) error) error {
	if err := cb.beforeExecute(); err != nil {
		return err
	}

	err := fn(ctx)

	if err != nil {
		cb.onFailure()
	} else {
		cb.onSuccess()
	}

	return err
}

// Middleware returns a crdbgo.Middleware that wraps operations with the circuit
// breaker. This allows composing the circuit breaker with other middleware
// in a chain:
//
//	chain := crdbgo.Chain(cb.Middleware(), loggingMiddleware)
func (cb *CircuitBreaker) Middleware() crdbgo.Middleware {
	return func(next crdbgo.TxFunc) crdbgo.TxFunc {
		return func(ctx context.Context) error {
			return cb.Execute(ctx, func(ctx context.Context) error {
				return next(ctx)
			})
		}
	}
}

// beforeExecute checks whether the circuit allows execution. It handles
// the Open -> HalfOpen transition when the break duration has elapsed.
func (cb *CircuitBreaker) beforeExecute() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case Closed:
		return nil

	case Open:
		if cb.now().Sub(cb.openedAt) >= cb.breakDuration {
			cb.setState(HalfOpen)
			return nil
		}
		return &CircuitBreakerOpenError{
			Since:    cb.openedAt,
			Duration: cb.breakDuration,
		}

	case HalfOpen:
		// Only one probe is allowed; subsequent calls while still HalfOpen
		// are blocked until the probe completes.
		return nil

	default:
		return nil
	}
}

// onSuccess handles a successful operation.
func (cb *CircuitBreaker) onSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount = 0

	if cb.state == HalfOpen {
		cb.setState(Closed)
	}
}

// onFailure handles a failed operation.
func (cb *CircuitBreaker) onFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.failureCount++

	switch cb.state {
	case Closed:
		if cb.failureCount >= cb.failureThreshold {
			cb.openedAt = cb.now()
			cb.setState(Open)
		}

	case HalfOpen:
		// Probe failed — reopen the circuit.
		cb.openedAt = cb.now()
		cb.failureCount = 0
		cb.setState(Open)
	}
}

// setState transitions to a new state and fires the callback.
// Must be called with cb.mu held.
func (cb *CircuitBreaker) setState(to State) {
	from := cb.state
	cb.state = to
	if cb.onStateChange != nil {
		cb.onStateChange(from, to)
	}
}
